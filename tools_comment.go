package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addCommentTool() mcp.Tool {
	return mcp.NewTool("add_comment",
		mcp.WithDescription(
			"Goalにコメントを投稿する。コメントは進捗記録の場ではなく、"+
				"理想状態の実現に必要なコンテキストを収集するためのコミュニケーションの場。"+
				"自分の中にない情報が必要な時に、メンバーに質問・相談・確認を行う。"+
				"parent_idでスレッド返信も可能。mentionsでメンバーをメンションすると通知が届く。"+
				"メンションする場合は、content本文中にも @shortID を含めること（例: '@abc12345 確認お願いします'）。"+
				"AIエージェントが投稿する場合は、末尾に署名（例: 'Claude Codeより'）を付けて人間のコメントと区別すること。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Comment text (max 10000 chars)"),
		),
		mcp.WithString("parent_id",
			mcp.Description("Parent comment ID for thread reply (short ID). Omit for top-level comment."),
		),
		mcp.WithString("mentions",
			mcp.Description("Comma-separated member IDs (short ID) to @mention. They will receive a notification."),
		),
	)
}

func handleAddComment(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		content := argStr(args, "content")
		if content == "" {
			return errResult("content is required"), nil
		}
		content = client.ids.ExpandMentionsInContent(content)

		mentionUUIDs, err := resolveMentionIDs(argStr(args, "mentions"), client.ids)
		if err != nil {
			return errResult(err.Error()), nil
		}

		// Ensure all mentioned UUIDs appear as @UUID in content
		// so the backend can resolve them to display names on read.
		content = ensureMentionsInContent(content, mentionUUIDs)

		body := map[string]any{
			"commentableType": "objective",
			"commentableId":   goalID,
			"content":         content,
		}
		if len(mentionUUIDs) > 0 {
			body["mentions"] = mentionUUIDs
		}
		if v := argStr(args, "parent_id"); v != "" {
			resolved, err := client.ids.Resolve(v)
			if err != nil {
				return errResult(err.Error()), nil
			}
			body["parentId"] = resolved
		}

		_, err = client.Post(ctx, "/api/v1/team/comments", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		result := "Comment posted."
		if len(mentionUUIDs) > 0 {
			result += fmt.Sprintf(" (%d member(s) mentioned — they will be notified)", len(mentionUUIDs))
		}
		result += fmt.Sprintf("\nGoal URL: %s/goals/%s", frontendBaseURL, goalID)
		return textResult(result), nil
	}
}

