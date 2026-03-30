package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/internal/store"
	"github.com/herbhall/samverk/pkg/models"
)

// TriageVerdict represents the outcome of a triage evaluation.
type TriageVerdict string

const (
	VerdictAutoUnblock   TriageVerdict = "auto_unblock"
	VerdictDecompose     TriageVerdict = "decompose"
	VerdictConfirmedHuman TriageVerdict = "confirmed_human"
)

// triageEvalResult is the structured JSON response from the AI evaluation.
type triageEvalResult struct {
	Verdict         string              `json:"verdict"`
	Reasoning       string              `json:"reasoning"`
	CriteriaResults []triageCriterion   `json:"criteria_results"`
	DocsConsulted   []string            `json:"docs_consulted"`
	SubIssues       []triageSubIssue    `json:"sub_issues"`
}

// triageCriterion captures whether a single evaluation criterion passed.
type triageCriterion struct {
	Criterion string `json:"criterion"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail"`
}

// triageSubIssue is a proposed sub-issue for the "decompose" verdict.
type triageSubIssue struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	AgentType string `json:"agent_type"`
}

// TriageAgent is a background goroutine that periodically evaluates
// needs-human issues and either unblocks, decomposes, or confirms them.
type TriageAgent struct {
	trackerEntries []TrackerEntry
	store          store.Store
	provider       provider.Provider
	config         *TriageConfig
	dispatcher     *Dispatcher
	logger         *zap.Logger
	lastDigest     time.Time
}

// NewTriageAgent creates a new triage agent. The provider should be a
// high-reasoning model (e.g., Claude Opus) for nuanced judgment.
func NewTriageAgent(
	entries []TrackerEntry,
	st store.Store,
	prov provider.Provider,
	cfg *TriageConfig,
	disp *Dispatcher,
	logger *zap.Logger,
) *TriageAgent {
	if cfg == nil {
		cfg = DefaultTriageConfig()
	}
	return &TriageAgent{
		trackerEntries: entries,
		store:          st,
		provider:       prov,
		config:         cfg,
		dispatcher:     disp,
		logger:         logger.Named("triage"),
		lastDigest:     time.Now(),
	}
}

// Run starts the triage loop. It blocks until ctx is cancelled.
func (t *TriageAgent) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.config.Interval)
	defer ticker.Stop()

	t.logger.Info("triage agent started",
		zap.Duration("interval", t.config.Interval),
		zap.Int("max_unblocks", t.config.MaxUnblocksPerCycle),
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.runCycle(ctx)
			t.maybeGenerateDigest(ctx)
		}
	}
}

// runCycle performs one triage evaluation cycle across all trackers.
func (t *TriageAgent) runCycle(ctx context.Context) {
	unblockedThisCycle := 0

	for i := range t.trackerEntries {
		entry := t.trackerEntries[i]
		issues, err := entry.Tracker.ListIssues(ctx, &forge.ListOptions{
			State:   forge.StateOpen,
			Labels:  []string{models.LabelStatusNeedsHuman},
			PerPage: 50,
		})
		if err != nil {
			t.logger.Error("triage: list needs-human issues",
				zap.String("owner", entry.Owner),
				zap.String("repo", entry.Repo),
				zap.Error(err),
			)
			continue
		}

		t.logger.Info("triage cycle started",
			zap.String("owner", entry.Owner),
			zap.String("repo", entry.Repo),
			zap.Int("needs_human_count", len(issues)),
		)

		for j := range issues {
			if unblockedThisCycle >= t.config.MaxUnblocksPerCycle {
				t.logger.Info("triage: max unblocks reached for cycle",
					zap.Int("max", t.config.MaxUnblocksPerCycle),
				)
				break
			}

			issue := issues[j]

			// Skip issues already evaluated recently (within this cycle interval).
			if t.recentlyEvaluated(ctx, issue.Number) {
				t.logger.Debug("triage: skipping recently evaluated issue",
					zap.Int("issue", issue.Number),
				)
				continue
			}

			verdict, err := t.evaluateIssue(ctx, entry, issue)
			if err != nil {
				t.logger.Error("triage: evaluate issue",
					zap.Int("issue", issue.Number),
					zap.Error(err),
				)
				continue
			}

			if verdict == VerdictAutoUnblock {
				unblockedThisCycle++
			}
		}
	}

	t.logger.Info("triage cycle completed",
		zap.Int("unblocked", unblockedThisCycle),
	)
}

