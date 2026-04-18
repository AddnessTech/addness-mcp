package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AddnessClient is an HTTP client for the Addness REST API.
type AddnessClient struct {
	baseURL    string
	mu         sync.RWMutex
	token      string
	orgID      string // active organization (full UUID)
	memberID   string // current user's member ID in active org
	httpClient *http.Client
	ids        *ShortIDCache
}

func NewAddnessClient(baseURL string, ids *ShortIDCache) *AddnessClient {
	c := &AddnessClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ids: ids,
	}
	c.loadSession()
	return c
}

func (c *AddnessClient) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

func (c *AddnessClient) SetOrganization(orgID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgID = c.ids.resolveOrFallback(orgID)
	c.memberID = ""
	c.saveSession()
}

func (c *AddnessClient) SetMemberID(memberID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memberID = memberID
	c.saveSession()
}

// --- Session persistence ---

type persistedSession struct {
	OrgID    string `json:"orgId"`
	MemberID string `json:"memberId"`
}

func sessionPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cacheDir, "addness-mcp", "session.json")
}

// loadSession restores orgID and memberID from disk. Called once at startup.
func (c *AddnessClient) loadSession() {
	path := sessionPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s persistedSession
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	c.orgID = s.OrgID
	c.memberID = s.MemberID
}

// saveSession writes orgID and memberID to disk. Must be called with c.mu held.
func (c *AddnessClient) saveSession() {
	path := sessionPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	s := persistedSession{
		OrgID:    c.orgID,
		MemberID: c.memberID,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func (c *AddnessClient) OrganizationID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.orgID
}

func (c *AddnessClient) MemberID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.memberID
}

func (c *AddnessClient) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	c.mu.RLock()
	token := c.token
	orgID := c.orgID
	c.mu.RUnlock()

	if token == "" {
		return nil, fmt.Errorf("not authenticated: call auth_login first")
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	// V1 team endpoints require X-Organization-ID header.
	// V2 endpoints use org ID from the URL path.
	if orgID != "" {
		req.Header.Set("X-Organization-ID", orgID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func (c *AddnessClient) Get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *AddnessClient) Post(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodPost, path, body)
}

func (c *AddnessClient) Patch(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodPatch, path, body)
}

func (c *AddnessClient) Put(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodPut, path, body)
}

func (c *AddnessClient) Delete(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodDelete, path, body)
}

func (c *AddnessClient) doJSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling body: %w", err)
		}
		reader = strings.NewReader(string(b))
	}
	return c.do(ctx, method, path, reader)
}
