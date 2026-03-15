// Package digest assembles check-in digests from forge issues.
//
// The digest is the primary surface through which users interact with Samverk
// during their limited availability windows. It answers three questions:
//   1. What needs my decision? (Tier 3 pending actions)
//   2. What happened while I was away? (Tier 2 completed actions)
//   3. What is the overall state? (Active work, backlog, cost)
package digest

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// DigestData is the top-level container returned by BuildDigest.
type DigestData struct {
	PendingActions   []PendingAction
	CompletedActions []CompletedAction
	ActiveWork       []ActiveWork
	PRsAwaiting      []PRAwaitingReview
	AutoMergedCount  int
	QueuedCount      int
	BlockedCount     int
	Cost             CostSummary
	LastCheckIn      time.Time
}

// PendingAction is a Tier 3 item awaiting user approval or rejection.
type PendingAction struct {
	IssueNumber  int
	Title        string
	ActionType   autonomy.ActionType
	Priority     models.Priority
	Context      string
	BlockedCount int
	RequestedAt  time.Time
}

// CompletedAction is a Tier 2 item the agent executed autonomously.
type CompletedAction struct {
	IssueNumber   int
	Title         string
	ActionType    autonomy.ActionType
	ResultSummary string
	TokensUsed    int
	ModelUsed     string
	CompletedAt   time.Time
}

// ActiveWork is an issue currently being worked by an agent.
type ActiveWork struct {
	IssueNumber int
	Title       string
	AgentType   models.AgentType
	ClaimedAt   time.Time
}

// PRAwaitingReview is an open PR shown in the digest's PR section.
type PRAwaitingReview struct {
	Project  string
	Number   int
	Title    string
	Author   string
	Tier     string
	TierDesc string
	CIStatus string
	Age      string
}

// CostSummary aggregates token spend since the last check-in.
type CostSummary struct {
	TokensUsed         int
	EstimatedCostUSD   float64
	BudgetRemainingUSD float64
}

// CostSource provides cost data for the digest. A nil CostSource
// is valid and results in zeroed cost fields.
type CostSource interface {
	ComputeCostSince(ctx context.Context, since time.Time) CostSummary
}

// BuildDigest queries the IssueTracker to populate DigestData in a single
// server-side call. The costs parameter may be nil for environments without
// cost tracking (e.g., this spike).
func BuildDigest(ctx context.Context, tracker forge.IssueTracker, costs CostSource, lastCheckIn time.Time) (DigestData, error) {
	var d DigestData
	d.LastCheckIn = lastCheckIn

	// 1. Tier 3 pending actions: status:needs-human, sorted by priority then age.
	needsHuman, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State: forge.StateOpen, Labels: []string{"status:needs-human"},
	})
	if err != nil {
		return d, err
	}
	for _, iss := range needsHuman {
		pa := PendingAction{
			IssueNumber: iss.Number,
			Title:       iss.Title,
			RequestedAt: iss.CreatedAt,
		}
		pr, _ := models.ParseFrontmatter(iss.Body)
		if pr != nil && pr.Frontmatter != nil {
			pa.ActionType = autonomy.ActionType(pr.Frontmatter.Type)
			pa.Priority = pr.Frontmatter.Priority
		}
		pa.Context = extractSection(iss.Body, "Context")
		pa.BlockedCount = countDependents(ctx, tracker, iss.Number)
		d.PendingActions = append(d.PendingActions, pa)
	}
	sortPendingActions(d.PendingActions)

	// 2. Tier 2 completed actions: closed since last check-in.
	closed, err := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateClosed})
	if err != nil {
		return d, err
	}
	for _, iss := range closed {
		if iss.ClosedAt == nil || !iss.ClosedAt.After(lastCheckIn) {
			continue
		}
		ca := CompletedAction{
			IssueNumber: iss.Number,
			Title:       iss.Title,
			CompletedAt: *iss.ClosedAt,
		}
		pr, _ := models.ParseFrontmatter(iss.Body)
		if pr != nil && pr.Frontmatter != nil {
			ca.ActionType = autonomy.ActionType(pr.Frontmatter.Type)
			ca.TokensUsed = pr.Frontmatter.ActualTokens
			ca.ModelUsed = pr.Frontmatter.ModelUsed
		}
		ca.ResultSummary = extractSection(iss.Body, "Result")
		d.CompletedActions = append(d.CompletedActions, ca)
	}

	// 3. Active work: status:in-progress.
	active, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State: forge.StateOpen, Labels: []string{"status:in-progress"},
	})
	if err != nil {
		return d, err
	}
	for _, iss := range active {
		aw := ActiveWork{
			IssueNumber: iss.Number,
			Title:       iss.Title,
			ClaimedAt:   iss.UpdatedAt,
		}
		pr, _ := models.ParseFrontmatter(iss.Body)
		if pr != nil && pr.Frontmatter != nil {
			aw.AgentType = pr.Frontmatter.AgentType
		}
		d.ActiveWork = append(d.ActiveWork, aw)
	}

	// 4. Counts.
	queued, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State: forge.StateOpen, Labels: []string{"status:queued"},
	})
	if err != nil {
		return d, err
	}
	d.QueuedCount = len(queued)

	blocked, err := tracker.ListIssues(ctx, &forge.ListOptions{
		State: forge.StateOpen, Labels: []string{"status:blocked"},
	})
	if err != nil {
		return d, err
	}
	d.BlockedCount = len(blocked)

	// 5. Cost summary.
	if costs != nil {
		d.Cost = costs.ComputeCostSince(ctx, lastCheckIn)
	}

	return d, nil
}

// priorityRank returns a numeric rank for sorting (lower = higher priority).
func priorityRank(p models.Priority) int {
	switch p {
	case models.PriorityCritical:
		return 0
	case models.PriorityHigh:
		return 1
	case models.PriorityNormal:
		return 2
	case models.PriorityLow:
		return 3
	default:
		return 4
	}
}

// sortPendingActions sorts by priority (critical first) then by age (oldest first).
func sortPendingActions(actions []PendingAction) {
	sort.Slice(actions, func(i, j int) bool {
		ri, rj := priorityRank(actions[i].Priority), priorityRank(actions[j].Priority)
		if ri != rj {
			return ri < rj
		}
		return actions[i].RequestedAt.Before(actions[j].RequestedAt)
	})
}

// extractSection pulls a named markdown section from the issue body.
// It looks for "## SectionName" or "### SectionName" and returns the content
// until the next heading or end of body.
func extractSection(body, section string) string {
	lines := strings.Split(body, "\n")
	var collecting bool
	var result []string
	target := strings.ToLower(section)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimLeft(trimmed, "# ")
			if strings.EqualFold(heading, target) {
				collecting = true
				continue
			}
			if collecting {
				break
			}
		}
		if collecting {
			result = append(result, line)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// countDependents counts open issues whose depends_on includes the given issue number.
func countDependents(ctx context.Context, tracker forge.IssueTracker, issueNumber int) int {
	// Query all open issues and check frontmatter for depends_on references.
	// This is intentionally simple for the spike -- a production implementation
	// would use a dependency index in the store.
	all, err := tracker.ListIssues(ctx, &forge.ListOptions{State: forge.StateOpen})
	if err != nil {
		return 0
	}
	count := 0
	for _, iss := range all {
		pr, _ := models.ParseFrontmatter(iss.Body)
		if pr != nil && pr.Frontmatter != nil {
			for _, dep := range pr.Frontmatter.DependsOn {
				if !dep.IsCrossProject() && dep.Number == issueNumber {
					count++
					break
				}
			}
		}
	}
	return count
}
