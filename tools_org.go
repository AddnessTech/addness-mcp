package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func listOrganizationsTool() mcp.Tool {
	return mcp.NewTool("list_organizations",
		mcp.WithDescription("所属する組織の一覧を取得する。現在アクティブな組織も表示される。"),
	)
}

func handleListOrganizations(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, err := client.Get(ctx, "/api/v2/organizations/me")
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		orgs, err := parseOrganizations(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		result := formatOrganizations(orgs)
		if client.OrganizationID() != "" {
			result += fmt.Sprintf("\nActive: %s", client.ids.Shorten(client.OrganizationID()))
		}
		return textResult(result), nil
	}
}

func listMembersTool() mcp.Tool {
	return mcp.NewTool("list_members",
		mcp.WithDescription("組織のメンバー一覧を取得する。メンバーIDはコメントのメンションやGoalへのアサインに使用できる。"),
		mcp.WithNumber("page_size",
			mcp.Description("Number of members per page (default: 100, max: 100)"),
		),
		mcp.WithNumber("page",
			mcp.Description("Page number, 1-indexed (default: 1)"),
		),
	)
}

func handleListMembers(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		pageSize := 100
		if v, ok := args["page_size"].(float64); ok && v > 0 {
			pageSize = int(v)
			if pageSize > 100 {
				pageSize = 100
			}
		}
		page := 1
		if v, ok := args["page"].(float64); ok && v > 0 {
			page = int(v)
		}

		path := "/api/v2/members?pageSize=" + strconv.Itoa(pageSize) + "&page=" + strconv.Itoa(page)
		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		members, paging, err := parseMembers(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(members) == 0 {
			return textResult("No members."), nil
		}

		result := fmt.Sprintf("Members (%d of %d):\n\n%s", len(members), paging.TotalCount, formatMembers(members))
		if paging.TotalPages > paging.Page {
			result += fmt.Sprintf("\n(page %d/%d — use page=%d to see more)", paging.Page, paging.TotalPages, paging.Page+1)
		}
		return textResult(result), nil
	}
}

func switchOrganizationTool() mcp.Tool {
	return mcp.NewTool("switch_organization",
		mcp.WithDescription("操作対象の組織を切り替える。複数組織に所属している場合に使用する。"),
		mcp.WithString("organization_id",
			mcp.Required(),
			mcp.Description("Organization ID (short ID)"),
		),
	)
}

func handleSwitchOrganization(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		orgID, _ := args["organization_id"].(string)
		if orgID == "" {
			return errResult("organization_id is required"), nil
		}

		// If the cache is cold (e.g. after restart), fetch orgs first so
		// that short IDs can be resolved to full UUIDs.
		_, err := client.ids.Resolve(orgID)
		if err != nil {
			// Cache miss — populate it by listing organizations.
			data, err := client.Get(ctx, "/api/v2/organizations/me")
			if err == nil {
				_, _ = parseOrganizations(data, client.ids) // populates cache
			}
		}

		client.SetOrganization(orgID)

		// Resolve current user's member ID
		memberData, err := client.Get(ctx, "/api/v2/members?pageSize=100")
		if err == nil {
			if mid := findCurrentMemberID(memberData); mid != "" {
				client.SetMemberID(mid)
			}
		}

		return textResult(fmt.Sprintf("Switched to organization: %s", client.ids.Shorten(client.OrganizationID()))), nil
	}
}