// recentlyEvaluated returns true if the issue was already evaluated
// within the current triage interval.
func (t *TriageAgent) recentlyEvaluated(ctx context.Context, issueNumber int) bool {
	if t.store == nil {
		return false
	}
	decision, err := t.store.GetTriageDecisionForIssue(ctx, issueNumber)
	if err != nil {
		return false
	}
	return time.Since(decision.CreatedAt) < t.config.Interval
}

// evaluateIssue runs the full triage flow for a single issue:
// gather context, check hard blocks, call AI evaluation, take action.
func (t *TriageAgent) evaluateIssue(ctx context.Context, entry TrackerEntry, issue *forge.Issue) (verdict TriageVerdict, err error) {
	project := entry.Owner + "/" + entry.Repo

	// Step 1: Check hard blocks before calling AI (saves tokens).
	if hardBlock := t.checkHardBlocks(issue); hardBlock != "" {
		verdict = VerdictConfirmedHuman
		t.recordDecision(ctx, issue.Number, project, verdict, hardBlock, "[]", "[]", "[]")
		t.postTriageComment(ctx, entry.Tracker, issue.Number, verdict, hardBlock)
		return verdict, nil
	}

	// Step 2: Gather context for AI evaluation.
	comments, err := entry.Tracker.ListComments(ctx, issue.Number)
	if err != nil {
		t.logger.Warn("triage: list comments failed",
			zap.Int("issue", issue.Number),
			zap.Error(err),
		)
		comments = nil
	}

	commentBodies := make([]string, 0, len(comments))
	for i := range comments {
		commentBodies = append(commentBodies, comments[i].Body)
	}

	// Get failure history.
	failureHistory := t.getFailureHistory(ctx, issue.Number)

	// Step 3: Build prompt and call AI provider.
	input := TriagePromptInput{
		IssueNumber:    issue.Number,
		IssueTitle:     issue.Title,
		IssueBody:      issue.Body,
		Comments:       commentBodies,
		FailureHistory: failureHistory,
		Labels:         issue.Labels,
		HardBlockRules: t.config.HardBlockKeywords,
		AutoApproveTypes: t.config.AutoApproveTypes,
	}

	evalResult, err := t.callProvider(ctx, input)
	if err != nil {
		return "", fmt.Errorf("provider evaluation: %w", err)
	}

	// Parse the verdict.
	switch evalResult.Verdict {
	case string(VerdictAutoUnblock):
		verdict = VerdictAutoUnblock
	case string(VerdictDecompose):
		verdict = VerdictDecompose
	default:
		verdict = VerdictConfirmedHuman
	}

	// Marshal criteria and docs for storage.
	criteriaJSON, _ := json.Marshal(evalResult.CriteriaResults)
	docsJSON, _ := json.Marshal(evalResult.DocsConsulted)

	// Step 4: Take action based on verdict.
	switch verdict {
	case VerdictAutoUnblock:
		err = t.autoUnblock(ctx, entry, issue)
		if err != nil {
			return "", fmt.Errorf("auto-unblock #%d: %w", issue.Number, err)
		}

	case VerdictDecompose:
		subIssueNumbers, decErr := t.createSubIssues(ctx, entry, issue, evalResult.SubIssues)
		if decErr != nil {
			t.logger.Error("triage: create sub-issues failed",
				zap.Int("issue", issue.Number),
				zap.Error(decErr),
			)
			// Fall through to record the decision anyway.
		}
		subJSON, _ := json.Marshal(subIssueNumbers)
		t.recordDecision(ctx, issue.Number, project, verdict, evalResult.Reasoning,
			string(criteriaJSON), string(docsJSON), string(subJSON))
		t.postTriageComment(ctx, entry.Tracker, issue.Number, verdict, evalResult.Reasoning)
		return verdict, nil

	case VerdictConfirmedHuman:
		// Leave as-is; just record and comment.
	}

	t.recordDecision(ctx, issue.Number, project, verdict, evalResult.Reasoning,
		string(criteriaJSON), string(docsJSON), "[]")
	t.postTriageComment(ctx, entry.Tracker, issue.Number, verdict, evalResult.Reasoning)

	return verdict, nil
}