func updateCommentTool() mcp.Tool {
	return mcp.NewTool("update_comment",
		mcp.WithDescription(
			"コメントを編集する。自分が投稿したコメントのみ編集可能。メンションの変更も可能。"+
				"メンションする場合は、content本文中にも @shortID を含めること。"+
				"mentionsを省略すると、content内のインライン@メンションが使われる（@shortIDがなければメンションはクリアされる）。"),
		mcp.WithString("comment_id",
			mcp.Required(),
			mcp.Description("Comment ID (short ID)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("New comment text (max 10000 chars)"),
		),
		mcp.WithString("mentions",
			mcp.Description("Comma-separated member IDs (short ID) to @mention. Also include @shortID in content text. Omit to use inline @mentions from content."),
		),
	)
}

func handleUpdateComment(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		commentID, err := client.ids.Resolve(argStr(args, "comment_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		content := argStr(args, "content")
		if content == "" {
			return errResult("content is required"), nil
		}
		content = client.ids.ExpandMentionsInContent(content)

		mentionUUIDs, err := resolveMentionIDs(argStr(args, "mentions"), client.ids)
		if err != nil {
			return errResult(err.Error()), nil
		}

		// Ensure all mentioned UUIDs appear as @UUID in content
		// so the backend can resolve them to display names on read.
		content = ensureMentionsInContent(content, mentionUUIDs)

		body := map[string]any{
			"content": content,
		}
		if len(mentionUUIDs) > 0 {
			body["mentions"] = mentionUUIDs
		}

		_, err = client.Put(ctx, fmt.Sprintf("/api/v1/team/comments/%s", commentID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Comment updated."), nil
	}
}

func deleteCommentTool() mcp.Tool {
	return mcp.NewTool("delete_comment",
		mcp.WithDescription("コメントを削除する。自分が投稿したコメントのみ削除可能。返信コメントも一緒に削除される。"),
		mcp.WithString("comment_id",
			mcp.Required(),
			mcp.Description("Comment ID (short ID)"),
		),
	)
}

func handleDeleteComment(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		commentID, err := client.ids.Resolve(argStr(args, "comment_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		_, err = client.Delete(ctx, fmt.Sprintf("/api/v1/team/comments/%s", commentID), nil)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Comment deleted."), nil
	}
}

func resolveCommentTool() mcp.Tool {
	return mcp.NewTool("resolve_comment",
		mcp.WithDescription(
			"コメントを解決済み/未解決にする。議論が結論に達したコメントを解決済みにして整理できる。"+
				"undo=trueで未解決に戻す。誰でも操作可能（投稿者以外もOK）。"),
		mcp.WithString("comment_id",
			mcp.Required(),
			mcp.Description("Comment ID (short ID)"),
		),
		mcp.WithBoolean("undo",
			mcp.Description("true to unresolve (revert to unresolved state)"),
		),
	)
}

func handleResolveComment(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		commentID, err := client.ids.Resolve(argStr(args, "comment_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		action := "resolve"
		if undo, _ := args["undo"].(bool); undo {
			action = "unresolve"
		}

		_, err = client.Patch(ctx, fmt.Sprintf("/api/v1/team/comments/%s/%s", commentID, action), nil)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		if action == "resolve" {
			return textResult("Comment resolved."), nil
		}
		return textResult("Comment unresolved."), nil
	}
}

func toggleReactionTool() mcp.Tool {
	return mcp.NewTool("toggle_reaction",
		mcp.WithDescription(
			"コメントにリアクション（絵文字）を追加/削除する。同じ絵文字で再度呼ぶと取り消し。"+
				"コメント投稿者に通知が届く（自分へのリアクションは通知なし）。"),
		mcp.WithString("comment_id",
			mcp.Required(),
			mcp.Description("Comment ID (short ID)"),
		),
		mcp.WithString("emoji",
			mcp.Required(),
			mcp.Description("Emoji character (e.g. '👍', '🎉', '❤️', '🚀')"),
		),
	)
}

func handleToggleReaction(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		commentID, err := client.ids.Resolve(argStr(args, "comment_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		emoji := argStr(args, "emoji")
		if emoji == "" {
			return errResult("emoji is required"), nil
		}

		body := map[string]any{
			"emoji": emoji,
		}

		_, err = client.Post(ctx, fmt.Sprintf("/api/v1/team/comments/%s/reactions", commentID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Reaction %s toggled.", emoji)), nil
	}
}

// ensureMentionsInContent appends @UUID references for any mentions
// not already present as @UUID in the content text.
// This ensures the backend can resolve mentions to display names on read.
func ensureMentionsInContent(content string, mentionUUIDs []string) string {
	if len(mentionUUIDs) == 0 {
		return content
	}

	lowerContent := strings.ToLower(content)
	var missing []string
	for _, uuid := range mentionUUIDs {
		if !strings.Contains(lowerContent, "@"+strings.ToLower(uuid)) {
			missing = append(missing, uuid)
		}
	}

	if len(missing) == 0 {
		return content
	}

	var sb strings.Builder
	sb.WriteString(content)
	sb.WriteString(" ")
	for i, uuid := range missing {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteByte('@')
		sb.WriteString(uuid)
	}
	return sb.String()
}

// resolveMentionIDs resolves comma-separated short IDs to full UUIDs.
func resolveMentionIDs(mentionsStr string, ids *ShortIDCache) ([]string, error) {
	if mentionsStr == "" {
		return nil, nil
	}
	var uuids []string
	for _, raw := range strings.Split(mentionsStr, ",") {
		shortID := strings.TrimSpace(raw)
		if shortID == "" {
			continue
		}
		fullID, err := ids.Resolve(shortID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mention %q: %v", shortID, err)
		}
		uuids = append(uuids, fullID)
	}
	return uuids, nil
}

func listMyCommentsTool() mcp.Tool {
	return mcp.NewTool("list_my_comments",
		mcp.WithDescription("自分が投稿したコメント一覧を取得する。どのGoalに何を書いたか振り返るのに便利。"+
			"resolvedフィルタで解決済み/未解決のみ取得可能。"),
		mcp.WithNumber("limit",
			mcp.Description("Max number of comments to return (default: 50, max: 100)"),
		),
		mcp.WithString("resolved",
			mcp.Description("Filter by resolved status: 'true' = resolved only, 'false' = unresolved only, omit = all"),
		),
	)
}

func handleListMyComments(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		memberID := client.MemberID()
		if memberID == "" {
			return errResult("member ID not resolved: use switch_organization first"), nil
		}

		args := req.GetArguments()
		limit := 50
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
			if limit > 100 {
				limit = 100
			}
		}

		path := fmt.Sprintf("/api/v1/team/comments?author_id=%s&limit=%d&sort=desc", memberID, limit)
		if v := argStr(args, "resolved"); v != "" {
			if v != "true" && v != "false" {
				return errResult(fmt.Sprintf("invalid resolved value %q: must be 'true' or 'false'", v)), nil
			}
			path += "&resolved=" + v
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		comments, err := parseComments(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(comments) == 0 {
			return textResult("No comments found."), nil
		}
		return textResult(formatComments(comments)), nil
	}
}

func listCommentsTool() mcp.Tool {
	return mcp.NewTool("list_comments",
		mcp.WithDescription(
			"Goalのコメント一覧を取得する。Goalに紐づく議論・経緯・意思決定の履歴を確認できる。"+
				"スレッド返信も含む。メンションは@名前で表示される。"+
				"resolvedフィルタで解決済み/未解決のみ取得可能。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("resolved",
			mcp.Description("Filter by resolved status: 'true' = resolved only, 'false' = unresolved only, omit = all"),
		),
	)
}

func handleListComments(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		commentPath := fmt.Sprintf("/api/v2/objectives/%s/comments", goalID)
		if v := argStr(args, "resolved"); v != "" {
			if v != "true" && v != "false" {
				return errResult(fmt.Sprintf("invalid resolved value %q: must be 'true' or 'false'", v)), nil
			}
			commentPath += "?resolved=" + v
		}

		data, err := client.Get(ctx, commentPath)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		comments, err := parseComments(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(comments) == 0 {
			return textResult("No comments."), nil
		}
		return textResult(formatComments(comments)), nil
	}
}
