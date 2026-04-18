package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func inviteMembersTool() mcp.Tool {
	return mcp.NewTool("invite_members",
		mcp.WithDescription(
			"メールアドレスで組織にメンバーを招待する。"+
				"Addnessに未登録の仕事仲間をゴール管理に巻き込める。"+
				"招待されたメンバーにはメールが届き、アカウント作成後に組織に参加できる。"),
		mcp.WithString("emails",
			mcp.Required(),
			mcp.Description("Comma-separated email addresses to invite (e.g. 'alice@example.com, bob@example.com')"),
		),
	)
}

func handleInviteMembers(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		emailsStr := argStr(args, "emails")
		if emailsStr == "" {
			return errResult("emails is required"), nil
		}

		var emails []string
		for _, e := range strings.Split(emailsStr, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				emails = append(emails, e)
			}
		}
		if len(emails) == 0 {
			return errResult("at least one email is required"), nil
		}

		body := map[string]any{
			"emails": emails,
		}

		data, err := client.Post(ctx, fmt.Sprintf("/api/v2/organizations/%s/invitations", client.OrganizationID()), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		invited, err := parseInvitedMembers(data, client.ids)
		if err != nil {
			return textResult(fmt.Sprintf("Invited %d member(s). They will receive an email invitation.", len(emails))), nil
		}

		result := fmt.Sprintf("Invited %d member(s):\n\n%s", len(invited), formatInvitedMembers(invited))
		result += "\nThey will receive an email invitation to join the organization."
		return textResult(result), nil
	}
}

func createInviteLinkTool() mcp.Tool {
	return mcp.NewTool("create_invite_link",
		mcp.WithDescription(
			"組織への招待リンクを作成する。リンクを共有するだけで誰でも参加できる。"+
				"外部メンバー（他組織から参加）か正規メンバーかを選択可能。"+
				"使用回数の上限や有効期限を設定できる。"),
		mcp.WithString("code",
			mcp.Required(),
			mcp.Description("Invite link code (URL-safe string, e.g. 'my-team-2024')"),
		),
		mcp.WithNumber("max_uses",
			mcp.Description("Maximum number of times the link can be used. Omit for unlimited."),
		),
		mcp.WithString("expires_at",
			mcp.Description("Expiration date in YYYY-MM-DD format. Omit for no expiration."),
		),
		mcp.WithBoolean("is_external",
			mcp.Description("true to invite as external members (from other organizations). Default: false (regular members)."),
		),
	)
}

func handleCreateInviteLink(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		code := argStr(args, "code")
		if code == "" {
			return errResult("code is required"), nil
		}

		body := map[string]any{
			"code": code,
		}
		if v, ok := args["max_uses"].(float64); ok && v > 0 {
			body["maxUses"] = int(v)
		}
		if v := argStr(args, "expires_at"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(fmt.Sprintf("invalid expires_at: %v", err)), nil
			}
			body["expiresAt"] = v + "T23:59:59Z"
		}
		if v, ok := args["is_external"].(bool); ok {
			body["isExternal"] = v
		}

		data, err := client.Post(ctx, fmt.Sprintf("/api/v2/organizations/%s/invite-links", client.OrganizationID()), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		link, err := parseInviteLink(data, client.ids)
		if err != nil {
			return textResult("Invite link created."), nil
		}

		return textResult(fmt.Sprintf("Invite link created.\n\n%s\n\nShare this link to invite people to the organization.", formatInviteLink(link))), nil
	}
}

func listInviteLinksTool() mcp.Tool {
	return mcp.NewTool("list_invite_links",
		mcp.WithDescription("組織の招待リンク一覧を取得する。アクティブなリンクとその使用状況を確認できる。"),
	)
}

func handleListInviteLinks(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/organizations/%s/invite-links", client.OrganizationID()))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		links, err := parseInviteLinks(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(links) == 0 {
			return textResult("No invite links."), nil
		}
		return textResult(formatInviteLinks(links)), nil
	}
}

func listInvitedMembersTool() mcp.Tool {
	return mcp.NewTool("list_invited_members",
		mcp.WithDescription("招待中のメンバー一覧を取得する。招待のステータス（招待済み/承諾/辞退/期限切れ/取消）を確認できる。"),
		mcp.WithString("status",
			mcp.Description("Filter by status: INVITED, ACCEPTED, DECLINED, EXPIRED, REVOKED (default: all)"),
			mcp.Enum("INVITED", "ACCEPTED", "DECLINED", "EXPIRED", "REVOKED"),
		),
	)
}

