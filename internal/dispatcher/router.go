package dispatcher

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"strings"
	"time"

	"samverk.dev/samverk/internal/agent"
	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
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
	models.AgentTypeInfra:        true,
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
	if labels[models.LabelAgentHuman] {
		return models.AgentTypeHuman
	}
	if labels["type:spike"] || labels["type:research"] {
		return models.AgentTypeResearch
	}
	if labels["bug"] {
		return models.AgentTypeCodeGen
	}
	// Doc-review labels route to docs agent (always Claude, never Ollama).
	if labels[models.LabelDocStale] || labels[models.LabelDocInconsistent] ||
		labels[models.LabelDocStructure] || labels[models.LabelDocDecisionReview] {
		return models.AgentTypeDocs
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

// Complexity classification thresholds. These control when issues are routed
// to local (cheaper) vs cloud (more capable) model chains.
var (
	// complexityLocalMaxTokens is the upper bound for complexity:local classification.
	// Issues with estimated_tokens below this AND few files are routed to local models.
	complexityLocalMaxTokens = 10000

	// complexityLocalMaxFiles is the maximum file_context count for local classification.
	complexityLocalMaxFiles = 3

	// complexityCloudMinTokens is the lower bound for complexity:cloud classification.
	// Issues with estimated_tokens above this are always routed to cloud models.
	complexityCloudMinTokens = 30000

	// complexityCloudMinFiles is the minimum file_context count for cloud classification.
	// Issues with this many or more files are routed to cloud models regardless of tokens.
	complexityCloudMinFiles = 4
)

// classifyComplexity determines the complexity level based on estimated tokens and file context.
// Returns one of complexity:local, complexity:cloud, or complexity:ambiguous.
// Rules:
//   - complexity:local if estimated_tokens < localMaxTokens and file_context has <= localMaxFiles
//   - complexity:cloud if estimated_tokens > cloudMinTokens or file_context has >= cloudMinFiles
//   - complexity:ambiguous otherwise
func classifyComplexity(fm *models.IssueFrontmatter) string {
	if fm == nil {
		return models.LabelComplexityAmbiguous
	}

	hasSmallTokens := fm.EstimatedTokens > 0 && fm.EstimatedTokens < complexityLocalMaxTokens
	hasSmallFileContext := len(fm.FileContext) <= complexityLocalMaxFiles
	hasLargeTokens := fm.EstimatedTokens > complexityCloudMinTokens
	hasLargeFileContext := len(fm.FileContext) >= complexityCloudMinFiles

	if hasSmallTokens && hasSmallFileContext {
		return models.LabelComplexityLocal
	}
	if hasLargeTokens || hasLargeFileContext {
		return models.LabelComplexityCloud
	}
	return models.LabelComplexityAmbiguous
}

// complexityLabel returns the complexity label on the issue, or "none" if absent.
func complexityLabel(issue *forge.Issue) string {
	for _, label := range issue.Labels {
		if label == models.LabelComplexityLocal ||
			label == models.LabelComplexityCloud ||
			label == models.LabelComplexityAmbiguous {
			return label
		}
	}
	return "none"
}

// hasComplexityLabel checks if the issue already has a complexity label.
func hasComplexityLabel(issue *forge.Issue) bool {
	for _, label := range issue.Labels {
		if label == models.LabelComplexityLocal ||
			label == models.LabelComplexityCloud ||
			label == models.LabelComplexityAmbiguous {
			return true
		}
	}
	return false
}

// selectProviderKey examines issue signals and returns the routing chain key
// that should be used for provider selection, along with a human-readable reason.
//
// Priority (highest first):
//  0. agent-type overrides — docs/test agents have fixed chains regardless of title
//  1. complex  — critical priority, high complexity, complexity:cloud, or architectural title keywords
//  2. local    — boilerplate/scaffold labels, complexity:local, or "chore:" title prefix
//  3. triage   — low priority label or short prose body (< 200 words, excluding frontmatter)
//  4. default  — everything else
func selectProviderKey(issue *forge.Issue, agentType models.AgentType) (key, reason string) {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}
	lower := strings.ToLower(issue.Title)

	// Agent-type overrides: docs agents use the default chain (which falls back
	// to Claude Haiku). Triage chain (Ollama-only) has 100% failure rate for
	// docs because Ollama produces empty/too-short output for prose tasks.
	if agentType == models.AgentTypeDocs {
		return "default", "agent type docs"
	}

	// Research agent: tier-based chain selection from issue labels.
	// research:quick → triage (codebase scan only)
	// research:standard (default) → default (dependency mapping + feasibility)
	// research:deep → complex (full analysis, architecture options)
	if agentType == models.AgentTypeResearch {
		if labels["research:deep"] {
			return "complex", "research tier deep"
		}
		if labels["research:quick"] {
			return "triage", "research tier quick"
		}
		return "default", "research tier standard"
	}

	// Complex: critical priority, high complexity, complexity:cloud, or architectural title keywords.
	if labels[models.LabelPriorityCritical] {
		return "complex", "label " + models.LabelPriorityCritical
	}
	if labels["complexity:high"] {
		return "complex", "label complexity:high"
	}
	if labels[models.LabelComplexityCloud] {
		return "complex", "label " + models.LabelComplexityCloud
	}
	for _, kw := range complexTitleKeywords {
		if strings.Contains(lower, kw) {
			return "complex", "title keyword " + kw
		}
	}

	// Local: boilerplate/scaffold labels, complexity:local, or chore title prefix.
	if labels["type:boilerplate"] {
		return "local", "label type:boilerplate"
	}
	if labels["type:scaffold"] {
		return "local", "label type:scaffold"
	}
	if labels[models.LabelComplexityLocal] {
		return "local", "label " + models.LabelComplexityLocal
	}
	if strings.HasPrefix(lower, "chore:") {
		return "local", "title prefix chore:"
	}

	// QC: dedicated chain ensures cross-model validation (ADR-030).
	// Checked after complex/local so critical QC issues still get the complex chain.
	if agentType == models.AgentTypeQC {
		return "qc", "agent type qc (cross-model validation)"
	}

	// Code-gen, test, and infra agents require CLI-capable providers that can produce
	// file changes (EDIT blocks or worktree commits). Ollama providers return
	// prose only, wasting sessions (#269, #270). Route to "code-gen" chain
	// which contains only CLI providers.
	//
	// Placed after complex/local so critical/high-complexity code-gen issues
	// still get the complex chain (which is already CLI-only). But before
	// triage/default to prevent code-gen from falling into Ollama-first chains.
	if agentType == models.AgentTypeCodeGen || agentType == models.AgentTypeTest || agentType == models.AgentTypeInfra {
		return "code-gen", "agent type " + string(agentType) + " (requires CLI provider)"
	}

	// Triage: low priority or short prose body (excluding YAML frontmatter).
	if labels[models.LabelPriorityLow] {
		return "triage", "label " + models.LabelPriorityLow
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

// classifyHumanSubtype determines what kind of human action is needed.
// Returns one of: human:decision, human:review, human:credentials, or human:workstation.
// When in doubt, defaults to human:workstation (most conservative).
func classifyHumanSubtype(issue *forge.Issue) string {
	lower := strings.ToLower(issue.Title + " " + issue.Body)

	// Check for credentials/secrets keywords
	credentialsKeywords := []string{"api key", "api token", "secret", "password", "credentials", "token", "auth key"}
	for _, kw := range credentialsKeywords {
		if strings.Contains(lower, kw) {
			return models.LabelHumanCredentials
		}
	}

	// Check for workstation keywords
	workstationKeywords := []string{"ssh", "workstation", "terminal", "cli", "command line", "browser", "cloudflare", "dashboard", "physical access", "localhost"}
	for _, kw := range workstationKeywords {
		if strings.Contains(lower, kw) {
			return models.LabelHumanWorkstation
		}
	}

	// Check for review keywords
	reviewKeywords := []string{"review", "approve", "pr", "pull request", "merge", "reject"}
	hasReviewKeyword := false
	for _, kw := range reviewKeywords {
		if strings.Contains(lower, kw) {
			hasReviewKeyword = true
			break
		}
	}
	if hasReviewKeyword {
		return models.LabelHumanReview
	}

	// Check for decision keywords
	decisionKeywords := []string{"decide", "choice", "which", "approve", "go/no-go", "yes or no", "pick one", "select"}
	for _, kw := range decisionKeywords {
		if strings.Contains(lower, kw) {
			return models.LabelHumanDecision
		}
	}

	// Default to workstation (most conservative)
	return models.LabelHumanWorkstation
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
		_ = tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusQueued)
		return nil
	}

	// Quality warning: check for missing file_context or constraints.
	// This is advisory only and does not block dispatch.
	d.postQualityWarningIfNeeded(ctx, tracker, owner, repo, issue.Number, issue.Labels, fm)

	// Scope validation: reject oversized issues that will exhaust agent token
	// budgets without producing useful output. These need decomposition.
	if d.rejectOversizedScope(ctx, tracker, issue, fm) {
		return nil // issue escalated to needs-human; do not dispatch
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
		// Apply status:needs-human label
		if err := tracker.AddLabels(ctx, issue.Number, models.LabelStatusNeedsHuman); err != nil {
			d.logger.Error("add label", zap.Int("issue", issue.Number), zap.String("label", models.LabelStatusNeedsHuman), zap.String("error", err.Error()))
		}
		// Classify and apply human:* subtype label
		subtype := classifyHumanSubtype(issue)
		if err := tracker.AddLabels(ctx, issue.Number, subtype); err != nil {
			d.logger.Debug("add human subtype label (non-fatal)", zap.Int("issue", issue.Number), zap.String("label", subtype), zap.String("error", err.Error()))
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

	// Auto-classify complexity if no complexity label exists.
	if !hasComplexityLabel(issue) {
		complexityLabel := classifyComplexity(fm)
		if err := tracker.AddLabels(ctx, issue.Number, complexityLabel); err != nil {
			// Best-effort: log but don't block routing
			d.logger.Debug("add complexity label (non-fatal)", zap.Int("issue", issue.Number), zap.String("label", complexityLabel), zap.String("error", err.Error()))
		} else {
			// Update the in-memory issue with the new label for selectProviderKey
			issue.Labels = append(issue.Labels, complexityLabel)
		}
	}

	// Capacity gate: only claim when a pool worker is available to process
	// the task immediately. Without this, the dispatcher greedily claims all
	// queued issues into a buffer, starving PC workers that poll for
	// status:queued issues on a 60s interval. Leaving excess issues as
	// status:queued lets PC workers (and future poll cycles) pick them up.
	if d.pool != nil {
		snap := d.pool.Snapshot()
		if snap.IdleWorkers <= 0 && snap.QueueDepth > 0 {
			d.logger.Debug("capacity gate: no idle workers, leaving issue queued for PC workers or next cycle",
				zap.Int("issue", issue.Number),
				zap.Int("active", snap.ActiveWorkers),
				zap.Int("queue_depth", snap.QueueDepth),
			)
			return nil
		}
	}

	if err := tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusQueued); err != nil {
		d.logger.Debug("remove queued label", zap.Int("issue", issue.Number), zap.String("error", err.Error()))
	}
	if err := tracker.AddLabels(ctx, issue.Number, models.LabelStatusClaimed); err != nil {
		return fmt.Errorf("add claimed label to #%d: %w", issue.Number, err)
	}
	d.recordPipelineEvent(ctx, owner, repo, issue.Number, models.LabelStatusQueued, models.LabelStatusClaimed, "dispatcher")
	if err := tracker.Assign(ctx, issue.Number, string(agentType)); err != nil {
		// Best-effort: Gitea requires assignee to be a repo collaborator,
		// GitHub silently ignores invalid assignees. Don't block routing.
		d.logger.Warn("assign issue (non-fatal)", zap.Int("issue", issue.Number), zap.String("agent", string(agentType)), zap.Error(err))
	}

	providerKey, reason := selectProviderKey(issue, agentType)
	d.logger.Info("routing decision",
		zap.Int("issue", issue.Number),
		zap.String("agent_type", string(agentType)),
		zap.String("chain", providerKey),
		zap.String("reason", reason),
		zap.String("complexity", complexityLabel(issue)),
	)

	// Chain promotion: if the issue is old and routed to a free (Ollama) chain,
	// promote to a plan-based CLI chain when idle workers are available.
	if d.pool != nil && isOllamaChain(providerKey) {
		snap := d.pool.Snapshot()
		if promoted, ok := shouldPromoteChain(providerKey, issue, snap.IdleWorkers); ok {
			d.logger.Info("chain promotion: stale issue promoted to higher-capacity chain",
				zap.Int("issue", issue.Number),
				zap.String("from", providerKey),
				zap.String("to", promoted),
				zap.Int("idle_workers", snap.IdleWorkers),
			)
			providerKey = promoted
			reason = fmt.Sprintf("promoted from %s (issue age > 3d, idle workers available)", reason)
		}
	}

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
		AgentID:          string(agentType),
		Owner:            owner,
		Repo:             repo,
		ClaimedAt:        now,
		LastHeartbeat:    now,
		FailureCount:     priorFailures,
		EstimatedTimeout: timeout,
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
		d.recordPipelineEvent(ctx, owner, repo, issue.Number, models.LabelStatusClaimed, models.LabelStatusInProgress, "dispatcher")
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

// postQualityWarningIfNeeded checks if file_context or constraints are missing
// and posts a one-time warning comment if needed. Does not block dispatch.
// The warning:quality label is the decision surface -- it persists across
// restarts and costs zero API calls to check (read from issue_cache).
// Scope validation thresholds. Issues exceeding these are escalated to
// needs-human for decomposition rather than dispatched to agents.
var (
	// maxAcceptanceCriteria is the maximum number of acceptance criteria
	// checkboxes before an issue is considered oversized.
	maxAcceptanceCriteria = 5

	// maxFileContextForScope is the maximum file_context entries before
	// an issue is considered oversized (independent of complexity routing).
	maxFileContextForScope = 8
)

// rejectOversizedScope checks whether an issue exceeds agent scope limits.
// Returns true if the issue was escalated (caller should not dispatch).
// Returns false if the issue is within scope (caller proceeds normally).
//
// An issue already labeled needs-human or warning:scope is skipped to avoid
// duplicate comments on re-queue cycles.
func (d *Dispatcher) rejectOversizedScope(ctx context.Context, tracker forge.IssueTracker, issue *forge.Issue, fm *models.IssueFrontmatter) bool {
	// Skip if already escalated, flagged, or manually overridden.
	for _, l := range issue.Labels {
		if l == models.LabelStatusNeedsHuman || l == "warning:scope" || l == "no-decompose" {
			return false
		}
	}

	criteria := agent.ParseAcceptanceCriteria(issue.Body)
	fileCount := 0
	if fm != nil {
		fileCount = len(fm.FileContext)
	}

	var reasons []string
	if len(criteria) > maxAcceptanceCriteria {
		reasons = append(reasons, fmt.Sprintf("%d acceptance criteria (max %d)", len(criteria), maxAcceptanceCriteria))
	}
	if fileCount > maxFileContextForScope {
		reasons = append(reasons, fmt.Sprintf("%d file_context entries (max %d)", fileCount, maxFileContextForScope))
	}

	if len(reasons) == 0 {
		return false
	}

	comment := fmt.Sprintf(
		"**[dispatcher] Scope validation failed**: this issue exceeds agent scope limits (%s). "+
			"Agents will exhaust their token budget reading code before they can implement changes.\n\n"+
			"Please decompose this issue into smaller sub-issues (max %d acceptance criteria, max %d files each) "+
			"or add the `no-decompose` label if you want to force dispatch.",
		strings.Join(reasons, "; "), maxAcceptanceCriteria, maxFileContextForScope,
	)

	if _, err := tracker.AddComment(ctx, issue.Number, comment); err != nil {
		d.logger.Warn("scope validation: failed to post comment",
			zap.Int("issue", issue.Number),
			zap.Error(err),
		)
	}
	if err := tracker.AddLabels(ctx, issue.Number, models.LabelStatusNeedsHuman, "warning:scope"); err != nil {
		d.logger.Warn("scope validation: failed to add labels",
			zap.Int("issue", issue.Number),
			zap.Error(err),
		)
	}
	_ = tracker.RemoveLabel(ctx, issue.Number, models.LabelStatusQueued)

	d.logger.Info("issue rejected: scope too large",
		zap.Int("issue", issue.Number),
		zap.Int("acceptance_criteria", len(criteria)),
		zap.Int("file_context", fileCount),
		zap.Strings("reasons", reasons),
	)
	return true
}

func (d *Dispatcher) postQualityWarningIfNeeded(ctx context.Context, tracker forge.IssueTracker, owner, repo string, issueNumber int, issueLabels []string, fm *models.IssueFrontmatter) {
	if fm == nil {
		return // No frontmatter to check
	}

	// Check if either field is missing
	missingFileContext := len(fm.FileContext) == 0
	missingConstraints := len(fm.Constraints) == 0

	if !missingFileContext && !missingConstraints {
		return // Both fields present, no warning needed
	}

	// Check label (survives restarts, zero API cost).
	if labelSliceContains(issueLabels, models.LabelWarningQuality) {
		return // Warning already posted (label is the decision surface)
	}

	// Build warning message
	var missing []string
	if missingFileContext {
		missing = append(missing, "`file_context`")
	}
	if missingConstraints {
		missing = append(missing, "`constraints`")
	}
	missingStr := strings.Join(missing, " and ")

	comment := fmt.Sprintf(
		"**[dispatcher] Issue quality warning**: missing %s (agent will explore blind and/or have no guardrails). Consider adding these fields for better results.",
		missingStr,
	)

	if _, err := tracker.AddComment(ctx, issueNumber, comment); err != nil {
		d.logger.Debug("quality check: failed to post warning",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}

	// Dual-write: add warning:quality label for future reads.
	if err := tracker.AddLabels(ctx, issueNumber, models.LabelWarningQuality); err != nil {
		d.logger.Info("quality check: failed to add warning:quality label",
			zap.Int("issue", issueNumber),
			zap.Error(err),
		)
	}
}
