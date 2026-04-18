package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validateDate(v string) error {
	if !datePattern.MatchString(v) {
		return fmt.Errorf("invalid date format %q (use YYYY-MM-DD)", v)
	}
	return nil
}

// --- Activity Log by Member ---

func getMemberActivityTool() mcp.Tool {
	return mcp.NewTool("get_member_activity",
		mcp.WithDescription("メンバーのアクティビティログを取得する。Goal作成・更新・完了・コメントなどの操作履歴で、メンバーの活動を把握できる。"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("Member ID (short ID)"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date in YYYY-MM-DD format"),
		),
		mcp.WithString("end_date",
			mcp.Description("End date in YYYY-MM-DD format"),
		),
		mcp.WithString("categories",
			mcp.Description("Comma-separated event categories to filter (objective,kpi,comment,deliverable,ai,mcp)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results (default: 30, max: 100)"),
		),
	)
}

func handleGetMemberActivity(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		memberID, err := client.ids.Resolve(argStr(args, "member_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		params := url.Values{}
		params.Set("member_id", memberID)

		if v := argStr(args, "start_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("start_date", v+"T00:00:00Z")
		}
		if v := argStr(args, "end_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("end_date", v+"T23:59:59.999999999Z")
		}
		if v := argStr(args, "categories"); v != "" {
			for _, cat := range strings.Split(v, ",") {
				cat = strings.TrimSpace(cat)
				if cat != "" {
					params.Add("event_categories[]", cat)
				}
			}
		}

		limit := 30
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 100 {
				limit = 100
			}
		}
		params.Set("limit", fmt.Sprintf("%d", limit))

		path := fmt.Sprintf("/api/v1/team/organizations/%s/activity-logs/by-member?%s", client.OrganizationID(), params.Encode())
		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		logs, total, err := parseActivityLogs(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(logs) == 0 {
			return textResult("No activity logs found."), nil
		}

		return textResult(fmt.Sprintf("Activity Logs (%d of %d)\n\n%s", len(logs), total, formatActivityLogs(logs))), nil
	}
}

// --- Activity Log by Goal ---

func getGoalActivityTool() mcp.Tool {
	return mcp.NewTool("get_goal_activity",
		mcp.WithDescription("特定のGoalのアクティビティログを取得する。誰がいつ何を変更したかの履歴で、Goal進捗の経緯を追える。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("start_date",
			mcp.Description("Start date in YYYY-MM-DD format"),
		),
		mcp.WithString("end_date",
			mcp.Description("End date in YYYY-MM-DD format"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("true to include child goals' activity (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of results (default: 30, max: 100)"),
		),
	)
}

func handleGetGoalActivity(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		params := url.Values{}
		if v := argStr(args, "start_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("start_date", v+"T00:00:00Z")
		}
		if v := argStr(args, "end_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("end_date", v+"T23:59:59.999999999Z")
		}
		if incl, _ := args["include_children"].(bool); incl {
			params.Set("include_children", "true")
		}

		limit := 30
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 100 {
				limit = 100
			}
		}
		params.Set("limit", fmt.Sprintf("%d", limit))

		path := fmt.Sprintf("/api/v1/team/organizations/%s/activity-logs/objectives/%s?%s", client.OrganizationID(), goalID, params.Encode())
		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		logs, total, err := parseActivityLogs(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(logs) == 0 {
			return textResult("No activity logs found for this goal."), nil
		}

		return textResult(fmt.Sprintf("Goal Activity Logs (%d of %d)\n\n%s", len(logs), total, formatActivityLogs(logs))), nil
	}
}

// --- Activity Summary ---

func getActivitySummaryTool() mcp.Tool {
	return mcp.NewTool("get_activity_summary",
		mcp.WithDescription("組織のアクティビティサマリーを取得する。カテゴリ別集計・アクティブメンバーTop10で、チーム全体の活動量を俯瞰できる。"),
		mcp.WithString("start_date",
			mcp.Description("Start date in YYYY-MM-DD format"),
		),
		mcp.WithString("end_date",
			mcp.Description("End date in YYYY-MM-DD format"),
		),
	)
}

func handleGetActivitySummary(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		params := url.Values{}
		if v := argStr(args, "start_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("start_date", v+"T00:00:00Z")
		}
		if v := argStr(args, "end_date"); v != "" {
			if err := validateDate(v); err != nil {
				return errResult(err.Error()), nil
			}
			params.Set("end_date", v+"T23:59:59.999999999Z")
		}

		path := fmt.Sprintf("/api/v1/team/organizations/%s/activity-logs/summary?%s", client.OrganizationID(), params.Encode())
		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		summary, err := parseActivitySummary(data)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		return textResult(formatActivitySummary(summary)), nil
	}
}

// --- Parsing & Formatting ---

type activityLogInfo struct {
	ID            string
	EventType     string
	EventCategory string
	OccurredAt    string
	ActorName     string
	Description   string
	GoalTitle     string
	ValueBefore   string
	ValueAfter    string
}

type activitySummaryInfo struct {
	TotalCount      int64
	CountByCategory map[string]int64
	MostActive      []activeMemberInfo
	RecentLogs      []activityLogInfo
}

type activeMemberInfo struct {
	Name  string
	Count int64
}

func parseActivityLogs(data []byte, ids *ShortIDCache) ([]activityLogInfo, int64, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, 0, err
	}

	// Handle V1 response wrapping: {"data": {"items": [...], "totalCount": N}}
	if d, ok := raw["data"].(map[string]any); ok {
		raw = d
	}

	totalCount := int64(0)
	if tc, ok := raw["totalCount"].(float64); ok {
		totalCount = int64(tc)
	}

	rawItems, _ := raw["items"].([]any)
	logs := make([]activityLogInfo, 0, len(rawItems))
	for _, item := range rawItems {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		log := activityLogInfo{
			EventType:     strVal(m, "eventType"),
			EventCategory: strVal(m, "eventCategory"),
			OccurredAt:    formatTime(strVal(m, "occurredAt")),
			Description:   strVal(m, "description"),
		}

		if actor, ok := m["actor"].(map[string]any); ok {
			log.ActorName, _ = actor["name"].(string)
		}

		if goalInfo, ok := m["goalInfo"].(map[string]any); ok {
			log.GoalTitle, _ = goalInfo["goalTitle"].(string)
		}

		if vc, ok := m["valueChange"].(map[string]any); ok {
			log.ValueBefore, _ = vc["beforeValue"].(string)
			log.ValueAfter, _ = vc["afterValue"].(string)
		}

		logs = append(logs, log)
	}
	return logs, totalCount, nil
}