// checkHardBlocks returns a non-empty reason if the issue matches a hard block rule.
func (t *TriageAgent) checkHardBlocks(issue *forge.Issue) string {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}

	// Check hard-block labels.
	for _, blockLabel := range t.config.HardBlockLabels {
		if labels[blockLabel] {
			return fmt.Sprintf("hard block: issue has label %q", blockLabel)
		}
	}

	// Check for human:review label (always hard-blocked).
	if labels[models.LabelHumanReview] {
		return "hard block: issue has human:review label"
	}

	// Check hard-block keywords in title and body.
	combined := strings.ToLower(issue.Title + " " + issue.Body)
	for _, keyword := range t.config.HardBlockKeywords {
		if strings.Contains(combined, strings.ToLower(keyword)) {
			return fmt.Sprintf("hard block: issue contains keyword %q", keyword)
		}
	}

	return ""
}

// getFailureHistory retrieves recent failure messages for the issue.
func (t *TriageAgent) getFailureHistory(ctx context.Context, issueNumber int) []string {
	if t.store == nil {
		return nil
	}
	failures, err := t.store.RecentFailuresForIssue(ctx, issueNumber, 5)
	if err != nil {
		t.logger.Warn("triage: get failure history",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
		return nil
	}
	return failures
}

// callProvider sends the evaluation prompt to the AI provider and parses the response.
func (t *TriageAgent) callProvider(ctx context.Context, input TriagePromptInput) (*triageEvalResult, error) {
	if t.provider == nil {
		return nil, fmt.Errorf("no provider configured for triage evaluation")
	}

	prompt := BuildTriagePrompt(input)

	resp, err := t.provider.Chat(ctx, provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	content := resp.Message.Content

	// Strip markdown code fences if present.
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			lines = lines[1 : len(lines)-1]
			content = strings.Join(lines, "\n")
		}
	}

	var result triageEvalResult
	if err = json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse evaluation response: %w (raw: %s)", err, truncate(content, 200))
	}

	return &result, nil
}

// autoUnblock removes needs-human and adds status:queued to re-enter the dispatch queue.
func (t *TriageAgent) autoUnblock(ctx context.Context, entry TrackerEntry, issue *forge.Issue) error {
	if err := entry.Tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusNeedsHuman); err != nil {
		return fmt.Errorf("remove needs-human: %w", err)
	}
	if err := entry.Tracker.AddLabels(ctx, issue.Number, models.LabelStatusQueued); err != nil {
		return fmt.Errorf("add queued: %w", err)
	}

	t.logger.Info("triage: auto-unblocked issue",
		zap.Int("issue", issue.Number),
		zap.String("owner", entry.Owner),
		zap.String("repo", entry.Repo),
	)

	// Record pipeline transition.
	if t.dispatcher != nil {
		t.dispatcher.recordPipelineEvent(ctx, entry.Owner, entry.Repo, issue.Number,
			models.LabelStatusNeedsHuman, models.LabelStatusQueued, "triage-agent")
	}

	// Clear persisted failure count so the issue gets a fresh start.
	if t.store != nil {
		_ = t.store.ClearIssueFailureCount(ctx, issue.Number)
	}

	return nil
}

// createSubIssues creates child issues for the decompose verdict.
func (t *TriageAgent) createSubIssues(ctx context.Context, entry TrackerEntry, parent *forge.Issue, subs []triageSubIssue) ([]int, error) {
	if len(subs) == 0 {
		return nil, nil
	}

	numbers := make([]int, 0, len(subs))
	for i, sub := range subs {
		agentType := sub.AgentType
		if agentType == "" {
			agentType = "code-gen"
		}

		body := buildChildBody(parent.Number, models.AgentType(agentType), sub.Body, nil)
		labels := []string{
			models.LabelStatusQueued,
			fmt.Sprintf("agent:%s", agentType),
			"decomposed",
		}

		created, err := entry.Tracker.CreateIssue(ctx, &forge.CreateIssueRequest{
			Title:  sub.Title,
			Body:   body,
			Labels: labels,
		})
		if err != nil {
			return numbers, fmt.Errorf("create sub-issue %d/%d: %w", i+1, len(subs), err)
		}

		numbers = append(numbers, created.Number)
		t.logger.Info("triage: sub-issue created",
			zap.Int("parent", parent.Number),
			zap.Int("child", created.Number),
			zap.String("title", sub.Title),
		)
	}

	return numbers, nil
}

