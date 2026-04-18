package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	frontendBaseURL = "https://www.addness.com"
	loginTimeout    = 5 * time.Minute
	httpTimeout     = 30 * time.Second
)

var loginHTTPClient = &http.Client{Timeout: httpTimeout}

// runLogin performs the Desktop Auth flow to obtain an API key.
func runLogin() error {
	apiURL := os.Getenv("ADDNESS_API_URL")
	if apiURL == "" {
		apiURL = "https://vt.api.addness.com"
	}
	frontURL := os.Getenv("ADDNESS_FRONTEND_URL")
	if frontURL == "" {
		frontURL = frontendBaseURL
	}

	fmt.Fprintln(os.Stderr, "Addness MCP - Login")
	fmt.Fprintln(os.Stderr, "")

	// 1. Generate Ed25519 keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating keypair: %w", err)
	}
	pubPEM := marshalEd25519PublicKey(pub)
	installationID := generateID()

	// 2. Register installation
	fmt.Fprintln(os.Stderr, "Registering installation...")
	if err := registerInstallation(apiURL, installationID, pubPEM); err != nil {
		return fmt.Errorf("registering installation: %w", err)
	}

	// 3. Generate PKCE code verifier/challenge
	codeVerifier := generateBase64URL(32)
	codeChallenge := sha256Base64URL(codeVerifier)

	// 4. Start localhost callback server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer func() { _ = listener.Close() }()

	// 5. Create start session
	state := generateID()
	timestamp := time.Now().Unix()

	startMsg := buildStartSignatureMessage(
		installationID, state, strconv.Itoa(port),
		"/organization/set", codeChallenge, "/sign-in", "",
		timestamp,
	)
	signature := signMessage(priv, startMsg)

	fmt.Fprintln(os.Stderr, "Creating auth session...")
	startToken, err := createStartSession(apiURL, installationID, state, strconv.Itoa(port), codeChallenge, signature, timestamp)
	if err != nil {
		return fmt.Errorf("creating start session: %w", err)
	}

	// 6. Open browser
	browserURL := fmt.Sprintf("%s/desktop/browser-auth?start_token=%s&source=cli", frontURL, startToken)
	fmt.Fprintf(os.Stderr, "Opening browser for login...\n")
	fmt.Fprintf(os.Stderr, "If browser doesn't open, visit:\n  %s\n\n", browserURL)
	openBrowser(browserURL)

	// 7. Wait for callback
	fmt.Fprintln(os.Stderr, "Waiting for login...")
	ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
	defer cancel()

	handoffID, err := waitForCallback(ctx, listener, state)
	if err != nil {
		return fmt.Errorf("waiting for callback: %w", err)
	}

	// 8. Exchange token
	fmt.Fprintln(os.Stderr, "Exchanging token...")
	exchangeTimestamp := time.Now().Unix()
	exchangeMsg := buildExchangeSignatureMessage(installationID, handoffID, codeVerifier, exchangeTimestamp)
	exchangeSig := signMessage(priv, exchangeMsg)

	result, err := exchangeToken(apiURL, handoffID, codeVerifier, installationID, exchangeSig, exchangeTimestamp)
	if err != nil {
		return fmt.Errorf("exchanging token: %w", err)
	}

	if result.APIKey == "" {
		return fmt.Errorf("no API key returned (user may not belong to any organization)")
	}

	// 9. Output config
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Login successful!")
	fmt.Fprintln(os.Stderr, "")

	if len(result.Organizations) > 0 {
		fmt.Fprintln(os.Stderr, "Organizations:")
		for _, org := range result.Organizations {
			fmt.Fprintf(os.Stderr, "  - %s (%s)\n", org.Name, org.ID)
		}
		fmt.Fprintln(os.Stderr, "")
	}

	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = "addness-mcp"
	}

	fmt.Fprintln(os.Stderr, "Run the following command to configure Claude Code:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "  claude mcp add -t stdio -e ADDNESS_API_TOKEN='%s' -e ADDNESS_API_URL='%s' -s user addness -- '%s'\n", result.APIKey, apiURL, execPath)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Then restart Claude Code.")

	return nil
}

