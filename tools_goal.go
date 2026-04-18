package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func listMyGoalsTool() mcp.Tool {
	return mcp.NewTool("list_my_goals",
		mcp.WithDescription("自分がアサインされているGoal一覧を取得する。階層パス・子Goal付きで現在の担当状況を俯瞰できる（デフォルト: 未完了のみ）"),
		mcp.WithBoolean("include_completed",
			mcp.Description("true to include completed/cancelled goals (default: false)"),
		),
		mcp.WithString("since",
			mcp.Description("Only show goals created within this period (e.g. '7d', '30d', '3m', '1y')"),
		),
	)
}

type myGoalWithContext struct {
	goal      goalInfo
	ancestors []ancestorInfo
	children  []goalChildInfo
}

func handleListMyGoals(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		includeCompleted, _ := args["include_completed"].(bool)
		since := argStr(args, "since")

		var cutoff time.Time
		if since != "" {
			d, ok := parseSince(since)
			if !ok {
				return errResult(fmt.Sprintf("invalid since format: %q (use e.g. '7d', '30d', '3m', '1y')", since)), nil
			}
			// Truncate to date boundary to avoid off-by-one from time-of-day
			cutoff = time.Now().Add(-d).Truncate(24 * time.Hour)
		}

		memberID := client.MemberID()
		if memberID == "" {
			return errResult("member ID not resolved: use switch_organization first"), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/members/%s/objectives", memberID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goals, err := parseGoalList(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		goals = filterGoals(goals, includeCompleted, cutoff)

		if len(goals) == 0 {
			return textResult("No goals assigned to you."), nil
		}

		results := fetchGoalContexts(ctx, client, goals)
		if !includeCompleted {
			var hidden int
			results, hidden = filterImplicitlyCompleted(results)
			if len(results) == 0 {
				return textResult(fmt.Sprintf("No active goals assigned to you. (%d goals hidden under completed parents — use include_completed=true to see all.)", hidden)), nil
			}
		}
		return textResult(formatMyGoalsWithContext(results)), nil
	}
}

func getGoalTool() mcp.Tool {
	return mcp.NewTool("get_goal",
		mcp.WithDescription("Goalの詳細を取得する。子Goal一覧・アサインメンバー・ステータス・期限など全情報を含む。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleGetGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		// Fetch goal detail, children, and assignments in parallel
		var (
			goalData, childrenData, assignData []byte
			errGoal, errChildren               error
			wg                                 sync.WaitGroup
		)
		wg.Add(3)
		go func() {
			defer wg.Done()
			goalData, errGoal = client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID))
		}()
		go func() {
			defer wg.Done()
			childrenData, errChildren = client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/children", goalID))
		}()
		go func() {
			defer wg.Done()
			assignData, _ = client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/assignments", goalID))
		}()
		wg.Wait()

		if errGoal != nil {
			return errResult(fmt.Sprintf("failed to get goal: %v", errGoal)), nil
		}
		if errChildren != nil {
			return errResult(fmt.Sprintf("failed to get children: %v", errChildren)), nil
		}

		goal, err := parseGoalDetail(goalData, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		goal.Members = parseGoalMembers(assignData)

		children, err := parseGoalChildren(childrenData, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse children error: %v", err)), nil
		}

		result := formatGoalDetail(goal)
		if len(children) > 0 {
			result += "\n\n## Children\n" + formatGoalChildList(children)
		}
		return textResult(result), nil
	}
}