// recordDecision persists a triage decision to the store.
func (t *TriageAgent) recordDecision(ctx context.Context, issueNumber int, project string, verdict TriageVerdict, reasoning, criteriaJSON, docsJSON, subIssuesJSON string) {
	if t.store == nil {
		return
	}
	d := &store.TriageDecision{
		IssueNumber:      issueNumber,
		Project:          project,
		Verdict:          string(verdict),
		Reasoning:        reasoning,
		CriteriaMet:      criteriaJSON,
		DocsReferenced:   docsJSON,
		SubIssuesCreated: subIssuesJSON,
		CreatedAt:        time.Now().UTC(),
	}
	if err := t.store.SaveTriageDecision(ctx, d); err != nil {
		t.logger.Error("triage: save decision",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}
}

// postTriageComment adds a structured comment on the issue explaining the verdict.
func (t *TriageAgent) postTriageComment(ctx context.Context, tracker forge.IssueTracker, issueNumber int, verdict TriageVerdict, reasoning string) {
	var action string
	switch verdict {
	case VerdictAutoUnblock:
		action = "Removed `needs-human`, added `status:queued`. Issue will be re-dispatched."
	case VerdictDecompose:
		action = "Created sub-issues for automatable parts. Original issue scope narrowed to human-blocked portion."
	case VerdictConfirmedHuman:
		action = "Confirmed as genuinely requiring human input. No status change."
	}

	comment := fmt.Sprintf(
		"TRIAGE [triage-agent] [%s]\nverdict: %s\nreasoning: %s\naction: %s",
		time.Now().UTC().Format(time.RFC3339),
		string(verdict),
		reasoning,
		action,
	)

	if _, err := tracker.AddComment(ctx, issueNumber, comment); err != nil {
		t.logger.Error("triage: add comment",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}
}

// maybeGenerateDigest checks if a digest is due and generates it.
func (t *TriageAgent) maybeGenerateDigest(ctx context.Context) {
	if time.Since(t.lastDigest) < t.config.DigestInterval {
		return
	}

	t.lastDigest = time.Now()

	if t.store == nil {
		return
	}

	since := time.Now().Add(-t.config.DigestInterval)
	decisions, err := t.store.ListTriageDecisions(ctx, since, 0)
	if err != nil {
		t.logger.Error("triage: list decisions for digest", zap.Error(err))
		return
	}

	if len(decisions) == 0 {
		t.logger.Info("triage digest: no decisions in period")
		return
	}

	digest := t.formatDigest(decisions)
	t.logger.Info("triage digest generated",
		zap.Int("decisions", len(decisions)),
		zap.String("digest", digest),
	)

	// Post digest as a comment on the first tracker's first needs-human issue
	// or log it. For now, just log — a dedicated tracking issue can be added later.
	broadcastEvent(t.dispatcher.broadcaster, "triage.digest", map[string]any{
		"digest":         digest,
		"decision_count": len(decisions),
	})
}

// formatDigest creates a human-readable summary of triage decisions.
func (t *TriageAgent) formatDigest(decisions []store.TriageDecision) string {
	var sb strings.Builder
	sb.WriteString("## Triage Digest\n\n")

	counts := map[string]int{
		"auto_unblock":    0,
		"decompose":       0,
		"confirmed_human": 0,
	}

	for i := range decisions {
		counts[decisions[i].Verdict]++
	}

	fmt.Fprintf(&sb, "**Period:** %s\n", t.config.DigestInterval)
	fmt.Fprintf(&sb, "**Total evaluated:** %d\n", len(decisions))
	fmt.Fprintf(&sb, "- Auto-unblocked: %d\n", counts["auto_unblock"])
	fmt.Fprintf(&sb, "- Decomposed: %d\n", counts["decompose"])
	fmt.Fprintf(&sb, "- Confirmed human: %d\n\n", counts["confirmed_human"])

	sb.WriteString("### Decisions\n\n")
	for i := range decisions {
		d := &decisions[i]
		fmt.Fprintf(&sb, "- **#%d** [%s/%s]: %s — %s\n",
			d.IssueNumber, d.Project, d.Verdict,
			d.Verdict, truncate(d.Reasoning, 100))
	}

	return sb.String()
}

// truncate returns the first n characters of s, adding "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
