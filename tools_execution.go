package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func listTodaysGoalsTool() mcp.Tool {
	return mcp.NewTool("list_todays_goals",
		mcp.WithDescription("今日のGoal一覧をツリー形式で取得する。デイリーの実行対象を確認できる。member_idで他メンバーの一覧も取得可能。"),
		mcp.WithString("date",
			mcp.Description("Date in YYYY-MM-DD format (default: today)"),
		),
		mcp.WithString("member_id",
			mcp.Description("Member ID (short ID) to view another member's goals. Omit for your own."),
		),
		mcp.WithBoolean("include_completed",
			mcp.Description("true to include completed goals (default: false)"),
		),
		mcp.WithNumber("max_depth",
			mcp.Description("Maximum tree depth to include (default: 2). Use 0 for unlimited."),
		),
	)
}

func handleListTodaysGoals(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		date := argStr(args, "date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		path := fmt.Sprintf("/api/v2/organizations/%s/todays-goals?date=%s", client.OrganizationID(), url.QueryEscape(date))
		if memberID := argStr(args, "member_id"); memberID != "" {
			resolved, err := client.ids.Resolve(memberID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			path += "&member_id=" + url.QueryEscape(resolved)
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		nodes, err := parseTodaysGoals(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		includeCompleted, _ := args["include_completed"].(bool)
		maxDepth := 2
		if v, ok := args["max_depth"].(float64); ok {
			maxDepth = int(v)
		}

		filtered := nodes[:0]
		for _, n := range nodes {
			if !includeCompleted && (n.CompletedAt != "" || n.Status == "CANCELLED") {
				continue
			}
			if maxDepth > 0 && n.Depth >= maxDepth {
				continue
			}
			filtered = append(filtered, n)
		}
		nodes = filtered

		if len(nodes) == 0 {
			return textResult(fmt.Sprintf("No goals for %s.", date)), nil
		}
		return textResult(fmt.Sprintf("Today's Goals (%s)\n\n%s", date, formatTodaysGoals(nodes))), nil
	}
}

// --- Goal History ---

func getGoalHistoryTool() mcp.Tool {
	return mcp.NewTool("get_goal_history",
		mcp.WithDescription("特定の日付でのGoal履歴を取得する。その日にフォーカスしていたGoal一覧で、日付間の差分比較や振り返りに便利。"),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("Date in YYYY-MM-DD format"),
		),
		mcp.WithString("member_id",
			mcp.Description("Member ID (short ID) to view another member's history. Omit for your own."),
		),
	)
}

func handleGetGoalHistory(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		date := argStr(args, "date")
		if date == "" {
			return errResult("date is required (YYYY-MM-DD)"), nil
		}

		path := fmt.Sprintf("/api/v2/organizations/%s/goal-history?date=%s", client.OrganizationID(), url.QueryEscape(date))
		if memberID := argStr(args, "member_id"); memberID != "" {
			resolved, err := client.ids.Resolve(memberID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			path += "&member_id=" + url.QueryEscape(resolved)
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		nodes, err := parseTodaysGoals(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(nodes) == 0 {
			return textResult(fmt.Sprintf("No goal history for %s.", date)), nil
		}
		return textResult(fmt.Sprintf("Goal History (%s)\n\n%s", date, formatTodaysGoals(nodes))), nil
	}
}