func getGoalAncestorsTool() mcp.Tool {
	return mcp.NewTool("get_goal_ancestors",
		mcp.WithDescription("Goalの祖先チェーンを取得する。ルートから対象Goalまでの階層パスで、組織目標との位置づけを確認できる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleGetGoalAncestors(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/ancestors?includeOwner=true", goalID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		ancestors, current, err := parseAncestorsWithCurrent(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(ancestors) == 0 && current == nil {
			return textResult("This is a root goal (no ancestors)."), nil
		}

		// Include the current goal at the end of the chain
		if current != nil {
			ancestors = append(ancestors, *current)
		}

		return textResult(formatAncestors(ancestors)), nil
	}
}

func updateGoalTool() mcp.Tool {
	return mcp.NewTool("update_goal",
		mcp.WithDescription("Goalを編集する。タイトル・完了基準（DoD）・ステータス・期限を更新できる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("title",
			mcp.Description("New title (max 128 chars)"),
		),
		mcp.WithString("definition_of_done",
			mcp.Description("完了基準（DoD）。ゴールが達成された状態を記述する。"),
		),
		mcp.WithString("status",
			mcp.Description("New status"),
			mcp.Enum("NONE", "IN_PROGRESS", "COMPLETED", "CANCELLED"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date in YYYY-MM-DD format, or 'clear' to remove"),
		),
	)
}

func handleUpdateGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := make(map[string]any)
		if v := argStr(args, "title"); v != "" {
			body["title"] = v
		}
		if v, ok := args["definition_of_done"]; ok {
			body["description"] = v
		}
		if v := argStr(args, "status"); v != "" {
			body["status"] = v
		}
		if v := argStr(args, "due_date"); v != "" {
			if v == "clear" {
				body["clearDueDate"] = "clear"
			} else {
				body["dueDate"] = v + "T00:00:00Z"
			}
		}

		if len(body) == 0 {
			return errResult("at least one field to update is required"), nil
		}

		data, err := client.Patch(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goal, err := parseGoalDetail(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		return textResult("Goal updated.\n\n" + formatGoalDetail(goal)), nil
	}
}

func completeGoalTool() mcp.Tool {
	return mcp.NewTool("complete_goal",
		mcp.WithDescription("Goalを完了/未完了にする。undoで完了を取り消してIN_PROGRESSに戻すことも可能。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithBoolean("undo",
			mcp.Description("true to uncomplete (revert to IN_PROGRESS)"),
		),
	)
}

func handleCompleteGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		undo, _ := args["undo"].(bool)

		// Check if this goal has an execution record (recurring goal).
		// If so, complete/uncomplete the execution record instead of the objective.
		result, lookupErr := findTodaysExecution(ctx, client, goalID)
		if lookupErr != nil {
			return errResult(fmt.Sprintf("定常ゴール確認中にエラーが発生しました: %v", lookupErr)), nil
		}
		if result.execID != "" {
			return completeViaExecution(ctx, client, goalID, result.execID, undo)
		}
		// today's goals にノードが出ていた場合の recurring チェック
		if result.isRecurring {
			return errResult("この定常ゴールには今日の実行レコードがありません。実行ページを開いてから再度お試しください。"), nil
		}
		// today's goals API は execution record がない定常ゴールをスキップするため、
		// ノード自体が返ってこないケースがある（例: 週次ゴールの非対象日）。
		// goal detail の hasRecurring で追加チェックし、永久完了を防止する。
		if isRecurringGoal(ctx, client, goalID) {
			return errResult("この定常ゴールには今日の実行レコードがありません。実行ページを開いてから再度お試しください。"), nil
		}

		body := map[string]any{}
		if undo {
			body["completedAt"] = nil
		} else {
			body["completedAt"] = time.Now().UTC().Format(time.RFC3339)
		}

		data, err := client.Patch(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goal, err := parseGoalDetail(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		action := "completed"
		if undo {
			action = "uncompleted"
		}
		return textResult(fmt.Sprintf("Goal %s.\n\n%s", action, formatGoalDetail(goal))), nil
	}
}

type executionLookupResult struct {
	execID      string // non-empty if today's execution record exists
	isRecurring bool   // true if the goal has recurring settings (even without today's execution)
}

// findTodaysExecution looks up today's execution record and recurring status for the given objective.
// Uses the todays-goals API (single call) to determine both.
func findTodaysExecution(ctx context.Context, client *AddnessClient, objectiveID string) (executionLookupResult, error) {
	orgID := client.OrganizationID()
	if orgID == "" {
		return executionLookupResult{}, nil
	}

	today := time.Now().Format("2006-01-02")
	path := fmt.Sprintf("/api/v2/organizations/%s/todays-goals?date=%s", orgID, today)
	data, err := client.Get(ctx, path)
	if err != nil {
		return executionLookupResult{}, fmt.Errorf("todays-goals API: %w", err)
	}

	nodes, err := parseTodaysGoals(data, client.ids)
	if err != nil {
		return executionLookupResult{}, fmt.Errorf("parse todays-goals: %w", err)
	}

	for _, n := range nodes {
		fullNodeID, _ := client.ids.Resolve(n.ID)
		if fullNodeID != objectiveID {
			continue
		}
		result := executionLookupResult{isRecurring: n.HasRecurr}
		if n.ExecID != "" {
			resolved, _ := client.ids.Resolve(n.ExecID)
			result.execID = resolved
		}
		return result, nil
	}
	return executionLookupResult{}, nil
}

// isRecurringGoal checks if a goal has recurring settings via the goal detail API.
// Used as a fallback when the goal doesn't appear in today's goals
// (the todays-goals API skips recurring goals without execution records).
func isRecurringGoal(ctx context.Context, client *AddnessClient, goalID string) bool {
	data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID))
	if err != nil {
		return false
	}
	goal, err := parseGoalDetail(data, client.ids)
	if err != nil {
		return false
	}
	return goal.HasRecurring
}

func completeViaExecution(ctx context.Context, client *AddnessClient, goalID, execID string, undo bool) (*mcp.CallToolResult, error) {
	body := map[string]any{}
	if undo {
		body["completedAt"] = nil
	} else {
		body["completedAt"] = time.Now().UTC().Format(time.RFC3339)
	}

	_, err := client.Put(ctx, fmt.Sprintf("/api/v2/execute-goals/%s", execID), body)
	if err != nil {
		return errResult(fmt.Sprintf("failed to update execution record: %v", err)), nil
	}

	// Fetch updated goal detail for response.
	data, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID))
	if err != nil {
		return errResult(fmt.Sprintf("execution updated but failed to fetch goal: %v", err)), nil
	}

	goal, err := parseGoalDetail(data, client.ids)
	if err != nil {
		return errResult(fmt.Sprintf("parse error: %v", err)), nil
	}

	action := "completed (today's execution)"
	if undo {
		action = "uncompleted (today's execution)"
	}
	return textResult(fmt.Sprintf("Goal %s.\n\n%s", action, formatGoalDetail(goal))), nil
}

