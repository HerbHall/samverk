package dispatcher

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"strings"
	"time"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// knownAgentTypes is the set of valid agent types for routing validation.
var knownAgentTypes = map[models.AgentType]bool{
	models.AgentTypeOrchestrator: true,
	models.AgentTypeDispatcher:   true,
	models.AgentTypeCodeGen:      true,
	models.AgentTypeTest:         true,
	models.AgentTypeDocs:         true,
	models.AgentTypeResearch:     true,
	models.AgentTypeQC:           true,
	models.AgentTypeHuman:        true,
}

// complexTitleKeywords are title signals that indicate architectural/heavy work.
var complexTitleKeywords = []string{"architect", "refactor", "redesign", "spike"}

// classify parses frontmatter from the issue body and validates the agent_type.
// Returns the agent type and parsed frontmatter (may be nil for heuristic matches).
// If frontmatter is present but malformed, returns an error immediately (no
// heuristic fallback). Heuristics are only attempted when frontmatter is absent.
func (d *Dispatcher) classify(_ context.Context, issue *forge.Issue) (models.AgentType, *models.IssueFrontmatter, error) {
	fm, err := d.parseFrontmatter(issue)
	if err != nil {
		return "", nil, fmt.Errorf("classify issue #%d: %w", issue.Number, err)
	}
	if fm == nil {
		if at := classifyByHeuristic(issue); at != "" {
			return at, nil, nil
		}
		return "", nil, fmt.Errorf("classify issue #%d: no frontmatter found and no heuristic match", issue.Number)
	}
	if fm.AgentType == "" {
		return "", nil, fmt.Errorf("classify issue #%d: agent_type is empty", issue.Number)
	}
	if !knownAgentTypes[fm.AgentType] {
		return "", nil, fmt.Errorf("classify issue #%d: unknown agent_type %q", issue.Number, fm.AgentType)
	}
	return fm.AgentType, fm, nil
}

// classifyByHeuristic attempts to determine the agent type from issue labels
// and title prefix when no YAML frontmatter is present.
// Returns empty string if no heuristic matches.
func classifyByHeuristic(issue *forge.Issue) models.AgentType {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}

	// Label-based rules (highest priority).
	if labels["agent:human"] {
		return models.AgentTypeHuman
	}
	if labels["type:spike"] || labels["type:research"] {
		return models.AgentTypeResearch
	}
	if labels["bug"] {
		return models.AgentTypeCodeGen
	}

	// Title-prefix rules.
	lower := strings.ToLower(issue.Title)
	switch {
	case strings.HasPrefix(lower, "fix:") || strings.HasPrefix(lower, "fix("):
		return models.AgentTypeCodeGen
	case strings.HasPrefix(lower, "feat:") || strings.HasPrefix(lower, "feat("),
		strings.HasPrefix(lower, "feature:") || strings.HasPrefix(lower, "feature("):
		return models.AgentTypeCodeGen
	case strings.HasPrefix(lower, "docs:") || strings.HasPrefix(lower, "docs("),
		strings.HasPrefix(lower, "chore:") || strings.HasPrefix(lower, "chore("):
		return models.AgentTypeDocs
	case strings.HasPrefix(lower, "test:") || strings.HasPrefix(lower, "test("):
		return models.AgentTypeTest
	}

	return ""
}

// selectProviderKey examines issue signals and returns the routing chain key
// that should be used for provider selection, along with a human-readable reason.
//
// Priority (highest first):
//  1. complex  — critical priority, high complexity, or architectural title keywords
//  2. local    — boilerplate/scaffold labels, or "chore:" title prefix
//  3. triage   — low priority label, docs agent type, or short body (< 200 words)
//  4. default  — everything else
func selectProviderKey(issue *forge.Issue, agentType models.AgentType) (key, reason string) {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}
	lower := strings.ToLower(issue.Title)

	// Complex: critical priority, high complexity, or architectural title keywords.
	if labels["priority:critical"] {
		return "complex", "label priority:critical"
	}
	if labels["complexity:high"] {
		return "complex", "label complexity:high"
	}
	for _, kw := range complexTitleKeywords {
		if strings.Contains(lower, kw) {
			return "complex", "title keyword " + kw
		}
	}

	// Local: boilerplate/scaffold labels or chore title prefix.
	if labels["type:boilerplate"] {
		return "local", "label type:boilerplate"
	}
	if labels["type:scaffold"] {
		return "local", "label type:scaffold"
	}
	if strings.HasPrefix(lower, "chore:") {
		return "local", "title prefix chore:"
	}

	// Triage: low priority, docs agent, or short issue body.
	if labels["priority:low"] {
		return "triage", "label priority:low"
	}
	if agentType == models.AgentTypeDocs {
		return "triage", "agent type docs"
	}
	if wordCount := len(strings.Fields(issue.Body)); wordCount < 200 {
		return "triage", fmt.Sprintf("short issue body (%d words)", wordCount)
	}

	return "default", "default routing"
}