func parseActivitySummary(data []byte) (activitySummaryInfo, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return activitySummaryInfo{}, err
	}

	// Handle V1 response wrapping
	if d, ok := raw["data"].(map[string]any); ok {
		raw = d
	}

	summary := activitySummaryInfo{
		CountByCategory: make(map[string]int64),
	}

	if tc, ok := raw["totalCount"].(float64); ok {
		summary.TotalCount = int64(tc)
	}

	if cbc, ok := raw["countByCategory"].(map[string]any); ok {
		for k, v := range cbc {
			if count, ok := v.(float64); ok {
				summary.CountByCategory[k] = int64(count)
			}
		}
	}

	if mam, ok := raw["mostActiveMembers"].([]any); ok {
		for _, m := range mam {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			name, _ := mm["memberName"].(string)
			count, _ := mm["count"].(float64)
			summary.MostActive = append(summary.MostActive, activeMemberInfo{
				Name:  name,
				Count: int64(count),
			})
		}
	}

	if ra, ok := raw["recentActivities"].([]any); ok {
		for _, item := range ra {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			log := activityLogInfo{
				EventType:     strVal(m, "eventType"),
				EventCategory: strVal(m, "eventCategory"),
				OccurredAt:    formatTime(strVal(m, "occurredAt")),
				Description:   strVal(m, "description"),
			}
			if actor, ok := m["actor"].(map[string]any); ok {
				log.ActorName, _ = actor["name"].(string)
			}
			summary.RecentLogs = append(summary.RecentLogs, log)
		}
	}

	return summary, nil
}

func formatActivityLogs(logs []activityLogInfo) string {
	var sb strings.Builder
	for _, l := range logs {
		fmt.Fprintf(&sb, "[%s] %s", l.OccurredAt, l.EventType)
		if l.ActorName != "" {
			fmt.Fprintf(&sb, " by %s", l.ActorName)
		}
		sb.WriteString("\n")
		if l.GoalTitle != "" {
			fmt.Fprintf(&sb, "  Goal: %s\n", l.GoalTitle)
		}
		if l.Description != "" {
			fmt.Fprintf(&sb, "  %s\n", l.Description)
		}
		if l.ValueBefore != "" || l.ValueAfter != "" {
			fmt.Fprintf(&sb, "  Change: %q → %q\n", l.ValueBefore, l.ValueAfter)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatActivitySummary(s activitySummaryInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Activity Summary\n\nTotal: %d activities\n\n", s.TotalCount)

	if len(s.CountByCategory) > 0 {
		sb.WriteString("## By Category\n")
		for cat, count := range s.CountByCategory {
			fmt.Fprintf(&sb, "  %s: %d\n", cat, count)
		}
		sb.WriteString("\n")
	}

	if len(s.MostActive) > 0 {
		sb.WriteString("## Most Active Members\n")
		for i, m := range s.MostActive {
			fmt.Fprintf(&sb, "  %d. %s (%d)\n", i+1, m.Name, m.Count)
		}
		sb.WriteString("\n")
	}

	if len(s.RecentLogs) > 0 {
		sb.WriteString("## Recent Activity\n")
		for _, l := range s.RecentLogs {
			fmt.Fprintf(&sb, "  [%s] %s", l.OccurredAt, l.EventType)
			if l.ActorName != "" {
				fmt.Fprintf(&sb, " by %s", l.ActorName)
			}
			if l.Description != "" {
				fmt.Fprintf(&sb, " — %s", truncate(l.Description, 60))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