func createGoalTool() mcp.Tool {
	return mcp.NewTool("create_goal",
		mcp.WithDescription("新しいGoalを作成する。parent_idで親Goalの下に子Goalとして作成可能。recurringで定常（繰り返し）パターンも同時に設定できる。"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Goal title (max 128 chars)"),
		),
		mcp.WithString("definition_of_done",
			mcp.Description("完了基準（DoD）。ゴールが達成された状態を記述する。"),
		),
		mcp.WithString("parent_id",
			mcp.Description("Parent goal ID (short ID). Omit for root goal."),
		),
		mcp.WithString("status",
			mcp.Description("Initial status (default: NONE)"),
			mcp.Enum("NONE", "IN_PROGRESS"),
		),
		mcp.WithString("due_date",
			mcp.Description("Due date in YYYY-MM-DD format"),
		),
		mcp.WithString("recurring",
			mcp.Description("Set recurring pattern on creation (DAILY, WEEKLY, MONTHLY, WEEKDAYS). For advanced patterns, use set_recurring after creation."),
			mcp.Enum("DAILY", "WEEKLY", "MONTHLY", "WEEKDAYS"),
		),
	)
}

func handleCreateGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		title := argStr(args, "title")
		if title == "" {
			return errResult("title is required"), nil
		}

		body := map[string]any{
			"organizationId": client.OrganizationID(),
			"title":          title,
		}
		if v := argStr(args, "definition_of_done"); v != "" {
			body["description"] = v
		}
		if v := argStr(args, "parent_id"); v != "" {
			resolved, err := client.ids.Resolve(v)
			if err != nil {
				return errResult(err.Error()), nil
			}
			body["parentObjectiveId"] = resolved
		}
		if v := argStr(args, "status"); v != "" {
			body["status"] = v
		}
		if v := argStr(args, "due_date"); v != "" {
			body["dueDate"] = v + "T00:00:00Z"
		}

		data, err := client.Post(ctx, "/api/v2/objectives", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goal, err := parseGoalDetail(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		result := "Goal created.\n\n" + formatGoalDetail(goal)

		// Set recurring pattern if requested
		if pattern := argStr(args, "recurring"); pattern != "" {
			goalFullID, _ := client.ids.Resolve(goal.ID)
			recBody := map[string]any{"pattern": pattern}
			recData, recErr := client.Post(ctx, fmt.Sprintf("/api/v2/objectives/%s/recurring", goalFullID), recBody)
			if recErr != nil {
				result += fmt.Sprintf("\n⚠ Failed to set recurring pattern: %v", recErr)
			} else {
				rec := parseRecurringResponse(recData)
				result += "\n" + formatRecurringDetail(rec)
				goal.HasRecurring = true
			}
		}

		return textResult(result), nil
	}
}

