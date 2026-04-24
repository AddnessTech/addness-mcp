package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Generic helpers ---

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: text,
			},
		},
	}
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "Error: " + msg,
			},
		},
		IsError: true,
	}
}

func requireOrg(client *AddnessClient) error {
	if client.OrganizationID() == "" {
		return fmt.Errorf("no organization selected: use switch_organization first")
	}
	return nil
}

func argStr(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// unwrapData extracts the "data" field from V2 API responses.
// V2 responses are wrapped as {"data": ..., "message": "success"}.
func unwrapData(data []byte) []byte {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Data) > 0 {
		return wrapper.Data
	}
	return data
}

func splitAndResolve(csv string, ids *ShortIDCache) ([]string, error) {
	parts := strings.Split(csv, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			resolved, err := ids.Resolve(p)
			if err != nil {
				return nil, err
			}
			result = append(result, resolved)
		}
	}
	return result, nil
}

// --- Goal filtering ---

// filterGoals filters goals by completion status and creation period.
// cutoff should be computed by the caller; zero value means no date filter.
func filterGoals(goals []goalInfo, includeCompleted bool, cutoff time.Time) []goalInfo {
	filtered := make([]goalInfo, 0, len(goals))
	for _, g := range goals {
		if !includeCompleted && (g.CompletedAt != "" || g.Status == "CANCELLED") {
			continue
		}
		if !cutoff.IsZero() && g.CreatedAt != "" {
			if t, err := time.Parse("2006-01-02", g.CreatedAt); err == nil {
				if t.Before(cutoff) {
					continue
				}
			}
		}
		filtered = append(filtered, g)
	}
	return filtered
}

// filterImplicitlyCompleted removes goals whose ancestor chain contains a
// completed goal. When a parent is marked complete, its children are
// considered implicitly complete and should not appear as active goals.
// Returns (filtered goals, number of goals removed by this filter).
func filterImplicitlyCompleted(goals []myGoalWithContext) ([]myGoalWithContext, int) {
	filtered := make([]myGoalWithContext, 0, len(goals))
	for _, g := range goals {
		ancestorCompleted := false
		for _, a := range g.ancestors {
			if a.CompletedAt != "" || a.Status == "COMPLETED" {
				ancestorCompleted = true
				break
			}
		}
		if !ancestorCompleted {
			filtered = append(filtered, g)
		}
	}
	return filtered, len(goals) - len(filtered)
}

