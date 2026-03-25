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
//
// When a heuristic match succeeds for an issue without frontmatter, classify
// auto-generates frontmatter and attempts to persist it back to the issue body
// via UpdateIssue. If persistence fails the generated frontmatter is still
// returned for in-memory routing.
func (d *Dispatcher) classify(ctx context.Context, owner, repo string, issue *forge.Issue) (models.AgentType, *models.IssueFrontmatter, error) {
	fm, err := d.parseFrontmatter(issue)
	if err != nil {
		return "", nil, fmt.Errorf("classify issue #%d: %w", issue.Number, err)
	}
	if fm == nil {
		if at := classifyByHeuristic(issue); at != "" {
			fm = d.autoInjectFrontmatter(ctx, owner, repo, issue, at)
			return at, fm, nil
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
//  0. agent-type overrides — docs/test agents have fixed chains regardless of title
//  1. complex  — critical priority, high complexity, or architectural title keywords
//  2. local    — boilerplate/scaffold labels, or "chore:" title prefix
//  3. triage   — low priority label or short prose body (< 200 words, excluding frontmatter)
//  4. default  — everything else
func selectProviderKey(issue *forge.Issue, agentType models.AgentType) (key, reason string) {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}
	lower := strings.ToLower(issue.Title)

	// Agent-type overrides: docs agents have a fixed chain regardless of title
	// keywords. This prevents docs issues with "architecture" in the title
	// from being routed to the expensive complex chain (#263).
	if agentType == models.AgentTypeDocs {
		return "triage", "agent type docs"
	}

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

	// QC: dedicated chain ensures cross-model validation (ADR-030).
	// Checked after complex/local so critical QC issues still get the complex chain.
	if agentType == models.AgentTypeQC {
		return "qc", "agent type qc (cross-model validation)"
	}

	// Code-gen and test agents require CLI-capable providers that can produce
	// file changes (EDIT blocks or worktree commits). Ollama providers return
	// prose only, wasting sessions (#269, #270). Route to "code-gen" chain
	// which contains only CLI providers.
	//
	// Placed after complex/local so critical/high-complexity code-gen issues
	// still get the complex chain (which is already CLI-only). But before
	// triage/default to prevent code-gen from falling into Ollama-first chains.
	if agentType == models.AgentTypeCodeGen || agentType == models.AgentTypeTest {
		return "code-gen", "agent type " + string(agentType) + " (requires CLI provider)"
	}

	// Triage: low priority or short prose body (excluding YAML frontmatter).
	if labels["priority:low"] {
		return "triage", "label priority:low"
	}
	proseBody := stripFrontmatter(issue.Body)
	if wordCount := len(strings.Fields(proseBody)); wordCount < 200 {
		return "triage", fmt.Sprintf("short issue body (%d words)", wordCount)
	}

	return "default", "default routing"
}

// stripFrontmatter removes YAML frontmatter (between --- markers) from the
// issue body so word counts reflect actual prose content, not metadata (#264).
func stripFrontmatter(body string) string {
	const marker = "---"
	start := strings.Index(body, marker)
	if start == -1 {
		return body
	}
	// Only strip if the marker is at the start of the body (possibly after whitespace).
	prefix := strings.TrimSpace(body[:start])
	if prefix != "" {
		return body
	}
	end := strings.Index(body[start+len(marker):], marker)
	if end == -1 {
		return body
	}
	// Return everything after the closing --- marker.
	return body[start+len(marker)+end+len(marker):]
}

// route assigns the issue to the agent pool matching agentType.
// It selects a provider routing chain based on issue signals, logs the selection,
// and records the claim in memory. Any failure count accumulated from prior
// timeout cycles is carried forward so the escalation threshold is not reset.
//
// Circuit breaker checks are applied before dispatching:
//   - Budget circuit open → do not dispatch anything
//   - Provider circuit open → skip that provider (fallback may still work)
func (d *Dispatcher) route(ctx context.Context, owner, repo string, issue *forge.Issue, agentType models.AgentType, fm *models.IssueFrontmatter) error {
	tracker := d.trackerFor(owner, repo)
	if tracker == nil {
		return fmt.Errorf("no tracker for %s/%s", owner, repo)
	}

	// State gate: never route closed issues. This prevents the
	// claim-fail-requeue loop when a closed issue has status:queued.
	if issue.State == forge.StateClosed {
		d.logger.Info("skipping closed issue", zap.Int("issue", issue.Number))
		_ = tracker.RemoveLabel(ctx, issue.Number, "status:queued")
		return nil
	}

	// Pre-flight health gate: check if the routing chain has any healthy
	// provider before claiming the issue. This prevents the tight
	// claim-fail-requeue loop when all providers are down.
	if d.healthMonitor != nil && d.pool != nil {
		providerKey, _ := selectProviderKey(issue, agentType)
		routing := d.pool.RegistryRouting()
		chain, ok := routing[providerKey]
		if !ok {
			chain = routing["default"]
		}
		if len(chain) > 0 && !d.healthMonitor.ChainHealthy(chain) {
			d.logger.Warn("no healthy providers for chain, deferring issue",
				zap.Int("issue", issue.Number),
				zap.String("chain", providerKey),
				zap.Strings("providers", chain),
			)
			return nil
		}
	}

	// Human-typed issues are tracked but never submitted to the agent pool.
	if agentType == models.AgentTypeHuman {
		d.logger.Info("issue classified as human", zap.Int("issue", issue.Number))
		if err := tracker.AddLabel(ctx, issue.Number, "status:needs-human"); err != nil {
			d.logger.Error("add label", zap.Int("issue", issue.Number), zap.String("label", "needs-human"), zap.String("error", err.Error()))
		}
		return nil
	}

	// Phase gate: skip agent types not permitted for this project's lifecycle phase.
	// The issue remains status:queued and will be re-evaluated when the phase changes.
	if d.projects != nil {
		if phase, found := d.projects.PhaseFor(owner, repo); found {
			if !phaseAllowed(phase, agentType) {
				d.logger.Info("agent type blocked by project phase, leaving queued",
					zap.Int("issue", issue.Number),
					zap.String("agent", string(agentType)),
					zap.String("phase", phase),
				)
				return nil
			}
		}
	}

	// Check budget circuit breaker before any dispatching.
	if d.circuitBreaker != nil && !d.circuitBreaker.AllowDispatch() {
		d.logger.Warn("budget circuit open, not dispatching",
			zap.Int("issue", issue.Number),
		)
		return nil
	}

	if err := tracker.RemoveLabel(ctx, issue.Number, "status:queued"); err != nil {
		d.logger.Debug("remove queued label", zap.Int("issue", issue.Number), zap.String("error", err.Error()))
	}
	if err := tracker.AddLabel(ctx, issue.Number, "status:claimed"); err != nil {
		return fmt.Errorf("add claimed label to #%d: %w", issue.Number, err)
	}
	d.recordPipelineEvent(ctx, owner, repo, issue.Number, "status:queued", "status:claimed", "dispatcher")
	if err := tracker.Assign(ctx, issue.Number, string(agentType)); err != nil {
		// Best-effort: Gitea requires assignee to be a repo collaborator,
		// GitHub silently ignores invalid assignees. Don't block routing.
		d.logger.Warn("assign issue (non-fatal)", zap.Int("issue", issue.Number), zap.String("agent", string(agentType)), zap.Error(err))
	}

	providerKey, reason := selectProviderKey(issue, agentType)

	// Estimate per-issue timeout: use calibrated (historical) when available,
	// fall back to heuristic signals or frontmatter override.
	var timeout time.Duration
	if d.store != nil {
		timeout = CalibratedTimeout(ctx, d.store, d.logger, issue, fm, agentType, providerKey)
	} else {
		timeout = EstimateTimeout(issue, fm, agentType, providerKey)
	}
	d.logger.Info("timeout estimated",
		zap.Int("issue", issue.Number),
		zap.Duration("timeout", timeout),
	)

	now := time.Now()
	key := issueKey(owner, repo, issue.Number)
	// Use persisted failure count (survives restarts) with in-memory fallback.
	priorFailures := d.getPersistedFailureCount(ctx, issue.Number)
	d.mu.Lock()
	if memCount := d.issueFailures[key]; memCount > priorFailures {
		priorFailures = memCount // in-memory may be ahead if store write lagged
	}
	d.claimed[key] = &claimedIssue{
		AgentID:       string(agentType),
		Owner:         owner,
		Repo:          repo,
		ClaimedAt:     now,
		LastHeartbeat: now,
		FailureCount:  priorFailures,
	}
	d.mu.Unlock()

	broadcastEvent(d.broadcaster, "worker.claimed", map[string]any{
		"issue_number": issue.Number,
		"agent_type":   string(agentType),
	})

	d.metrics.IssueClaimed()
	d.metrics.IssueRouted()

	d.logger.Info("routed",
		zap.Int("issue", issue.Number),
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.String("agent", string(agentType)),
		zap.String("provider", providerKey),
		zap.String("reason", reason),
		zap.Int("attempt", priorFailures+1),
	)

	// Submit to agent pool if available.
	if d.pool != nil && d.store != nil {
		sessionID := fmt.Sprintf("sess_%d_%d", issue.Number, time.Now().Unix())
		session := &models.Session{
			ID:               sessionID,
			IssueNumber:      issue.Number,
			AgentType:        string(agentType),
			Status:           models.SessionStatusPending,
			EstimatedTimeout: timeout,
			StartedAt:        time.Now(),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := d.store.CreateSession(ctx, session); err != nil {
			return fmt.Errorf("create session for #%d: %w", issue.Number, err)
		}
		hbKey := key // capture for closure
		task := agent.Task{
			Issue:       issue,
			Tracker:     tracker,
			Owner:       owner,
			Repo:        repo,
			AgentType:   agentType,
			SessionID:   sessionID,
			ProviderKey: providerKey,
			Timeout:     timeout,
			Frontmatter: fm,
			HeartbeatFunc: func() {
				d.mu.Lock()
				if c, ok := d.claimed[hbKey]; ok {
					c.LastHeartbeat = time.Now()
				}
				d.mu.Unlock()
			},
		}
		if err := d.pool.Submit(task); err != nil {
			return fmt.Errorf("submit agent task for #%d: %w", issue.Number, err)
		}
		d.recordPipelineEvent(ctx, owner, repo, issue.Number, "status:claimed", "status:in-progress", "dispatcher")
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
