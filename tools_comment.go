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
			"Goalにコメントを投稿する。Slackやメールで���なく、Goalに直接コメントする���とで："+
				"��脈がゴールに集約される／後から経緯を追える���通知もAddness内で完結／"+
				"「誰が・どのゴ��ルで・何を話してるか」が構造化される。"+
				"parent_idで��レッド返信も可能。mentionsでメンバーをメンションすると通知が届く。"+
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
		return textResult(result), nil
	}
}

func updateCommentTool() mcp.Tool {
	return mcp.NewTool("update_comment",
		mcp.WithDescription("コメントを編集する。自分が投稿したコメントのみ編集可能。メンションの変更も可能。"),
		mcp.WithString("comment_id",
			mcp.Required(),
			mcp.Description("Comment ID (short ID)"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("New comment text (max 10000 chars)"),
		),
		mcp.WithString("mentions",
			mcp.Description("Comma-separated member IDs (short ID) to @mention. Omit to keep existing mentions."),
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

func listCommentsTool() mcp.Tool {
	return mcp.NewTool("list_comments",
		mcp.WithDescription(
			"Goalのコメント一覧を取得する。Goalに紐づく議論・経緯・意思決定の履歴を確認できる。"+
				"スレッド返信も含む。メンションは@名前で表示される。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
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

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/comments", goalID))
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