func handleListInvitedMembers(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		path := fmt.Sprintf("/api/v2/organizations/%s/invited-members", client.OrganizationID())
		if status := argStr(args, "status"); status != "" {
			path += "?status=" + url.QueryEscape(strings.ToLower(status))
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		members, err := parseInvitedMembers(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(members) == 0 {
			return textResult("No invited members."), nil
		}
		return textResult(formatInvitedMembers(members)), nil
	}
}

// --- Parsing & Formatting ---

type invitedMemberInfo struct {
	ID        string
	Email     string
	Name      string
	Status    string
	ExpiresAt string
	CreatedAt string
}

type inviteLinkInfo struct {
	ID          string
	Code        string
	MaxUses     int
	CurrentUses int
	ExpiresAt   string
	IsActive    bool
	IsExternal  bool
}

func parseInvitedMembers(data []byte, ids *ShortIDCache) ([]invitedMemberInfo, error) {
	unwrapped := unwrapData(data)

	// Handle array or object wrapper
	var items []map[string]any
	if err := json.Unmarshal(unwrapped, &items); err != nil {
		var wrapper struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
			return nil, err
		}
		items = wrapper.Items
	}

	members := make([]invitedMemberInfo, 0, len(items))
	for _, m := range items {
		fullID, _ := m["id"].(string)
		members = append(members, invitedMemberInfo{
			ID:        ids.Shorten(fullID),
			Email:     strVal(m, "email"),
			Name:      strVal(m, "name"),
			Status:    strVal(m, "status"),
			ExpiresAt: formatTime(strVal(m, "expiresAt")),
			CreatedAt: formatTime(strVal(m, "createdAt")),
		})
	}
	return members, nil
}

func parseInviteLink(data []byte, ids *ShortIDCache) (inviteLinkInfo, error) {
	unwrapped := unwrapData(data)
	var raw map[string]any
	if err := json.Unmarshal(unwrapped, &raw); err != nil {
		return inviteLinkInfo{}, err
	}
	return extractInviteLink(raw, ids), nil
}

func parseInviteLinks(data []byte, ids *ShortIDCache) ([]inviteLinkInfo, error) {
	unwrapped := unwrapData(data)

	var items []map[string]any
	if err := json.Unmarshal(unwrapped, &items); err != nil {
		var wrapper struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
			return nil, err
		}
		items = wrapper.Items
	}

	links := make([]inviteLinkInfo, 0, len(items))
	for _, item := range items {
		links = append(links, extractInviteLink(item, ids))
	}
	return links, nil
}

func extractInviteLink(m map[string]any, ids *ShortIDCache) inviteLinkInfo {
	link := inviteLinkInfo{
		Code:      strVal(m, "code"),
		ExpiresAt: formatTime(strVal(m, "expiresAt")),
	}
	if id, ok := m["id"].(string); ok {
		link.ID = ids.Shorten(id)
	}
	if v, ok := m["maxUses"].(float64); ok {
		link.MaxUses = int(v)
	}
	if v, ok := m["currentUses"].(float64); ok {
		link.CurrentUses = int(v)
	}
	link.IsActive, _ = m["isActive"].(bool)
	link.IsExternal, _ = m["isExternal"].(bool)
	return link
}

func formatInvitedMembers(members []invitedMemberInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Invited Members (%d):\n\n", len(members))
	for _, m := range members {
		name := m.Email
		if m.Name != "" {
			name = m.Name + " <" + m.Email + ">"
		}
		fmt.Fprintf(&sb, "  [%s] %s — %s", m.ID, name, m.Status)
		if m.ExpiresAt != "" {
			fmt.Fprintf(&sb, " (expires: %s)", m.ExpiresAt)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// appBaseURL returns the frontend app URL from ADDNESS_APP_URL env,
// falling back to https://app.addness.com.
func appBaseURL() string {
	if v := os.Getenv("ADDNESS_APP_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://app.addness.com"
}

func formatInviteLink(link inviteLinkInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Code: %s\n", link.Code)
	if link.IsExternal {
		sb.WriteString("Type: External member\n")
	} else {
		sb.WriteString("Type: Regular member\n")
	}
	if link.MaxUses > 0 {
		fmt.Fprintf(&sb, "Uses: %d / %d\n", link.CurrentUses, link.MaxUses)
	} else {
		fmt.Fprintf(&sb, "Uses: %d (unlimited)\n", link.CurrentUses)
	}
	if link.ExpiresAt != "" {
		fmt.Fprintf(&sb, "Expires: %s\n", link.ExpiresAt)
	}
	sb.WriteString("Active: ")
	if link.IsActive {
		sb.WriteString("Yes")
	} else {
		sb.WriteString("No")
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "URL: %s/invite/%s", appBaseURL(), url.PathEscape(link.Code))
	return sb.String()
}

func formatInviteLinks(links []inviteLinkInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Invite Links (%d):\n\n", len(links))

	base := appBaseURL()
	for _, l := range links {
		status := "active"
		if !l.IsActive {
			status = "inactive"
		}
		memberType := "regular"
		if l.IsExternal {
			memberType = "external"
		}
		fmt.Fprintf(&sb, "  [%s] /%s — %s, %s", l.ID, l.Code, status, memberType)
		if l.MaxUses > 0 {
			fmt.Fprintf(&sb, ", %d/%d uses", l.CurrentUses, l.MaxUses)
		} else {
			fmt.Fprintf(&sb, ", %d uses", l.CurrentUses)
		}
		if l.ExpiresAt != "" {
			fmt.Fprintf(&sb, ", expires %s", l.ExpiresAt)
		}
		fmt.Fprintf(&sb, "\n    URL: %s/invite/%s\n", base, url.PathEscape(l.Code))
	}
	return sb.String()
}
