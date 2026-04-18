package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setRecurringTool() mcp.Tool {
	return mcp.NewTool("set_recurring",
		mcp.WithDescription("Goalに定常（繰り返し）パターンを設定する。既に設定がある場合は上書き更新される。毎日・毎週・毎月・平日の基本パターン、またはカスタム繰り返しルールを設定可能。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("pattern",
			mcp.Description("Basic pattern. Use this OR the custom fields below, not both."),
			mcp.Enum("DAILY", "WEEKLY", "MONTHLY", "WEEKDAYS"),
		),
		mcp.WithString("recurrence_type",
			mcp.Description("Custom recurrence type (use instead of pattern for advanced rules)"),
			mcp.Enum("DAILY", "WEEKLY", "MONTHLY"),
		),
		mcp.WithNumber("interval",
			mcp.Description("Repeat every N days/weeks/months (default: 1). Used with recurrence_type."),
		),
		mcp.WithArray("days_of_week",
			mcp.Description("Days of week for WEEKLY recurrence (e.g. [\"MONDAY\",\"WEDNESDAY\",\"FRIDAY\"])"),
			mcp.Items(map[string]any{"type": "string", "enum": []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}}),
		),
		mcp.WithArray("days_of_month",
			mcp.Description("Days of month for MONTHLY recurrence (e.g. [1, 15])"),
			mcp.Items(map[string]any{"type": "number"}),
		),
		mcp.WithString("end_date",
			mcp.Description("End date for recurrence in YYYY-MM-DD format"),
		),
		mcp.WithBoolean("is_last_day",
			mcp.Description("true to recur on the last day of each month"),
		),
		mcp.WithNumber("nth_week",
			mcp.Description("Week number within month (1-5) for patterns like '2nd Tuesday'"),
		),
		mcp.WithBoolean("repeat_from_completion",
			mcp.Description("true to restart interval from completion date instead of fixed schedule"),
		),
	)
}

func handleSetRecurring(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := make(map[string]any)

		pattern := argStr(args, "pattern")
		recurrenceType := argStr(args, "recurrence_type")

		if pattern != "" && recurrenceType != "" {
			return errResult("specify either 'pattern' (basic) or 'recurrence_type' (custom), not both"), nil
		}
		if pattern == "" && recurrenceType == "" {
			return errResult("either 'pattern' or 'recurrence_type' is required"), nil
		}

		if pattern != "" {
			body["pattern"] = pattern
		}

		if recurrenceType != "" {
			body["recurrenceType"] = recurrenceType
			if v, ok := args["interval"].(float64); ok {
				body["interval"] = int(v)
			}
			if v := argStr(args, "end_date"); v != "" {
				body["endDate"] = v + "T00:00:00Z"
			}
			if v, ok := args["is_last_day"].(bool); ok {
				body["isLastDay"] = v
			}
			if v, ok := args["nth_week"].(float64); ok {
				body["nthWeek"] = int(v)
			}
			if v, ok := args["repeat_from_completion"].(bool); ok {
				body["repeatFromCompletion"] = v
			}
			if v, ok := args["days_of_week"].([]any); ok {
				days := make([]string, 0, len(v))
				for _, d := range v {
					if s, ok := d.(string); ok {
						days = append(days, s)
					}
				}
				if len(days) > 0 {
					body["daysOfWeek"] = days
				}
			}
			if v, ok := args["days_of_month"].([]any); ok {
				days := make([]int, 0, len(v))
				for _, d := range v {
					if n, ok := d.(float64); ok {
						days = append(days, int(n))
					}
				}
				if len(days) > 0 {
					body["daysOfMonth"] = days
				}
			}
		}

		// Try PUT first (update existing), fall back to POST on 404 only
		path := fmt.Sprintf("/api/v2/objectives/%s/recurring", goalID)
		data, err := client.Put(ctx, path, body)
		if err != nil {
			if !strings.Contains(err.Error(), "API error 404") {
				return errResult(fmt.Sprintf("failed: %v", err)), nil
			}
			// 404 means no existing recurring — create new
			data, err = client.Post(ctx, path, body)
			if err != nil {
				return errResult(fmt.Sprintf("failed: %v", err)), nil
			}
		}

		rec := parseRecurringResponse(data)
		return textResult("Recurring pattern set.\n\n" + formatRecurringDetail(rec)), nil
	}
}

