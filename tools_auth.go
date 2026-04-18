package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func authLoginTool() mcp.Tool {
	return mcp.NewTool("auth_login",
		mcp.WithDescription("Addness APIキーを設定する。設定画面で発行したAPIキー(sk-...)を指定。"),
		mcp.WithString("token",
			mcp.Required(),
			mcp.Description("API key (sk-...)"),
		),
	)
}

func handleAuthLogin(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		token, _ := args["token"].(string)
		if token == "" {
			return errResult("token is required"), nil
		}

		client.SetToken(token)

		// Verify by fetching organizations
		data, err := client.Get(ctx, "/api/v2/organizations/me")
		if err != nil {
			client.SetToken("")
			return errResult(fmt.Sprintf("authentication failed: %v", err)), nil
		}

		orgs, err := parseOrganizations(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parsing organizations: %v", err)), nil
		}

		// Auto-select if single org
		if len(orgs) == 1 {
			client.SetOrganization(orgs[0].fullID)
		}

		result := "Authenticated successfully.\n\n"
		result += formatOrganizations(orgs)
		if len(orgs) == 1 {
			result += fmt.Sprintf("\nAuto-selected organization: %s (%s)", orgs[0].Name, orgs[0].ID)
		} else if len(orgs) > 1 {
			result += "\nUse switch_organization to select an organization."
		}

		return textResult(result), nil
	}
}
