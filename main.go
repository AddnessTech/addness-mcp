package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Subcommands
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
		"1.0.0",
		server.WithToolCapabilities(true),
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
