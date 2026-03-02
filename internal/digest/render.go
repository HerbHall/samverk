package digest

import (
	"fmt"
	"strings"
	"time"
)

// FormatDigest renders DigestData as the conversational text format
// defined in docs/check-in-digest-design.md.
func FormatDigest(d DigestData) string {
	var b strings.Builder

	away := time.Since(d.LastCheckIn)
	fmt.Fprintf(&b, "SAMVERK: Welcome back. You've been away %s. Here's where things stand.\n", formatDuration(away))

	hasPending := len(d.PendingActions) > 0
	hasCompleted := len(d.CompletedActions) > 0
	hasActive := len(d.ActiveWork) > 0
	hasAny := hasPending || hasCompleted || hasActive || d.QueuedCount > 0 || d.BlockedCount > 0

	if !hasAny {
		b.WriteString("\nNo agents have run yet. To get started:\n")
		b.WriteString("1. Tell me about your project and what you're working on\n")
		b.WriteString("2. I'll create the first set of task issues\n")
		b.WriteString("3. Agents will pick them up and start working\n")
		return b.String()
	}

	if !hasPending {
		b.WriteString("\nNo decisions needed -- agents are unblocked.\n")
	}

	if hasPending {
		renderPendingActions(&b, d.PendingActions)
	}
	if hasCompleted {
		renderCompletedActions(&b, d.CompletedActions)
	}
	renderStatus(&b, d)

	b.WriteString("\nWhat would you like to do?\n")
	return b.String()
}

func renderPendingActions(b *strings.Builder, actions []PendingAction) {
	fmt.Fprintf(b, "\n--- NEEDS YOUR DECISION (%d item%s, blocking work) ---\n",
		len(actions), plural(len(actions)))

	for i, a := range actions {
		idx := i + 1
		actionType := string(a.ActionType)
		if actionType == "" {
			actionType = "action"
		}

		fmt.Fprintf(b, "\n[%d] %s: %s\n", idx, strings.ToUpper(actionType), a.Title)
		if a.Context != "" {
			fmt.Fprintf(b, "    Why: %s\n", truncate(a.Context, 80))
		}
		fmt.Fprintf(b, "    Blocks: %d dependent issue%s\n", a.BlockedCount, plural(a.BlockedCount))
		fmt.Fprintf(b, "    Waiting: %s\n", formatDuration(time.Since(a.RequestedAt)))
		fmt.Fprintf(b, "    > %d approve | %dr reject | %d? more context\n", idx, idx, idx)
	}
}

func renderCompletedActions(b *strings.Builder, actions []CompletedAction) {
	fmt.Fprintf(b, "\n--- COMPLETED AUTONOMOUSLY (%d action%s since last check-in) ---\n",
		len(actions), plural(len(actions)))

	groups := groupByDay(actions)
	for _, g := range groups {
		fmt.Fprintf(b, "\n%s:\n", g.label)
		for _, a := range g.actions {
			actionType := string(a.ActionType)
			if actionType == "" {
				actionType = "CLOSE"
			}
			summary := a.Title
			if a.ResultSummary != "" {
				summary = a.ResultSummary
			}
			fmt.Fprintf(b, "- %s #%d: %s\n", strings.ToUpper(actionType), a.IssueNumber, summary)
		}
	}
}

func renderStatus(b *strings.Builder, d DigestData) {
	b.WriteString("\n--- STATUS ---\n\n")

	if len(d.ActiveWork) > 0 {
		nums := make([]string, len(d.ActiveWork))
		for i, w := range d.ActiveWork {
			nums[i] = fmt.Sprintf("#%d %s", w.IssueNumber, w.Title)
		}
		fmt.Fprintf(b, "Active: %d issue%s in progress (%s)\n",
			len(d.ActiveWork), plural(len(d.ActiveWork)), strings.Join(nums, ", "))
	} else {
		b.WriteString("Active: 0 issues in progress\n")
	}

	fmt.Fprintf(b, "Queued: %d issue%s waiting\n", d.QueuedCount, plural(d.QueuedCount))
	fmt.Fprintf(b, "Blocked: %d issue%s (dependency, not user)\n", d.BlockedCount, plural(d.BlockedCount))

	if d.Cost.TokensUsed > 0 {
		fmt.Fprintf(b, "Cost: ~$%.2f (%dk tokens) since last check-in",
			d.Cost.EstimatedCostUSD, d.Cost.TokensUsed/1000)
		if d.Cost.BudgetRemainingUSD > 0 {
			fmt.Fprintf(b, " | $%.2f remaining", d.Cost.BudgetRemainingUSD)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Cost: no cost data available\n")
	}
}

type dayGroup struct {
	label   string
	actions []CompletedAction
}

func groupByDay(actions []CompletedAction) []dayGroup {
	if len(actions) == 0 {
		return nil
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)

	var todayActions, yesterdayActions, olderActions []CompletedAction

	for _, a := range actions {
		switch {
		case a.CompletedAt.After(today):
			todayActions = append(todayActions, a)
		case a.CompletedAt.After(yesterday):
			yesterdayActions = append(yesterdayActions, a)
		default:
			olderActions = append(olderActions, a)
		}
	}

	var groups []dayGroup
	if len(todayActions) > 0 {
		groups = append(groups, dayGroup{label: "Today", actions: todayActions})
	}
	if len(yesterdayActions) > 0 {
		groups = append(groups, dayGroup{label: "Yesterday", actions: yesterdayActions})
	}
	if len(olderActions) > 0 {
		groups = append(groups, dayGroup{label: "Earlier", actions: olderActions})
	}
	return groups
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours < 1 {
		mins := int(d.Minutes())
		if mins < 1 {
			return "just now"
		}
		return fmt.Sprintf("%dm", mins)
	}
	if hours < 48 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func truncate(s string, maxLen int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
