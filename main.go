package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

const serverInstructions = `Addness はチームの目標・タスク・コンテキストを一元管理するワークスペースです。
以下の原則に従ってください。

1. ゴールはタイトル名で呼ぶ — ユーザーへの出力ではゴールのタイトル名を使い、IDは補助情報として扱う。
2. AI署名 — AIエージェントがコメントを投稿する場合、末尾に署名（例: "Claude Codeより"）を付けて人間のコメントと区別する。
3. CANCELLED = 一時停止 — ステータス CANCELLED は「中止」ではなく「一時停止（paused）」を意味する。親がCANCELLEDでも配下を勝手に移動・削除しないこと。
4. DoDの確認 — Definition of Done（完了基準）が空のゴールに取り組む前に、オーナーとDoDを擦り合わせることを推奨する。
5. Addnessが真実源 — タスク・プロジェクト・進捗の情報はAddnessに集約する。ローカルファイルや外部ツールではなく、Addnessのゴール・コメントに記録すること。`

func main() {
	// Subcommands
	if len(os.Args) > 1 && os.Args[1] == "version" || len(os.Args) > 1 && os.Args[1] == "--version" || len(os.Args) > 1 && os.Args[1] == "-v" {
		fmt.Println("addness-mcp " + version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "login" {
		if err := runLogin(); err != nil {
			log.Fatalf("login failed: %v", err)
		}
		return
	}

	baseURL := os.Getenv("ADDNESS_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	ids := NewShortIDCache()
	client := NewAddnessClient(baseURL, ids)

	// Pre-set token from env if available
	if token := os.Getenv("ADDNESS_API_TOKEN"); token != "" {
		client.SetToken(token)
	}

	s := server.NewMCPServer(
		"addness",
		version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	// Auth
	s.AddTool(authLoginTool(), handleAuthLogin(client))

	// Organization
	s.AddTool(listOrganizationsTool(), handleListOrganizations(client))
	s.AddTool(switchOrganizationTool(), handleSwitchOrganization(client))
	s.AddTool(listMembersTool(), handleListMembers(client))

	// Notifications
	s.AddTool(listNotificationsTool(), handleListNotifications(client))
	s.AddTool(markNotificationsReadTool(), handleMarkNotificationsRead(client))

	// Goals
	s.AddTool(listMyGoalsTool(), handleListMyGoals(client))
	s.AddTool(getGoalTool(), handleGetGoal(client))
	s.AddTool(getGoalAncestorsTool(), handleGetGoalAncestors(client))
	s.AddTool(updateGoalTool(), handleUpdateGoal(client))
	s.AddTool(completeGoalTool(), handleCompleteGoal(client))
	s.AddTool(createGoalTool(), handleCreateGoal(client))
	s.AddTool(moveGoalTool(), handleMoveGoal(client))
	s.AddTool(reorderGoalTool(), handleReorderGoal(client))
	s.AddTool(listMemberGoalsTool(), handleListMemberGoals(client))
	s.AddTool(deleteGoalTool(), handleDeleteGoal(client))

	// Search, Subgoals & Archive
	s.AddTool(listSubgoalsTool(), handleListSubgoals(client))
	s.AddTool(searchGoalsTool(), handleSearchGoals(client))
	s.AddTool(archiveGoalTool(), handleArchiveGoal(client))
	s.AddTool(unarchiveGoalTool(), handleUnarchiveGoal(client))

	// Comments
	s.AddTool(listCommentsTool(), handleListComments(client))
	s.AddTool(addCommentTool(), handleAddComment(client))
	s.AddTool(updateCommentTool(), handleUpdateComment(client))
	s.AddTool(deleteCommentTool(), handleDeleteComment(client))
	s.AddTool(resolveCommentTool(), handleResolveComment(client))
	s.AddTool(toggleReactionTool(), handleToggleReaction(client))

	// Assignments
	s.AddTool(assignMemberTool(), handleAssignMember(client))
	s.AddTool(unassignMemberTool(), handleUnassignMember(client))
	s.AddTool(listAssignmentsTool(), handleListAssignments(client))

	// Invitations
	s.AddTool(inviteMembersTool(), handleInviteMembers(client))
	s.AddTool(createInviteLinkTool(), handleCreateInviteLink(client))
	s.AddTool(listInviteLinksTool(), handleListInviteLinks(client))
	s.AddTool(listInvitedMembersTool(), handleListInvitedMembers(client))

	// Recurring Goals
	s.AddTool(setRecurringTool(), handleSetRecurring(client))
	s.AddTool(removeRecurringTool(), handleRemoveRecurring(client))
	s.AddTool(getRecurringTool(), handleGetRecurring(client))

	// Today's Goals & History
	s.AddTool(listTodaysGoalsTool(), handleListTodaysGoals(client))
	s.AddTool(getGoalHistoryTool(), handleGetGoalHistory(client))

	// Activity Logs
	s.AddTool(getMemberActivityTool(), handleGetMemberActivity(client))
	s.AddTool(getGoalActivityTool(), handleGetGoalActivity(client))
	s.AddTool(getActivitySummaryTool(), handleGetActivitySummary(client))

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
