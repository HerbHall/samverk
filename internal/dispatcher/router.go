package dispatcher

import (
	"context"
	"fmt"
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
// Returns an error if frontmatter is missing or agent_type is invalid.
func (d *Dispatcher) classify(_ context.Context, issue *forge.Issue) (models.AgentType, error) {
	fm, err := d.parseFrontmatter(issue)
	if err != nil {
		return "", fmt.Errorf("classify issue #%d: %w", issue.Number, err)
	}
	if fm == nil {
		if at := classifyByHeuristic(issue); at != "" {
			return at, nil
		}
		return "", fmt.Errorf("classify issue #%d: no frontmatter found and no heuristic match", issue.Number)
	}
	if fm.AgentType == "" {
		return "", fmt.Errorf("classify issue #%d: agent_type is empty", issue.Number)
	}
	if !knownAgentTypes[fm.AgentType] {
		return "", fmt.Errorf("classify issue #%d: unknown agent_type %q", issue.Number, fm.AgentType)
	}
	return fm.AgentType, nil
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
func (d *Dispatcher) route(ctx context.Context, issue *forge.Issue, agentType models.AgentType) error {
	if err := d.tracker.RemoveLabel(ctx, issue.Number, "status:queued"); err != nil {
		d.logger.Printf("remove queued label from #%d: %v", issue.Number, err)
	}
	if err := d.tracker.AddLabel(ctx, issue.Number, "status:claimed"); err != nil {
		return fmt.Errorf("add claimed label to #%d: %w", issue.Number, err)
	}
	if err := d.tracker.Assign(ctx, issue.Number, string(agentType)); err != nil {
		return fmt.Errorf("assign #%d to %s: %w", issue.Number, agentType, err)
	}

	providerKey, reason := selectProviderKey(issue, agentType)
	d.logger.Printf("selected provider=%s reason=%s issue=#%d", providerKey, reason, issue.Number)

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

	d.logger.Printf("routed issue #%d to %s", issue.Number, agentType)

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
		task := agent.Task{
			Issue:       issue,
			AgentType:   agentType,
			SessionID:   sessionID,
			ProviderKey: providerKey,
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
