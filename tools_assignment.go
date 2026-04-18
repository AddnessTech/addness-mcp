package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func assignMemberTool() mcp.Tool {
	return mcp.NewTool("assign_member",
		mcp.WithDescription(
			"Goalにメンバーをアサインする。アサインされたメンバーにはGoalが表示され、通知も届く。"+
				"ロールはOWNER（責任者、1名のみ）/EDITOR（編集者）/MEMBER（メンバー）から選択。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("Member ID (short ID) — use list_members to find IDs"),
		),
		mcp.WithString("role",
			mcp.Description("Role: OWNER (one per goal), EDITOR, or MEMBER (default: MEMBER)"),
			mcp.Enum("OWNER", "EDITOR", "MEMBER"),
		),
	)
}

func handleAssignMember(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		memberID, err := client.ids.Resolve(argStr(args, "member_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := map[string]any{
			"organizationMemberId": memberID,
		}
		if role := argStr(args, "role"); role != "" {
			body["role"] = role
		}

		data, err := client.Post(ctx, fmt.Sprintf("/api/v2/objectives/%s/assignments", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		assignment, err := parseAssignment(data)
		if err != nil {
			return textResult("Member assigned."), nil
		}
		return textResult(fmt.Sprintf("Member assigned.\n\n%s", formatAssignment(assignment))), nil
	}
}

func unassignMemberTool() mcp.Tool {
	return mcp.NewTool("unassign_member",
		mcp.WithDescription("Goalからメンバーのアサインを解除する。assignment_idはget_goalの結果から取得できる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("assignment_id",
			mcp.Required(),
			mcp.Description("Assignment ID (short ID) — shown in get_goal response"),
		),
	)
}

func handleUnassignMember(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		assignmentID, err := client.ids.Resolve(argStr(args, "assignment_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		_, err = client.Delete(ctx, fmt.Sprintf("/api/v2/objectives/%s/assignments/%s", goalID, assignmentID), nil)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Member unassigned."), nil
	}
}

func listAssignmentsTool() mcp.Tool {
	return mcp.NewTool("list_assignments",
		mcp.WithDescription("Goalのアサインメンバー一覧を取得する。各メンバーのロール（OWNER/EDITOR/MEMBER）と assignment_id を確認できる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleListAssignments(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/assignments", goalID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		assignments, err := parseAssignments(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(assignments) == 0 {
			return textResult("No members assigned."), nil
		}
		return textResult(formatAssignments(assignments)), nil
	}
}

// --- Parsing & Formatting ---

type assignmentInfo struct {
	ID         string
	MemberID   string
	MemberName string
	Role       string
	IsAIAgent  bool
	IsExternal bool
}

func parseAssignment(data []byte) (assignmentInfo, error) {
	unwrapped := unwrapData(data)
	var raw map[string]any
	if err := json.Unmarshal(unwrapped, &raw); err != nil {
		return assignmentInfo{}, err
	}
	return extractAssignment(raw), nil
}

func parseAssignments(data []byte, ids *ShortIDCache) ([]assignmentInfo, error) {
	unwrapped := unwrapData(data)
	var wrapper struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return nil, err
	}

	assignments := make([]assignmentInfo, 0, len(wrapper.Assignments))
	for _, a := range wrapper.Assignments {
		info := extractAssignment(a)
		info.ID = ids.Shorten(info.ID)
		info.MemberID = ids.Shorten(info.MemberID)
		assignments = append(assignments, info)
	}
	return assignments, nil
}

func extractAssignment(a map[string]any) assignmentInfo {
	info := assignmentInfo{}

	if id, ok := a["id"].(string); ok {
		info.ID = id
	}

	if role, ok := a["role"].(map[string]any); ok {
		info.Role, _ = role["name"].(string)
	}

	if member, ok := a["member"].(map[string]any); ok {
		info.MemberID, _ = member["id"].(string)
		info.MemberName, _ = member["name"].(string)
		info.IsAIAgent, _ = member["isAiAgent"].(bool)
		info.IsExternal, _ = member["isExternal"].(bool)
	}

	return info
}

func formatAssignment(a assignmentInfo) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s (%s)", a.MemberName, a.Role))
	if a.IsAIAgent {
		parts = append(parts, "[AI]")
	}
	if a.IsExternal {
		parts = append(parts, "[External]")
	}
	return strings.Join(parts, " ")
}

func formatAssignments(assignments []assignmentInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Assignments (%d):\n\n", len(assignments))
	for _, a := range assignments {
		tags := ""
		if a.IsAIAgent {
			tags += " [AI]"
		}
		if a.IsExternal {
			tags += " [External]"
		}
		fmt.Fprintf(&sb, "  [%s] %s — %s (member:%s)%s\n", a.ID, a.MemberName, a.Role, a.MemberID, tags)
	}
	return sb.String()
}