// route assigns the issue to the agent pool matching agentType.
// It selects a provider routing chain based on issue signals, logs the selection,
// and records the claim in memory. Any failure count accumulated from prior
// timeout cycles is carried forward so the escalation threshold is not reset.
func (d *Dispatcher) route(ctx context.Context, issue *forge.Issue, agentType models.AgentType, fm *models.IssueFrontmatter) error {
	// Human-typed issues are tracked but never submitted to the agent pool.
	if agentType == models.AgentTypeHuman {
		d.logger.Info("issue classified as human", zap.Int("issue", issue.Number))
		if err := d.tracker.AddLabel(ctx, issue.Number, "status:needs-human"); err != nil {
			d.logger.Error("add label", zap.Int("issue", issue.Number), zap.String("label", "needs-human"), zap.String("error", err.Error()))
		}
		return nil
	}

	if err := d.tracker.RemoveLabel(ctx, issue.Number, "status:queued"); err != nil {
		d.logger.Debug("remove queued label", zap.Int("issue", issue.Number), zap.String("error", err.Error()))
	}
	if err := d.tracker.AddLabel(ctx, issue.Number, "status:claimed"); err != nil {
		return fmt.Errorf("add claimed label to #%d: %w", issue.Number, err)
	}
	if err := d.tracker.Assign(ctx, issue.Number, string(agentType)); err != nil {
		return fmt.Errorf("assign #%d to %s: %w", issue.Number, agentType, err)
	}

	providerKey, reason := selectProviderKey(issue, agentType)

	// Estimate per-issue timeout from complexity signals or frontmatter override.
	timeout := EstimateTimeout(issue, fm, agentType, providerKey)
	d.logger.Info("timeout estimated",
		zap.Int("issue", issue.Number),
		zap.Duration("timeout", timeout),
	)

	now := time.Now()
	d.mu.Lock()
	priorFailures := d.issueFailures[issue.Number]
	d.claimed[issue.Number] = &claimedIssue{
		AgentID:       string(agentType),
		ClaimedAt:     now,
		LastHeartbeat: now,
		FailureCount:  priorFailures,
	}
	d.mu.Unlock()

	d.logger.Info("routed",
		zap.Int("issue", issue.Number),
		zap.String("agent", string(agentType)),
		zap.String("provider", providerKey),
		zap.String("reason", reason),
		zap.Int("attempt", priorFailures+1),
	)

	// Submit to agent pool if available.
	if d.pool != nil && d.store != nil {
		sessionID := fmt.Sprintf("sess_%d_%d", issue.Number, time.Now().Unix())
		session := &models.Session{
			ID:          sessionID,
			IssueNumber: issue.Number,
			AgentType:   string(agentType),
			Status:      models.SessionStatusPending,
			StartedAt:   time.Now(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := d.store.CreateSession(ctx, session); err != nil {
			return fmt.Errorf("create session for #%d: %w", issue.Number, err)
		}
		issueNum := issue.Number
		task := agent.Task{
			Issue:       issue,
			AgentType:   agentType,
			SessionID:   sessionID,
			ProviderKey: providerKey,
			Timeout:     timeout,
			HeartbeatFunc: func() {
				d.mu.Lock()
				if c, ok := d.claimed[issueNum]; ok {
					c.LastHeartbeat = time.Now()
				}
				d.mu.Unlock()
			},
		}
		if err := d.pool.Submit(task); err != nil {
			return fmt.Errorf("submit agent task for #%d: %w", issue.Number, err)
		}
	}
	return nil
}

// parseFrontmatter extracts the IssueFrontmatter from an issue's body.
// Returns (nil, nil) if no frontmatter block exists.
func (d *Dispatcher) parseFrontmatter(issue *forge.Issue) (*models.IssueFrontmatter, error) {
	result, err := models.ParseFrontmatter(issue.Body)
	if err != nil {
		return nil, err
	}
	return result.Frontmatter, nil
}