func removeRecurringTool() mcp.Tool {
	return mcp.NewTool("remove_recurring",
		mcp.WithDescription("Goalから定常（繰り返し）パターンを解除する。通常のGoalに戻す。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleRemoveRecurring(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		_, err = client.Delete(ctx, fmt.Sprintf("/api/v2/objectives/%s/recurring", goalID), nil)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Recurring pattern removed."), nil
	}
}

func getRecurringTool() mcp.Tool {
	return mcp.NewTool("get_recurring",
		mcp.WithDescription("Goalの定常（繰り返し）パターン詳細を取得する。パターン種別・間隔・曜日・日付など全設定を確認できる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleGetRecurring(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/recurring", goalID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		rec := parseRecurringResponse(data)
		if rec.Description == "" && rec.Pattern == "" && rec.RecurrenceType == "" {
			return textResult("No recurring pattern set on this goal."), nil
		}

		return textResult(formatRecurringDetail(rec)), nil
	}
}

// recurringInfo holds parsed recurring goal pattern details.
type recurringInfo struct {
	Pattern              string
	Description          string
	IsBasicPattern       bool
	RecurrenceType       string
	Interval             int
	DaysOfWeek           []string
	DaysOfMonth          []int
	EndDate              string
	IsLastDay            bool
	NthWeek              int
	RepeatFromCompletion bool
}

func parseRecurringResponse(data []byte) recurringInfo {
	var raw map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return recurringInfo{}
	}

	info := recurringInfo{}
	info.Description, _ = raw["description"].(string)
	info.IsBasicPattern, _ = raw["isBasicPattern"].(bool)

	if v, ok := raw["pattern"].(string); ok {
		info.Pattern = v
	}
	if v, ok := raw["recurrenceType"].(string); ok {
		info.RecurrenceType = v
	}
	if v, ok := raw["interval"].(float64); ok {
		info.Interval = int(v)
	}
	if v, ok := raw["daysOfWeek"].([]any); ok {
		for _, d := range v {
			if s, ok := d.(string); ok {
				info.DaysOfWeek = append(info.DaysOfWeek, s)
			}
		}
	}
	if v, ok := raw["daysOfMonth"].([]any); ok {
		for _, d := range v {
			if n, ok := d.(float64); ok {
				info.DaysOfMonth = append(info.DaysOfMonth, int(n))
			}
		}
	}
	if v, ok := raw["endDate"].(string); ok {
		info.EndDate = formatTime(v)
	}
	if v, ok := raw["isLastDay"].(bool); ok {
		info.IsLastDay = v
	}
	if v, ok := raw["nthWeek"].(float64); ok {
		info.NthWeek = int(v)
	}
	if v, ok := raw["repeatFromCompletion"].(bool); ok {
		info.RepeatFromCompletion = v
	}

	return info
}

func formatRecurringDetail(r recurringInfo) string {
	var sb strings.Builder
	sb.WriteString("## Recurring Pattern\n")

	if r.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", r.Description)
	}

	if r.IsBasicPattern && r.Pattern != "" {
		fmt.Fprintf(&sb, "Type: Basic (%s)\n", r.Pattern)
	} else if r.RecurrenceType != "" {
		fmt.Fprintf(&sb, "Type: Custom (%s", r.RecurrenceType)
		if r.Interval > 1 {
			fmt.Fprintf(&sb, ", every %d", r.Interval)
		}
		sb.WriteString(")\n")

		if len(r.DaysOfWeek) > 0 {
			fmt.Fprintf(&sb, "Days of week: %s\n", strings.Join(r.DaysOfWeek, ", "))
		}
		if len(r.DaysOfMonth) > 0 {
			parts := make([]string, len(r.DaysOfMonth))
			for i, d := range r.DaysOfMonth {
				parts[i] = fmt.Sprintf("%d", d)
			}
			fmt.Fprintf(&sb, "Days of month: %s\n", strings.Join(parts, ", "))
		}
		if r.IsLastDay {
			sb.WriteString("Last day of month: yes\n")
		}
		if r.NthWeek > 0 {
			fmt.Fprintf(&sb, "Week: %d\n", r.NthWeek)
		}
		if r.EndDate != "" {
			fmt.Fprintf(&sb, "End date: %s\n", r.EndDate)
		}
		if r.RepeatFromCompletion {
			sb.WriteString("Repeat from completion: yes\n")
		}
	}

	return sb.String()
}