// --- Desktop Auth API calls ---

func registerInstallation(apiURL, installationID, publicKeyPEM string) error {
	body, _ := json.Marshal(map[string]string{
		"installationId": installationID,
		"publicKey":      publicKeyPEM,
	})
	resp, err := loginHTTPClient.Post(apiURL+"/api/v1/public/desktop/auth/installations/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return nil
}

func createStartSession(apiURL, installationID, state, port, codeChallenge, signature string, timestamp int64) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"installationId": installationID,
		"state":          state,
		"port":           port,
		"nextPath":       "/organization/set",
		"codeChallenge":  codeChallenge,
		"authPath":       "/sign-in",
		"timestamp":      timestamp,
		"signature":      signature,
	})
	resp, err := loginHTTPClient.Post(apiURL+"/api/v1/public/desktop/auth/start-sessions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Data struct {
			StartToken string `json:"start_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result.Data.StartToken, nil
}

type exchangeResult struct {
	APIKey        string
	Organizations []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
}

func exchangeToken(apiURL, handoffID, codeVerifier, installationID, signature string, timestamp int64) (*exchangeResult, error) {
	body, _ := json.Marshal(map[string]any{
		"handoffId":      handoffID,
		"codeVerifier":   codeVerifier,
		"installationId": installationID,
		"timestamp":      timestamp,
		"signature":      signature,
		"source":         "cli",
	})
	resp, err := loginHTTPClient.Post(apiURL+"/api/v1/public/desktop/auth/token-exchange", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	var raw struct {
		Data struct {
			APIKey        string `json:"api_key"`
			Organizations []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"organizations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &exchangeResult{
		APIKey:        raw.Data.APIKey,
		Organizations: raw.Data.Organizations,
	}, nil
}

// --- Localhost callback server ---

func waitForCallback(ctx context.Context, listener net.Listener, expectedState string) (string, error) {
	handoffCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback/", func(w http.ResponseWriter, r *http.Request) {
		callbackState := r.URL.Query().Get("state")
		handoffID := r.URL.Query().Get("handoff_id")

		if callbackState != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("state mismatch: expected %s, got %s", expectedState, callbackState)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:60px">
<h2>Login successful!</h2><p>You can close this tab and return to the terminal.</p>
</body></html>`)

		handoffCh <- handoffID
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	select {
	case handoffID := <-handoffCh:
		return handoffID, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("login timed out (waited %v)", loginTimeout)
	}
}

// --- Crypto helpers ---

func marshalEd25519PublicKey(pub ed25519.PublicKey) string {
	derBytes, _ := x509.MarshalPKIXPublicKey(pub)
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: derBytes}
	return string(pem.EncodeToMemory(block))
}

func signMessage(priv ed25519.PrivateKey, message string) string {
	sig := ed25519.Sign(priv, []byte(message))
	return base64.RawURLEncoding.EncodeToString(sig)
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func generateBase64URL(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// Match pattern: [A-Za-z0-9-]{1,128}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	return strings.ReplaceAll(encoded, "_", "x")
}

func buildStartSignatureMessage(installationID, state, port, nextPath, codeChallenge, authPath, referralCode string, timestamp int64) string {
	return strings.Join([]string{
		"visiontodo-desktop-auth-start",
		"installation_id=" + installationID,
		"state=" + state,
		"port=" + port,
		"next_path=" + nextPath,
		"code_challenge=" + codeChallenge,
		"auth_path=" + authPath,
		"referral_code=" + referralCode,
		"timestamp=" + strconv.FormatInt(timestamp, 10),
	}, "\n")
}

func buildExchangeSignatureMessage(installationID, handoffID, codeVerifier string, timestamp int64) string {
	return strings.Join([]string{
		"visiontodo-desktop-auth-exchange",
		"installation_id=" + installationID,
		"handoff_id=" + handoffID,
		"code_verifier=" + codeVerifier,
		"timestamp=" + strconv.FormatInt(timestamp, 10),
	}, "\n")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