func moveGoalTool() mcp.Tool {
	return mcp.NewTool("move_goal",
		mcp.WithDescription("Goalの親を変更して別のGoalの下に移動する。目標の階層構造を再編成できる。完了済みGoalは移動不可（先にcomplete_goalでundo=trueしてから移動すること）。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithString("new_parent_id",
			mcp.Required(),
			mcp.Description("New parent goal ID (short ID)"),
		),
	)
}

func handleMoveGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		newParentID, err := client.ids.Resolve(argStr(args, "new_parent_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		// Pre-check: warn if goal is completed (tree snapshot excludes completed goals)
		goalData, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID))
		if err != nil {
			return errResult(fmt.Sprintf("failed to get goal: %v", err)), nil
		}
		preCheck, err := parseGoalDetail(goalData, client.ids)
		if err == nil && preCheck.CompletedAt != "" {
			return errResult("Cannot move a completed goal. Use complete_goal with undo=true first, then move, then re-complete."), nil
		}

		body := map[string]any{
			"newParentId": newParentID,
		}

		data, err := client.Post(ctx, fmt.Sprintf("/api/v2/objectives/%s/parent", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goal, err := parseGoalDetail(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("move succeeded but failed to parse response: %v", err)), nil
		}
		return textResult("Goal moved.\n\n" + formatGoalDetail(goal)), nil
	}
}

func reorderGoalTool() mcp.Tool {
	return mcp.NewTool("reorder_goal",
		mcp.WithDescription("Goalの表示順を変更する。兄弟Goal間の優先度を並び替えできる。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithNumber("order_no",
			mcp.Required(),
			mcp.Description("New order number (lower = higher priority). Use values between existing goals to insert."),
		),
	)
}

func handleReorderGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		orderNo, ok := args["order_no"].(float64)
		if !ok {
			return errResult("order_no is required"), nil
		}

		body := map[string]any{
			"orderNo": orderNo,
		}

		_, err = client.Patch(ctx, fmt.Sprintf("/api/v2/objectives/%s", goalID), body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Goal reordered (orderNo: %g).", orderNo)), nil
	}
}

