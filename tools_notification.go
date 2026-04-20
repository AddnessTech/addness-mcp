package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func listNotificationsTool() mcp.Tool {
	return mcp.NewTool("list_notifications",
		mcp.WithDescription("通知一覧を取得する。メンション・コメント・Goal更新などの通知を未読/既読でフィルタ可能。"),
		mcp.WithString("status",
			mcp.Description("Filter: 'unread', 'read', or 'all' (default: 'unread')"),
			mcp.Enum("unread", "read", "all"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max number of notifications to return (default: 50, max: 100)"),
		),
		mcp.WithString("category",
			mcp.Description("Filter by notification category"),
			mcp.Enum("mentions", "comments", "reactions", "completed", "created", "updates", "assignments", "deliverables", "ai"),
		),
	)
}

func handleListNotifications(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		status, _ := args["status"].(string)
		if status == "" {
			status = "unread"
		}

		limit := 50
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 100 {
				limit = 100
			}
		}

		path := fmt.Sprintf("/api/v2/organizations/%s/notifications?limit=%s", client.OrganizationID(), strconv.Itoa(limit))
		if status != "all" {
			readVal := "false"
			if status == "read" {
				readVal = "true"
			}
			path += "&read=" + readVal
		}
		if category := argStr(args, "category"); category != "" {
			path += "&event_tag=" + category
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		notifications, unreadCount, err := parseNotifications(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		result := fmt.Sprintf("Notifications (filter: %s, unread: %d)\n\n", status, unreadCount)
		if len(notifications) == 0 {
			result += "No notifications."
		} else {
			result += formatNotifications(notifications)
		}
		return textResult(result), nil
	}
}

func markNotificationsReadTool() mcp.Tool {
	return mcp.NewTool("mark_notifications_read",
		mcp.WithDescription("通知を既読にする。個別のIDを指定するか、'all'で全件一括既読にできる。"),
		mcp.WithString("notification_ids",
			mcp.Description("Comma-separated notification IDs, or 'all' to mark all as read"),
			mcp.Required(),
		),
	)
}

func handleMarkNotificationsRead(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		idsStr, _ := args["notification_ids"].(string)

		basePath := fmt.Sprintf("/api/v2/organizations/%s/notifications", client.OrganizationID())

		if idsStr == "all" {
			_, err := client.Post(ctx, basePath+"/mark-all-read", nil)
			if err != nil {
				return errResult(fmt.Sprintf("failed: %v", err)), nil
			}
			return textResult("All notifications marked as read."), nil
		}

		ids, err := splitAndResolve(idsStr, client.ids)
		if err != nil {
			return errResult(err.Error()), nil
		}
		body := map[string]any{"notificationIds": ids}
		_, err = client.Post(ctx, basePath+"/mark-read", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Marked %d notification(s) as read.", len(ids))), nil
	}
}