// parseSince parses duration strings like "7d", "30d", "3m", "1y".
func parseSince(s string) (time.Duration, bool) {
	if len(s) < 2 {
		return 0, false
	}
	unit := s[len(s)-1]
	num, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || num <= 0 {
		return 0, false
	}
	switch unit {
	case 'd':
		return time.Duration(num) * 24 * time.Hour, true
	case 'w':
		return time.Duration(num) * 7 * 24 * time.Hour, true
	case 'm':
		return time.Duration(num) * 30 * 24 * time.Hour, true
	case 'y':
		return time.Duration(num) * 365 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// --- Organization types & formatting ---

type orgInfo struct {
	ID     string
	fullID string
	Name   string
	Plan   string
}

func parseOrganizations(data []byte, ids *ShortIDCache) ([]orgInfo, error) {
	// V2 /organizations/me returns {"data": {"organizations": [...]}}
	var dataWrapper struct {
		Data struct {
			Organizations []map[string]any `json:"organizations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &dataWrapper); err == nil && len(dataWrapper.Data.Organizations) > 0 {
		return buildOrgInfos(dataWrapper.Data.Organizations, ids), nil
	}

	// Fallback: {"organizations": [...]}
	var wrapper struct {
		Organizations []map[string]any `json:"organizations"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	return buildOrgInfos(wrapper.Organizations, ids), nil
}

func buildOrgInfos(raw []map[string]any, ids *ShortIDCache) []orgInfo {
	orgs := make([]orgInfo, 0, len(raw))
	for _, o := range raw {
		fullID, _ := o["id"].(string)
		name, _ := o["name"].(string)
		plan, _ := o["planType"].(string)
		orgs = append(orgs, orgInfo{
			ID:     ids.Shorten(fullID),
			fullID: fullID,
			Name:   name,
			Plan:   plan,
		})
	}
	return orgs
}

func formatOrganizations(orgs []orgInfo) string {
	if len(orgs) == 0 {
		return "No organizations found."
	}
	var sb strings.Builder
	sb.WriteString("Organizations:\n")
	for _, o := range orgs {
		fmt.Fprintf(&sb, "  [%s] %s (plan: %s)\n", o.ID, o.Name, o.Plan)
	}
	return sb.String()
}

// --- Notification types & formatting ---

type notifInfo struct {
	ID        string
	Type      string
	Title     string
	Body      string
	IsRead    bool
	Count     int
	Actor     string
	CreatedAt string
}

func parseNotifications(data []byte, ids *ShortIDCache) ([]notifInfo, int64, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, 0, err
	}

	unreadCount, _ := raw["unreadCount"].(float64)

	rawNotifs, _ := raw["notifications"].([]any)
	notifs := make([]notifInfo, 0, len(rawNotifs))
	for _, n := range rawNotifs {
		nm, _ := n.(map[string]any)
		fullID, _ := nm["id"].(string)
		nType, _ := nm["eventType"].(string)
		title, _ := nm["subjectTitle"].(string)
		readAt, _ := nm["readAt"].(string)
		isRead := readAt != ""

		count := 1
		if nIDs, ok := nm["notificationIds"].([]any); ok && len(nIDs) > 0 {
			count = len(nIDs)
		}

		occurredAt, _ := nm["occurredAt"].(string)

		actor := ""
		if actors, ok := nm["actors"].([]any); ok && len(actors) > 0 {
			if a, ok := actors[0].(map[string]any); ok {
				actor, _ = a["name"].(string)
			}
		}

		// Extract enrichment from metadata
		body := ""
		if metadata, ok := nm["metadata"].(map[string]any); ok {
			body = buildNotificationBody(nType, metadata)
		}

		notifs = append(notifs, notifInfo{
			ID:        ids.Shorten(fullID),
			Type:      nType,
			Title:     title,
			Body:      body,
			IsRead:    isRead,
			Count:     count,
			Actor:     actor,
			CreatedAt: formatTime(occurredAt),
		})
	}
	return notifs, int64(unreadCount), nil
}

// buildNotificationBody extracts a human-readable body from notification metadata.
func buildNotificationBody(eventType string, metadata map[string]any) string {
	switch {
	case strings.Contains(eventType, "comment.reaction"):
		return buildReactionBody(metadata)
	case strings.Contains(eventType, "comment"):
		return buildCommentBody(metadata)
	case strings.Contains(eventType, "updated"):
		return buildUpdatedBody(metadata)
	case strings.Contains(eventType, "assignment"):
		return buildAssignmentBody(metadata)
	case strings.Contains(eventType, "lifecycle"):
		return buildLifecycleBody(metadata)
	default:
		if cp, ok := metadata["content_preview"].(string); ok && cp != "" {
			return cp
		}
		return ""
	}
}

func buildCommentBody(metadata map[string]any) string {
	if cp, ok := metadata["content_preview"].(string); ok && cp != "" {
		return cp
	}
	return ""
}

func buildReactionBody(metadata map[string]any) string {
	var parts []string
	if rc, ok := metadata["reaction_counts"].(map[string]any); ok {
		for emoji, count := range rc {
			if c, ok := count.(float64); ok && c > 0 {
				parts = append(parts, fmt.Sprintf("%s×%.0f", emoji, c))
			}
		}
		sort.Strings(parts)
	}
	prefix := strings.Join(parts, " ")
	if ccp, ok := metadata["comment_content_preview"].(string); ok && ccp != "" {
		if prefix != "" {
			return prefix + " → " + ccp
		}
		return "→ " + ccp
	}
	return prefix
}

func buildUpdatedBody(metadata map[string]any) string {
	diffs, ok := metadata["field_diffs"].([]any)
	if !ok || len(diffs) == 0 {
		return ""
	}
	var lines []string
	for _, d := range diffs {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		field, _ := dm["field"].(string)
		oldVal, _ := dm["old_value"].(string)
		newVal, _ := dm["new_value"].(string)
		if field != "" {
			if oldVal == "" {
				oldVal = "(empty)"
			}
			if newVal == "" {
				newVal = "(empty)"
			}
			lines = append(lines, fmt.Sprintf("%s: %s → %s", field, oldVal, newVal))
		}
	}
	return strings.Join(lines, ", ")
}

func buildAssignmentBody(metadata map[string]any) string {
	if role, ok := metadata["assignment_role"].(string); ok && role != "" {
		return "role: " + role
	}
	return ""
}

func buildLifecycleBody(metadata map[string]any) string {
	resources, ok := metadata["resources"].([]any)
	if !ok || len(resources) == 0 {
		return ""
	}
	var lines []string
	for _, r := range resources {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		title, _ := rm["title"].(string)
		op, _ := rm["operation"].(string)
		if title != "" && op != "" {
			lines = append(lines, fmt.Sprintf("%s (%s)", title, op))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, ", ")
}

func formatNotifications(notifs []notifInfo) string {
	var sb strings.Builder
	for _, n := range notifs {
		readMark := "●"
		if n.IsRead {
			readMark = "○"
		}
		line := fmt.Sprintf("%s [%s] %s: %s", readMark, n.ID, n.Type, n.Title)
		if n.Actor != "" {
			line += fmt.Sprintf(" (by %s)", n.Actor)
		}
		if n.Count > 1 {
			line += fmt.Sprintf(" (+%d)", n.Count-1)
		}
		if n.Body != "" {
			line += "\n    " + truncate(n.Body, 80)
		}
		line += "\n    " + n.CreatedAt
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// --- Goal types & formatting ---

type goalInfo struct {
	ID            string
	fullID        string
	Title         string
	Status        string
	Description   string
	DoD           string
	Owner         string
	Members       []string
	DueDate       string
	CompletedAt   string
	ParentID      string
	HasChildren   bool
	ChildCount    int
	CreatedAt     string
	HasRecurring  bool
	RecurringDesc string
}

type goalChildInfo struct {
	ID           string
	Title        string
	Status       string
	CompletedAt  string
	HasChildren  bool
	Owner        string
	HasRecurring bool
}

type ancestorInfo struct {
	ID          string
	Title       string
	Status      string
	CompletedAt string
	Depth       int
	Owner       string
}

func parseGoalList(data []byte, ids *ShortIDCache) ([]goalInfo, error) {
	var raw []map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return nil, err
	}

	goals := make([]goalInfo, 0, len(raw))
	for _, g := range raw {
		goals = append(goals, extractGoalInfo(g, ids))
	}
	return goals, nil
}

func formatMyGoalsWithContext(goals []myGoalWithContext) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "My Goals (%d):\n", len(goals))
	for _, g := range goals {
		fmt.Fprintf(&sb, "\n[%s] %s %s", g.goal.ID, goalIcon(g.goal), g.goal.Title)
		if g.goal.HasRecurring {
			sb.WriteString(" [recurring]")
		}
		if g.goal.DueDate != "" {
			fmt.Fprintf(&sb, " due:%s", g.goal.DueDate)
		}
		sb.WriteString("\n")

		// Ancestor path
		if len(g.ancestors) > 0 {
			names := make([]string, len(g.ancestors))
			for i, a := range g.ancestors {
				names[i] = a.Title
			}
			fmt.Fprintf(&sb, "  path: %s\n", strings.Join(names, " > "))
		}

		// Children
		for _, c := range g.children {
			icon := statusIcon(c.Status)
			if c.CompletedAt != "" {
				icon = "[x]"
			}
			fmt.Fprintf(&sb, "  └ [%s] %s %s", c.ID, icon, c.Title)
			if c.HasRecurring {
				sb.WriteString(" [recurring]")
			}
			if c.HasChildren {
				sb.WriteString(" [+]")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func parseGoalDetail(data []byte, ids *ShortIDCache) (goalInfo, error) {
	var raw map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return goalInfo{}, err
	}
	return extractGoalInfo(raw, ids), nil
}

func parseGoalChildren(data []byte, ids *ShortIDCache) ([]goalChildInfo, error) {
	var wrapper struct {
		Children []map[string]any `json:"children"`
	}
	if err := json.Unmarshal(unwrapData(data), &wrapper); err != nil {
		return nil, err
	}
	raw := wrapper.Children

	children := make([]goalChildInfo, 0, len(raw))
	for _, c := range raw {
		fullID, _ := c["id"].(string)
		title, _ := c["title"].(string)
		status, _ := c["status"].(string)
		completedAt, _ := c["completedAt"].(string)
		hasChildren, _ := c["hasChildren"].(bool)
		hasRecurring, _ := c["hasRecurring"].(bool)
		owner := extractOwnerName(c)

		children = append(children, goalChildInfo{
			ID:           ids.Shorten(fullID),
			Title:        title,
			Status:       status,
			CompletedAt:  formatTime(completedAt),
			HasChildren:  hasChildren,
			Owner:        owner,
			HasRecurring: hasRecurring,
		})
	}
	return children, nil
}

func parseAncestors(data []byte, ids *ShortIDCache) ([]ancestorInfo, error) {
	all, _, err := parseAncestorsWithCurrent(data, ids)
	return all, err
}

func parseAncestorsWithCurrent(data []byte, ids *ShortIDCache) (ancestors []ancestorInfo, current *ancestorInfo, err error) {
	var wrapper struct {
		Ancestors []map[string]any `json:"ancestors"`
		Current   map[string]any   `json:"current"`
	}
	if err := json.Unmarshal(unwrapData(data), &wrapper); err != nil {
		return nil, nil, err
	}

	ancestors = make([]ancestorInfo, 0, len(wrapper.Ancestors))
	for i, a := range wrapper.Ancestors {
		ancestors = append(ancestors, extractAncestorInfo(a, i, ids))
	}

	if wrapper.Current != nil {
		c := extractAncestorInfo(wrapper.Current, len(wrapper.Ancestors), ids)
		current = &c
	}

	return ancestors, current, nil
}

func extractAncestorInfo(a map[string]any, depth int, ids *ShortIDCache) ancestorInfo {
	fullID, _ := a["id"].(string)
	title, _ := a["title"].(string)
	status, _ := a["status"].(string)
	completedAt, _ := a["completedAt"].(string)
	owner := extractOwnerName(a)

	return ancestorInfo{
		ID:          ids.Shorten(fullID),
		Title:       title,
		Status:      status,
		CompletedAt: formatTime(completedAt),
		Depth:       depth,
		Owner:       owner,
	}
}

func extractGoalInfo(g map[string]any, ids *ShortIDCache) goalInfo {
	fullID, _ := g["id"].(string)
	title, _ := g["title"].(string)
	status, _ := g["status"].(string)
	desc, _ := g["description"].(string)
	dod, _ := g["definitionOfDone"].(string)
	dueDate, _ := g["dueDate"].(string)
	completedAt, _ := g["completedAt"].(string)
	createdAt, _ := g["createdAt"].(string)
	hasChildren, _ := g["hasChildren"].(bool)
	parentID, _ := g["parentId"].(string)

	totalChildren := 0
	if tc, ok := g["totalChildrenCount"].(float64); ok {
		totalChildren = int(tc)
	}

	owner := extractOwnerName(g)

	hasRecurring, _ := g["hasRecurring"].(bool)
	recurringDesc := ""
	if rec, ok := g["recurring"].(map[string]any); ok {
		recurringDesc, _ = rec["description"].(string)
	}

	return goalInfo{
		ID:            ids.Shorten(fullID),
		fullID:        fullID,
		Title:         title,
		Status:        status,
		Description:   desc,
		DoD:           dod,
		Owner:         owner,
		DueDate:       formatTime(dueDate),
		CompletedAt:   formatTime(completedAt),
		ParentID:      ids.ShortenOptional(strPtr(parentID)),
		HasChildren:   hasChildren,
		ChildCount:    totalChildren,
		CreatedAt:     formatTime(createdAt),
		HasRecurring:  hasRecurring,
		RecurringDesc: recurringDesc,
	}
}

func extractOwnerName(m map[string]any) string {
	if owner, ok := m["owner"].(map[string]any); ok {
		name, _ := owner["name"].(string)
		return name
	}
	return ""
}

func parseGoalMembers(data []byte) []string {
	var wrapper struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(unwrapData(data), &wrapper); err != nil {
		return nil
	}
	var members []string
	for _, a := range wrapper.Assignments {
		// Skip owner — already shown separately
		if role, ok := a["role"].(map[string]any); ok {
			if name, _ := role["name"].(string); name == "OWNER" {
				continue
			}
		}
		if member, ok := a["member"].(map[string]any); ok {
			if name, _ := member["name"].(string); name != "" {
				members = append(members, name)
			}
		}
	}
	return members
}

func formatMemberGoalsWithContext(goals []myGoalWithContext) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Member Goals (%d):\n", len(goals))
	for _, g := range goals {
		fmt.Fprintf(&sb, "\n[%s] %s %s", g.goal.ID, goalIcon(g.goal), g.goal.Title)
		if g.goal.HasRecurring {
			sb.WriteString(" [recurring]")
		}
		if g.goal.Owner != "" {
			fmt.Fprintf(&sb, " (@%s)", g.goal.Owner)
		}
		if g.goal.DueDate != "" {
			fmt.Fprintf(&sb, " due:%s", g.goal.DueDate)
		}
		sb.WriteString("\n")

		if len(g.ancestors) > 0 {
			names := make([]string, len(g.ancestors))
			for i, a := range g.ancestors {
				names[i] = a.Title
			}
			fmt.Fprintf(&sb, "  path: %s\n", strings.Join(names, " > "))
		}

		for _, c := range g.children {
			icon := statusIcon(c.Status)
			if c.CompletedAt != "" {
				icon = "[x]"
			}
			fmt.Fprintf(&sb, "  └ [%s] %s %s", c.ID, icon, c.Title)
			if c.HasRecurring {
				sb.WriteString(" [recurring]")
			}
			if c.HasChildren {
				sb.WriteString(" [+]")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func parseDescendants(data []byte, ids *ShortIDCache) ([]goalInfo, error) {
	unwrapped := unwrapData(data)
	// Try array first
	var raw []map[string]any
	if err := json.Unmarshal(unwrapped, &raw); err == nil {
		goals := make([]goalInfo, 0, len(raw))
		for _, g := range raw {
			goals = append(goals, extractGoalInfo(g, ids))
		}
		return goals, nil
	}
	// Try object with descendants key
	var wrapper struct {
		Descendants []map[string]any `json:"descendants"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return nil, err
	}
	goals := make([]goalInfo, 0, len(wrapper.Descendants))
	for _, g := range wrapper.Descendants {
		goals = append(goals, extractGoalInfo(g, ids))
	}
	return goals, nil
}

func parseSearchResults(data []byte, ids *ShortIDCache) ([]goalInfo, error) {
	var wrapper struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(unwrapData(data), &wrapper); err != nil {
		return nil, err
	}
	goals := make([]goalInfo, 0, len(wrapper.Items))
	for _, g := range wrapper.Items {
		goals = append(goals, extractGoalInfo(g, ids))
	}
	return goals, nil
}

func formatGoalDetail(g goalInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s %s\n", goalIcon(g), g.Title)
	displayStatus := g.Status
	if g.Status == "CANCELLED" {
		displayStatus = "PAUSED"
	}
	fmt.Fprintf(&sb, "ID: %s | Status: %s", g.ID, displayStatus)
	if g.Owner != "" {
		fmt.Fprintf(&sb, " | Owner: %s", g.Owner)
	}
	sb.WriteString("\n")
	if g.fullID != "" {
		fmt.Fprintf(&sb, "URL: %s/goals/%s\n", frontendBaseURL, g.fullID)
	}
	if g.HasRecurring {
		if g.RecurringDesc != "" {
			fmt.Fprintf(&sb, "Recurring: %s\n", g.RecurringDesc)
		} else {
			sb.WriteString("Recurring: yes\n")
		}
	}
	if len(g.Members) > 0 {
		fmt.Fprintf(&sb, "Members: %s\n", strings.Join(g.Members, " / "))
	}
	if g.ParentID != "" {
		fmt.Fprintf(&sb, "Parent: %s\n", g.ParentID)
	}
	if g.DueDate != "" {
		fmt.Fprintf(&sb, "Due: %s\n", g.DueDate)
	}
	if g.CompletedAt != "" {
		fmt.Fprintf(&sb, "Completed: %s\n", g.CompletedAt)
	}
	if g.Description != "" {
		fmt.Fprintf(&sb, "\n説明（現在の状態）:\n%s\n", g.Description)
	}
	if g.DoD != "" {
		fmt.Fprintf(&sb, "\n完了基準（理想の状態）:\n%s\n", g.DoD)
	} else {
		sb.WriteString("\n⚠ 完了基準が未設定です。作業開始前にオーナーと理想の状態を擦り合わせてください。\n")
	}
	return sb.String()
}

func formatGoalChildList(children []goalChildInfo) string {
	var sb strings.Builder
	for _, c := range children {
		line := fmt.Sprintf("  [%s] %s %s", c.ID, statusIcon(c.Status), c.Title)
		if c.HasRecurring {
			line += " [recurring]"
		}
		if c.Owner != "" {
			line += fmt.Sprintf(" (@%s)", c.Owner)
		}
		if c.HasChildren {
			line += " [+]"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func formatAncestors(ancestors []ancestorInfo) string {
	var sb strings.Builder
	sb.WriteString("Goal Hierarchy (root → target):\n")
	for _, a := range ancestors {
		indent := strings.Repeat("  ", a.Depth)
		icon := statusIcon(a.Status)
		if a.CompletedAt != "" {
			icon = "[x]"
		}
		line := fmt.Sprintf("%s[%s] %s %s", indent, a.ID, icon, a.Title)
		if a.Owner != "" {
			line += fmt.Sprintf(" (@%s)", a.Owner)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// --- Today's goals types & formatting ---

type todaysGoalNode struct {
	ID          string
	ParentID    string
	Depth       int
	Title       string
	Status      string
	IsLeaf      bool
	HasRecurr   bool
	ExecID      string
	ExecStatus  string
	CompletedAt string
}

func parseTodaysGoals(data []byte, ids *ShortIDCache) ([]todaysGoalNode, error) {
	var raw map[string]any
	if err := json.Unmarshal(unwrapData(data), &raw); err != nil {
		return nil, err
	}

	rawNodes, _ := raw["nodes"].([]any)
	nodes := make([]todaysGoalNode, 0, len(rawNodes))
	for _, n := range rawNodes {
		nm, _ := n.(map[string]any)
		fullID, _ := nm["id"].(string)
		parentID, _ := nm["parentId"].(string)
		depth := 0
		if d, ok := nm["depth"].(float64); ok {
			depth = int(d)
		}
		title, _ := nm["title"].(string)
		status, _ := nm["status"].(string)
		isLeaf, _ := nm["isLeaf"].(bool)
		hasRecurr, _ := nm["hasRecurring"].(bool)

		node := todaysGoalNode{
			ID:        ids.Shorten(fullID),
			ParentID:  ids.ShortenOptional(strPtr(parentID)),
			Depth:     depth,
			Title:     title,
			Status:    status,
			IsLeaf:    isLeaf,
			HasRecurr: hasRecurr,
		}

		// Check objective-level completedAt first
		if ca, ok := nm["completedAt"].(string); ok && ca != "" {
			node.CompletedAt = formatTime(ca)
		}

		if exec, ok := nm["execution"].(map[string]any); ok {
			execID, _ := exec["id"].(string)
			node.ExecID = ids.Shorten(execID)
			execStatus, _ := exec["status"].(string)
			node.ExecStatus = execStatus
			if node.CompletedAt == "" {
				if ca, ok := exec["completedAt"].(string); ok && ca != "" {
					node.CompletedAt = formatTime(ca)
				}
			}
		}

		nodes = append(nodes, node)
	}
	return nodes, nil
}

func formatTodaysGoals(nodes []todaysGoalNode) string {
	var sb strings.Builder
	for _, n := range nodes {
		indent := strings.Repeat("  ", n.Depth)
		check := "[ ]"
		if n.CompletedAt != "" {
			check = "[x]"
		}

		line := fmt.Sprintf("%s%s %s", indent, check, n.Title)
		if n.ExecID != "" {
			line += fmt.Sprintf(" (exec:%s)", n.ExecID)
		} else {
			line += fmt.Sprintf(" (goal:%s)", n.ID)
		}
		if n.HasRecurr {
			line += " [recurring]"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// --- Member helpers ---

type memberInfo struct {
	ID            string
	Name          string
	IsCurrentUser bool
}

type pagingInfo struct {
	Page       int
	PageSize   int
	TotalCount int
	TotalPages int
}

func parseMembers(data []byte, ids *ShortIDCache) ([]memberInfo, pagingInfo, error) {
	unwrapped := unwrapData(data)
	var wrapper struct {
		Members []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			IsCurrentUser bool   `json:"isCurrentUser"`
		} `json:"members"`
		TotalCount int `json:"totalCount"`
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		TotalPages int `json:"totalPages"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return nil, pagingInfo{}, err
	}
	members := make([]memberInfo, 0, len(wrapper.Members))
	for _, m := range wrapper.Members {
		members = append(members, memberInfo{
			ID:            ids.Shorten(m.ID),
			Name:          m.Name,
			IsCurrentUser: m.IsCurrentUser,
		})
	}
	paging := pagingInfo{
		Page:       wrapper.Page,
		PageSize:   wrapper.PageSize,
		TotalCount: wrapper.TotalCount,
		TotalPages: wrapper.TotalPages,
	}
	return members, paging, nil
}

func findCurrentMemberID(data []byte) string {
	unwrapped := unwrapData(data)
	var wrapper struct {
		Members []struct {
			ID            string `json:"id"`
			IsCurrentUser bool   `json:"isCurrentUser"`
		} `json:"members"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return ""
	}
	for _, m := range wrapper.Members {
		if m.IsCurrentUser {
			return m.ID
		}
	}
	return ""
}

func formatMembers(members []memberInfo) string {
	var sb strings.Builder
	for _, m := range members {
		marker := "  "
		if m.IsCurrentUser {
			marker = "→ "
		}
		fmt.Fprintf(&sb, "%s[%s] %s\n", marker, m.ID, m.Name)
	}
	return sb.String()
}

// --- Comment types & formatting ---

type commentInfo struct {
	ID         string
	Content    string
	Author     string
	ParentID   string
	CreatedAt  string
	ResolvedAt string
	Resolver   string
}

func parseComments(data []byte, ids *ShortIDCache) ([]commentInfo, error) {
	unwrapped := unwrapData(data)
	var wrapper struct {
		Comments []map[string]any `json:"comments"`
	}
	if err := json.Unmarshal(unwrapped, &wrapper); err != nil {
		return nil, err
	}

	comments := make([]commentInfo, 0, len(wrapper.Comments))
	for _, c := range wrapper.Comments {
		fullID, _ := c["id"].(string)
		content, _ := c["content"].(string)
		parentID, _ := c["parentId"].(string)
		createdAt, _ := c["createdAt"].(string)

		author := ""
		if a, ok := c["author"].(map[string]any); ok {
			author, _ = a["name"].(string)
		} else if name, ok := c["author"].(string); ok {
			author = name
		}

		// Resolve @UUID mentions to display names
		if mentions, ok := c["mentions"].([]any); ok && len(mentions) > 0 {
			replacements := make(map[string]string)
			for _, m := range mentions {
				mention, ok := m.(map[string]any)
				if !ok {
					continue
				}
				memberID, _ := mention["orgMemberId"].(string)
				name, _ := mention["name"].(string)
				if memberID != "" && name != "" {
					replacements[strings.ToLower(memberID)] = name
				}
			}
			if len(replacements) > 0 {
				content = resolveMentions(content, replacements)
			}
		}

		resolvedAt, _ := c["resolvedAt"].(string)
		resolver := ""
		if r, ok := c["resolver"].(map[string]any); ok {
			resolver, _ = r["name"].(string)
		}

		comments = append(comments, commentInfo{
			ID:         ids.Shorten(fullID),
			Content:    content,
			Author:     author,
			ParentID:   ids.ShortenOptional(strPtr(parentID)),
			CreatedAt:  formatTime(createdAt),
			ResolvedAt: formatTime(resolvedAt),
			Resolver:   resolver,
		})
	}
	return comments, nil
}

func formatComments(comments []commentInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Comments (%d):\n\n", len(comments))
	for _, c := range comments {
		prefix := ""
		if c.ParentID != "" {
			prefix = "  ↳ "
		}
		header := fmt.Sprintf("%s[%s] (%s, %s)", prefix, c.ID, c.Author, c.CreatedAt)
		if c.ResolvedAt != "" {
			header += " [RESOLVED"
			if c.Resolver != "" {
				header += " by " + c.Resolver
			}
			header += "]"
		}
		sb.WriteString(header + "\n")
		// Print body with indentation to keep comment boundaries clear
		indent := prefix + "  "
		for _, line := range strings.Split(c.Content, "\n") {
			fmt.Fprintf(&sb, "%s%s\n", indent, line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// --- Utility ---

func goalIcon(g goalInfo) string {
	if g.CompletedAt != "" {
		return "[x]"
	}
	return statusIcon(g.Status)
}

func statusIcon(status string) string {
	switch status {
	case "IN_PROGRESS":
		return "[~]"
	case "CANCELLED":
		// CANCELLED means "paused" in Addness, not terminated/deleted.
		return "[paused]"
	case "BLOCKED":
		return "[!]"
	default:
		return "[ ]"
	}
}

func formatTime(isoTime string) string {
	if isoTime == "" {
		return ""
	}
	if len(isoTime) >= 10 {
		return isoTime[:10]
	}
	return isoTime
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// resolveMentions replaces @UUID patterns in content with @displayName.
var mentionRegex = regexp.MustCompile(`(?i)@([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)

func resolveMentions(content string, replacements map[string]string) string {
	return mentionRegex.ReplaceAllStringFunc(content, func(match string) string {
		id := strings.ToLower(strings.TrimPrefix(match, "@"))
		if name, ok := replacements[id]; ok {
			return "@" + name
		}
		return match
	})
}