func listMemberGoalsTool() mcp.Tool {
	return mcp.NewTool("list_member_goals",
		mcp.WithDescription("指定メンバーのGoal一覧を取得する。階層パス・子Goal付きで、そのメンバーの担当範囲を把握できる。"),
		mcp.WithString("member_id",
			mcp.Required(),
			mcp.Description("Member ID (short ID)"),
		),
		mcp.WithBoolean("include_completed",
			mcp.Description("true to include completed/cancelled goals (default: false)"),
		),
		mcp.WithString("since",
			mcp.Description("Only show goals created within this period (e.g. '7d', '30d', '3m', '1y')"),
		),
	)
}

func handleListMemberGoals(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		memberID, err := client.ids.Resolve(argStr(args, "member_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}
		includeCompleted, _ := args["include_completed"].(bool)
		since := argStr(args, "since")

		var cutoff time.Time
		if since != "" {
			d, ok := parseSince(since)
			if !ok {
				return errResult(fmt.Sprintf("invalid since format: %q (use e.g. '7d', '30d', '3m', '1y')", since)), nil
			}
			cutoff = time.Now().Add(-d).Truncate(24 * time.Hour)
		}

		data, err := client.Get(ctx, fmt.Sprintf("/api/v2/members/%s/objectives", memberID))
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goals, err := parseGoalList(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		goals = filterGoals(goals, includeCompleted, cutoff)

		if len(goals) == 0 {
			return textResult("No goals found for this member."), nil
		}

		results := fetchGoalContexts(ctx, client, goals)
		if !includeCompleted {
			var hidden int
			results, hidden = filterImplicitlyCompleted(results)
			if len(results) == 0 {
				return textResult(fmt.Sprintf("No active goals found for this member. (%d goals hidden under completed parents — use include_completed=true to see all.)", hidden)), nil
			}
		}
		return textResult(formatMemberGoalsWithContext(results)), nil
	}
}

func deleteGoalTool() mcp.Tool {
	return mcp.NewTool("delete_goal",
		mcp.WithDescription("Goalを削除する。子Goalがある場合も含めて削除される。この操作は取り消せない。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleDeleteGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := map[string]any{
			"objectiveIds": []string{goalID},
		}

		_, err = client.Delete(ctx, "/api/v2/objectives/delete", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Goal deleted."), nil
	}
}

func listSubgoalsTool() mcp.Tool {
	return mcp.NewTool("list_subgoals",
		mcp.WithDescription("指定Goalの子孫を再帰的に取得する。特定Goalの配下を深掘りしたいときに使う。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
		mcp.WithNumber("max_depth",
			mcp.Description("Maximum depth to include (default: 0 = unlimited)"),
		),
		mcp.WithBoolean("include_completed",
			mcp.Description("true to include completed/cancelled goals (default: false)"),
		),
	)
}

func handleListSubgoals(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		path := fmt.Sprintf("/api/v2/objectives/%s/descendants?include_owner=true", goalID)
		if v, ok := args["max_depth"].(float64); ok {
			if v < 0 {
				return errResult("max_depth must be >= 0"), nil
			}
			if v > 0 {
				path += fmt.Sprintf("&max_depth=%d", int(v))
			}
		}

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		// descendants APIはV2なのでunwrapDataは不要（parseGoalListが内部でunwrapData済み）
		// ただしレスポンスがobjectラッパーの場合があるため、配列でなければunwrapする
		goals, err := parseDescendants(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		includeCompleted, _ := args["include_completed"].(bool)
		if !includeCompleted {
			goals = filterGoals(goals, false, time.Time{})
		}

		if len(goals) == 0 {
			return textResult("No subgoals found."), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Subgoals (%d):\n\n", len(goals))
		for _, g := range goals {
			fmt.Fprintf(&sb, "[%s] %s %s", g.ID, goalIcon(g), g.Title)
			if g.Owner != "" {
				fmt.Fprintf(&sb, " (@%s)", g.Owner)
			}
			if g.HasChildren {
				sb.WriteString(" [+]")
			}
			sb.WriteString("\n")
		}
		return textResult(sb.String()), nil
	}
}

func searchGoalsTool() mcp.Tool {
	return mcp.NewTool("search_goals",
		mcp.WithDescription("Goalをタイトルで検索する。部分一致でマッチしたGoal一覧を返す。"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query (matched against goal title)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max results (default: 20)"),
		),
	)
}

func handleSearchGoals(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := requireOrg(client); err != nil {
			return errResult(err.Error()), nil
		}

		args := req.GetArguments()
		query := argStr(args, "query")
		if query == "" {
			return errResult("query is required"), nil
		}

		limit := 20
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}

		path := fmt.Sprintf("/api/v1/team/objectives/search?title=%s&organizationId=%s&permission=read&limit=%d",
			url.QueryEscape(query), client.OrganizationID(), limit)

		data, err := client.Get(ctx, path)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		goals, err := parseSearchResults(data, client.ids)
		if err != nil {
			return errResult(fmt.Sprintf("parse error: %v", err)), nil
		}

		if len(goals) == 0 {
			return textResult(fmt.Sprintf("No goals found for query: %q", query)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Search results for %q (%d):\n\n", query, len(goals))
		for _, g := range goals {
			fmt.Fprintf(&sb, "[%s] %s %s", g.ID, goalIcon(g), g.Title)
			if g.Owner != "" {
				fmt.Fprintf(&sb, " (@%s)", g.Owner)
			}
			sb.WriteString("\n")
		}
		return textResult(sb.String()), nil
	}
}

func archiveGoalTool() mcp.Tool {
	return mcp.NewTool("archive_goal",
		mcp.WithDescription("Goalをアーカイブする。子Goal含めて一括アーカイブされる。アーカイブしたGoalはunarchive_goalで復元可能。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleArchiveGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := map[string]any{
			"objectiveIds": []string{goalID},
		}

		_, err = client.Post(ctx, "/api/v2/objectives/archive", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Goal archived."), nil
	}
}

func unarchiveGoalTool() mcp.Tool {
	return mcp.NewTool("unarchive_goal",
		mcp.WithDescription("アーカイブ済みGoalを復元する。"),
		mcp.WithString("goal_id",
			mcp.Required(),
			mcp.Description("Goal ID (short ID)"),
		),
	)
}

func handleUnarchiveGoal(client *AddnessClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		goalID, err := client.ids.Resolve(argStr(args, "goal_id"))
		if err != nil {
			return errResult(err.Error()), nil
		}

		body := map[string]any{
			"objectiveIds": []string{goalID},
		}

		_, err = client.Post(ctx, "/api/v2/objectives/unarchive", body)
		if err != nil {
			return errResult(fmt.Sprintf("failed: %v", err)), nil
		}

		return textResult("Goal unarchived."), nil
	}
}

// maxGoalConcurrency limits the number of concurrent API requests
// when fetching ancestors/children for multiple goals.
const maxGoalConcurrency = 10

// fetchGoalContexts fetches ancestors and children for each goal concurrently
// with bounded parallelism.
func fetchGoalContexts(ctx context.Context, client *AddnessClient, goals []goalInfo) []myGoalWithContext {
	results := make([]myGoalWithContext, len(goals))
	sem := make(chan struct{}, maxGoalConcurrency)
	var wg sync.WaitGroup

	for i, g := range goals {
		results[i].goal = g
		goalFullID := client.ids.resolveOrFallback(g.ID)

		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if d, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/ancestors", id)); err == nil {
				results[idx].ancestors, _ = parseAncestors(d, client.ids)
			}
		}(i, goalFullID)

		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if d, err := client.Get(ctx, fmt.Sprintf("/api/v2/objectives/%s/children", id)); err == nil {
				results[idx].children, _ = parseGoalChildren(d, client.ids)
			}
		}(i, goalFullID)
	}
	wg.Wait()
	return results
}
