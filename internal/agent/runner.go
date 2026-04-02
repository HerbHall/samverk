package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/autonomy"
	"samverk.dev/samverk/internal/cost"
	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/internal/provider"
	"samverk.dev/samverk/internal/store"
	"samverk.dev/samverk/internal/synapset"
	"samverk.dev/samverk/pkg/models"
)

// heartbeatPulseInterval is how often the runner calls task.HeartbeatFunc
// during the blocking provider.Chat() call to prevent the dispatcher from
// treating the session as hung. Must be well below the dispatcher timeout
// (HeartbeatInterval × HeartbeatTimeoutMultiplier, default 30 min).
const heartbeatPulseInterval = 5 * time.Minute

// defaultProgressInterval is the default interval between mid-task PROGRESS
// comments. Set to 0 to disable periodic progress posting.
const defaultProgressInterval = 30 * time.Minute

// Runner executes a single agent task: sends the issue to an AI provider,
// records cost, and posts the response as an issue comment or opens a PR.
type Runner struct {
	provider         provider.Provider
	model            string
	tracker          forge.IssueTracker
	repoWriter       forge.RepoWriter
	prManager        forge.PullRequestManager
	store            store.Store
	costs            *cost.Tracker
	logger           *zap.Logger
	cleanupCtx       context.Context // used for session updates and failure comments; survives task timeout
	progressInterval time.Duration   // interval between PROGRESS posts; 0 disables
	synapset         *synapset.Client // optional; nil disables memory enrichment
	policy           autonomy.AutonomyPolicy // optional; nil disables tier enforcement
	repoDir          string // local repo path; when set, enables workspace isolation
}

// NewRunner creates a runner bound to a specific provider and model.
// The repoWriter and prManager are optional; when nil, code-gen/test agents
// fall back to posting comments instead of opening PRs.
func NewRunner(p provider.Provider, model string, tracker forge.IssueTracker, st store.Store, costs *cost.Tracker, logger *zap.Logger) *Runner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Runner{
		provider:         p,
		model:            model,
		tracker:          tracker,
		store:            st,
		costs:            costs,
		logger:           logger,
		progressInterval: defaultProgressInterval,
	}
}

// SetProgressInterval overrides the default interval between PROGRESS posts.
// A value of 0 disables periodic progress posting entirely.
func (r *Runner) SetProgressInterval(d time.Duration) {
	r.progressInterval = d
}

// SetRepoWriter configures write access for branch/file operations.
func (r *Runner) SetRepoWriter(rw forge.RepoWriter) {
	r.repoWriter = rw
}

// SetPRManager configures pull request operations.
func (r *Runner) SetPRManager(pm forge.PullRequestManager) {
	r.prManager = pm
}

// SetSynapset configures the Synapset memory client for context enrichment.
// When nil, memory enrichment is disabled.
func (r *Runner) SetSynapset(sc *synapset.Client) {
	r.synapset = sc
}

// SetPolicy configures the autonomy policy for tier enforcement.
// When nil, all actions are allowed (backward compatible).
func (r *Runner) SetPolicy(p autonomy.AutonomyPolicy) {
	r.policy = p
}

// SetRepoDir configures the local repository path for workspace isolation.
// When set, code-gen and test agents execute in isolated git worktrees.
func (r *Runner) SetRepoDir(dir string) {
	r.repoDir = dir
}

// Run processes a single task through the AI provider pipeline.
//
// Steps:
//  1. Update session status to "active"
//  2. Check budget -- if exceeded, mark "failed" and return
//  3. Detect prior checkpoint for resume
//  4. Build system prompt and chat request (with resume context if available)
//  5. Call provider.Chat
//  6. Record cost via cost tracker
//  7. Post response as issue comment (dedup edits against checkpoint)
//  8. Mark session "completed"
//
// On any error, the session is marked "failed" and an error comment is posted.
func (r *Runner) Run(ctx context.Context, task Task) error {
	r.logger.Info("task start",
		zap.Int("issue", task.Issue.Number),
		zap.String("session", task.SessionID),
		zap.String("agent", string(task.AgentType)),
		zap.String("provider", task.ProviderKey),
	)

	// Step 1: Mark session active.
	if err := r.updateSessionStatus(ctx, task.SessionID, models.SessionStatusActive, ""); err != nil {
		return fmt.Errorf("activate session: %w", err)
	}

	// Step 2: Check budget.
	if err := r.costs.CheckBudget(ctx); err != nil {
		if errors.Is(err, cost.ErrBudgetExceeded) {
			r.failTask(ctx, task, "budget exceeded: daily spend limit reached")
			return err
		}
		r.failTask(ctx, task, fmt.Sprintf("budget check failed: %v", err))
		return fmt.Errorf("check budget: %w", err)
	}

	// Step 3: Detect prior checkpoint for resume (reads from SQLite, not
	// issue comments -- see #516 for why comment scanning was removed).
	resumePrompt := r.detectCheckpoint(ctx, task)

	// Step 3b: Create isolated workspace for code-gen/test agents.
	var workDir string
	if r.repoDir != "" {
		switch task.AgentType {
		case models.AgentTypeCodeGen, models.AgentTypeTest:
			ws, wsCleanup, wsErr := CreateWorkspace(r.repoDir, task.SessionID, task.Issue.Number, r.logger)
			if wsErr != nil {
				r.logger.Warn("workspace creation failed; continuing without isolation",
					zap.Int("issue", task.Issue.Number),
					zap.Error(wsErr),
				)
			} else {
				workDir = ws
				defer wsCleanup()

				// Write MCP config and CLAUDE.md into the worktree.
				if mcpErr := WriteMCPConfig(workDir); mcpErr != nil {
					r.logger.Warn("failed to write MCP config to workspace",
						zap.Int("issue", task.Issue.Number),
						zap.Error(mcpErr),
					)
				}
				projectType := DetectProjectType(task.Issue.Labels)
				keyFiles := ExploreFileList(workDir, task.Issue.Body)
				claudeMD := GenerateAgentCLAUDEMD(projectType, task.Issue.Body, keyFiles...)
				if writeErr := os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte(claudeMD), 0o600); writeErr != nil {
					r.logger.Warn("failed to write CLAUDE.md to workspace",
						zap.Int("issue", task.Issue.Number),
						zap.Error(writeErr),
					)
				}
			}
		case models.AgentTypeDocs, models.AgentTypeResearch, models.AgentTypeQC,
			models.AgentTypeHuman, models.AgentTypeOrchestrator, models.AgentTypeDispatcher,
			models.AgentTypeInfra, models.AgentTypePC:
			// Non-code agents don't need workspace isolation.
		}
	}

	// Step 3c: Fetch Synapset patterns for API agent enrichment.
	var patterns []string
	if r.synapset != nil && task.Issue.Title != "" {
		var fetchErr error
		patterns, fetchErr = FetchRelevantPatterns(ctx, r.synapset, task.Issue.Title, maxMemoryResults)
		if fetchErr != nil {
			r.logger.Warn("failed to fetch relevant patterns",
				zap.Int("issue", task.Issue.Number),
				zap.Error(fetchErr),
			)
		}
	}

	// Step 3d: Explore repo context (CLAUDE.md, sibling files) before prompt building.
	// Frontmatter file_context paths are passed explicitly so the explorer reads
	// them first (highest priority), supplemented by regex discovery from issue body.
	exploreDir := workDir
	if exploreDir == "" {
		exploreDir = r.repoDir
	}
	var fmPaths []string
	if task.Frontmatter != nil {
		fmPaths = task.Frontmatter.FileContext
	}
	exploreCtx := ExploreContext(exploreDir, task.Issue.Body, fmPaths...)
	if len(exploreCtx) > 0 {
		r.logger.Info("explore phase complete",
			zap.Int("issue", task.Issue.Number),
			zap.Int("files_discovered", len(exploreCtx)),
		)
	}

	// Step 4: Build chat request.
	fileContext := r.extractFileContext(task.Issue.Body, workDir)
	// Merge explored context into fileContext (explore provides base context,
	// extractFileContext may override with workspace-specific versions).
	if len(exploreCtx) > 0 {
		if fileContext == nil {
			fileContext = make(map[string]string, len(exploreCtx))
		}
		for path, content := range exploreCtx {
			if existing, exists := fileContext[path]; !exists || existing == "" {
				fileContext[path] = content
			}
		}
	}
	systemPrompt := BuildSystemPrompt(task, fileContext, patterns...)
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: systemPrompt},
	}
	if resumePrompt != "" {
		messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: resumePrompt})
	}

	// Step 4b: Enrich with Synapset memory context (best-effort, non-fatal).
	if memoryContext := r.fetchMemoryContext(ctx, task); memoryContext != "" {
		messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: memoryContext})
	}

	// Step 4c: Include prior failure context so retries avoid the same mistakes.
	if r.store != nil {
		if priorErrors, fetchErr := r.store.RecentFailuresForIssue(ctx, task.Issue.Number, 3); fetchErr == nil && len(priorErrors) > 0 {
			var failCtx strings.Builder
			failCtx.WriteString("## Prior Failure Context\n\n")
			failCtx.WriteString("Previous attempts on this issue failed. Avoid these mistakes:\n\n")
			for i, msg := range priorErrors {
				fmt.Fprintf(&failCtx, "%d. %s\n", i+1, msg)
			}
			messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: failCtx.String()})
		}
	}

	// Step 4d: Include human comments added after issue creation.
	// Humans add clarifications, design decisions, and scope corrections via
	// comments. Without this, retried agents never see human feedback.
	if r.tracker != nil {
		if comments, fetchErr := r.tracker.ListComments(ctx, task.Issue.Number); fetchErr == nil && len(comments) > 0 {
			humanComments := filterHumanComments(comments)
			if len(humanComments) > 0 {
				var commentCtx strings.Builder
				commentCtx.WriteString("## Human Feedback\n\n")
				commentCtx.WriteString("These comments were added by humans after the issue was created. Follow any instructions or clarifications:\n\n")
				for _, c := range humanComments {
					fmt.Fprintf(&commentCtx, "### %s (%s)\n\n%s\n\n", c.Author, c.CreatedAt.Format("2006-01-02 15:04"), c.Body)
				}
				messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: commentCtx.String()})
			}
		}
	}

	messages = append(messages, provider.Message{Role: provider.RoleUser, Content: task.Issue.Body})
	req := provider.ChatRequest{
		Model:      r.model,
		Messages:   messages,
		WorkingDir: workDir,
		MaxTurns:   estimateMaxTurns(task),
	}

	// Step 5: Call provider (with heartbeat and streaming activity detection).
	//
	// Two heartbeat mechanisms work together:
	//   a) Ticker-based: fires every heartbeatPulseInterval as a fallback for
	//      providers that don't support ActivityNotifier.
	//   b) Activity-based: if the provider implements provider.ActivityNotifier,
	//      the callback fires on every chunk of output, resetting the heartbeat
	//      timer immediately. This prevents false timeout kills during long but
	//      active streaming sessions.
	{
		stop := make(chan struct{})

		if task.HeartbeatFunc != nil {
			task.HeartbeatFunc() // fire immediately before blocking call
		}

		if task.HeartbeatFunc != nil || r.progressInterval > 0 {
			go func() {
				heartTicker := time.NewTicker(heartbeatPulseInterval)
				defer heartTicker.Stop()

				var progressTicker *time.Ticker
				var progressCh <-chan time.Time
				if r.progressInterval > 0 {
					progressTicker = time.NewTicker(r.progressInterval)
					progressCh = progressTicker.C
					defer progressTicker.Stop()
				}

				var lastProgressHash string
				for {
					select {
					case <-heartTicker.C:
						if task.HeartbeatFunc != nil {
							task.HeartbeatFunc()
						}
					case <-progressCh:
						r.postProgress(ctx, task, &lastProgressHash)
					case <-stop:
						return
					}
				}
			}()
		}

		defer close(stop)
	}

	// Wire streaming activity detection if the provider supports it.
	if notifier, ok := r.provider.(provider.ActivityNotifier); ok {
		notifier.SetOnActivity(func() {
			if task.HeartbeatFunc != nil {
				task.HeartbeatFunc()
			}
		})
		// Clear the callback after Chat returns to avoid stale references.
		defer notifier.SetOnActivity(nil)
	}

	resp, err := r.provider.Chat(ctx, req)
	if err != nil {
		failureClass := "provider_error"
		if provider.IsRetryable(err) {
			failureClass = "timeout"
		}
		r.failTaskWithClass(ctx, task, fmt.Sprintf("provider error: %v", err), failureClass)
		return fmt.Errorf("provider chat: %w", err)
	}

	// Step 6: Record cost.
	if err = r.costs.RecordUsage(ctx, task.SessionID, r.provider.Name(), r.model, resp); err != nil {
		r.logger.Error("failed to record cost",
			zap.String("session_id", task.SessionID),
			zap.String("error", err.Error()),
		)
		// Non-fatal: continue even if cost recording fails.
	}

	// Step 6b: Check for token outlier (best-effort, non-fatal).
	if task.Frontmatter != nil && task.Frontmatter.EstimatedTokens > 0 {
		if warning := r.costs.CheckOutlier(ctx, task.Issue.Number, task.Frontmatter.EstimatedTokens); warning != "" {
			r.logger.Warn("token outlier detected",
				zap.Int("issue", task.Issue.Number),
				zap.String("warning", warning),
			)
			if _, commentErr := r.tracker.AddComment(ctx, task.Issue.Number, warning); commentErr != nil {
				r.logger.Error("failed to post outlier warning", zap.Error(commentErr))
			}
		}
	}

	// Step 6c: Validate output before posting (code-gen/test agents).
	// Build file context list for frontend validation checks.
	var valFileContext []string
	if task.Frontmatter != nil && len(task.Frontmatter.FileContext) > 0 {
		valFileContext = task.Frontmatter.FileContext
	} else if len(fileContext) > 0 {
		for path := range fileContext {
			valFileContext = append(valFileContext, path)
		}
	}
	valResult := ValidateBeforePost(ctx, task.AgentType, workDir, resp.Message.Content, valFileContext, task.Issue.Labels, r.logger)
	if valResult != nil && !valResult.Pass {
		if !valResult.Retryable {
			r.failTask(ctx, task, fmt.Sprintf("validation failed (not retryable): %s", valResult.Errors[0].Message))
			return fmt.Errorf("validation: %s", valResult.Errors[0].Message)
		}
		// Code error — retry once with validation errors as context.
		retryResp, retryErr := r.retryWithValidationErrors(ctx, task, req, workDir, valResult)
		if retryErr != nil {
			r.failTask(ctx, task, fmt.Sprintf("validation retry failed: %v", retryErr))
			return fmt.Errorf("validation retry: %w", retryErr)
		}
		resp = retryResp // use the retry response for postProcess
	}

	// Step 6d: Validate EDIT block format for code-gen/test agents (#269).
	// Ollama models often return prose instead of structured EDIT blocks.
	// Without this check, the response falls through to postComment() and
	// produces comment-only output with zero PRs.
	//
	// Only applies when: (a) no workspace (API flow, not CLI), and
	// (b) repo writer + PR manager are configured (EDIT blocks can create PRs).
	canUsePRPath := workDir == "" && r.repoWriter != nil && r.prManager != nil
	editVal := ValidateEditBlocks(task.AgentType, resp.Message.Content, !canUsePRPath)
	if editVal != nil && !editVal.Pass {
		r.logger.Warn("EDIT block validation failed, retrying with format guidance",
			zap.Int("issue", task.Issue.Number),
			zap.String("agent", string(task.AgentType)),
		)
		retryResp, retryErr := r.retryWithValidationErrors(ctx, task, req, workDir, editVal)
		if retryErr != nil {
			r.failTaskWithClass(ctx, task,
				fmt.Sprintf("format validation failed: agent did not produce EDIT blocks after retry: %v", retryErr),
				"format_error")
			return fmt.Errorf("EDIT block validation: %w", retryErr)
		}
		resp = retryResp
	}

	// Step 7: Post-process response based on agent type.
	if err = r.postProcess(ctx, task, resp.Message.Content, workDir); err != nil {
		r.failTask(ctx, task, fmt.Sprintf("post-process error: %v", err))
		return fmt.Errorf("post-process: %w", err)
	}

	// Step 7b: Store task outcome in Synapset memory (best-effort, non-fatal).
	r.storeTaskMemory(r.safeCtx(ctx), task, resp.Message.Content)

	// Step 7c: Store agent output in session for quality gate evaluation.
	if storeErr := r.storePartialOutput(r.safeCtx(ctx), task.SessionID, resp.Message.Content); storeErr != nil {
		r.logger.Warn("failed to store partial output", zap.Error(storeErr))
	}

	// Step 8: Mark session completed (use cleanup context to survive timeout).
	if err = r.completeSession(r.safeCtx(ctx), task.SessionID, resp.MaxTurnsHit, resp.TurnsUsed); err != nil {
		return fmt.Errorf("complete session: %w", err)
	}

	// Step 9: Update task profile (non-fatal; best-effort).
	if profErr := r.store.UpdateTaskProfile(ctx, string(task.AgentType), r.provider.Name()); profErr != nil {
		r.logger.Warn("failed to update task profile",
			zap.String("agent_type", string(task.AgentType)),
			zap.String("provider", r.provider.Name()),
			zap.String("error", profErr.Error()),
		)
	}

	// Step 9b: Log timeout accuracy (non-fatal, best-effort).
	r.logTimeoutAccuracy(ctx, task)

	return nil
}

// retryWithValidationErrors re-prompts the provider with validation error
// context, giving it one chance to self-correct. Costs are attributed to the
// same session. Returns the retry response or error.
func (r *Runner) retryWithValidationErrors(
	ctx context.Context,
	task Task,
	originalReq provider.ChatRequest,
	workDir string,
	valResult *ValidationResult,
) (*provider.ChatResponse, error) {
	// Build error context from validation errors.
	var errMsg strings.Builder
	errMsg.WriteString("Your previous output failed validation. Fix these errors:\n\n")
	for _, ve := range valResult.Errors {
		fmt.Fprintf(&errMsg, "## %s error\n%s\n", ve.Phase, ve.Message)
		if ve.Output != "" {
			fmt.Fprintf(&errMsg, "```\n%s\n```\n", ve.Output)
		}
	}

	// Append error context as a new user message.
	// Copy the original Messages slice to avoid mutating it.
	retryReq := originalReq
	retryReq.Messages = append(append([]provider.Message{}, originalReq.Messages...), provider.Message{
		Role:    provider.RoleUser,
		Content: errMsg.String(),
	})

	r.logger.Info("retrying with validation errors",
		zap.Int("issue", task.Issue.Number),
		zap.Int("error_count", len(valResult.Errors)),
	)

	// Re-call provider.
	retryResp, err := r.provider.Chat(ctx, retryReq)
	if err != nil {
		return nil, fmt.Errorf("retry provider call: %w", err)
	}

	// Record cost for retry (same session).
	if costErr := r.costs.RecordUsage(ctx, task.SessionID, r.provider.Name(), r.model, retryResp); costErr != nil {
		r.logger.Error("retry cost recording failed", zap.Error(costErr))
	}

	// Re-validate the retry output.
	// Build file context list for frontend validation checks (same as original validation).
	var retryFileContext []string
	if task.Frontmatter != nil && len(task.Frontmatter.FileContext) > 0 {
		retryFileContext = task.Frontmatter.FileContext
	} else {
		// Extract from issue body if no frontmatter
		issueFileContext := r.extractFileContext(task.Issue.Body, workDir)
		for path := range issueFileContext {
			retryFileContext = append(retryFileContext, path)
		}
	}
	retryVal := ValidateBeforePost(ctx, task.AgentType, workDir, retryResp.Message.Content, retryFileContext, task.Issue.Labels, r.logger)
	if retryVal != nil && !retryVal.Pass {
		var allErrors strings.Builder
		for i, ve := range retryVal.Errors {
			if i > 0 {
				allErrors.WriteString("; ")
			}
			fmt.Fprintf(&allErrors, "[%s] %s", ve.Phase, ve.Message)
		}
		return nil, fmt.Errorf("validation failed after retry (%d errors): %s", len(retryVal.Errors), allErrors.String())
	}

	return retryResp, nil
}

// postProgress posts a PROGRESS comment if the session has new partial output
// since the last progress post. Uses hash-based dedup to avoid duplicate posts.
func (r *Runner) postProgress(ctx context.Context, task Task, lastHash *string) {
	session, err := r.store.GetSession(ctx, task.SessionID)
	if err != nil || session.PartialOutput == "" {
		return
	}
	hash := HashCheckpoint(session.PartialOutput)
	if hash == *lastHash {
		return // no new content since last progress post
	}
	comment := FormatProgress(task.SessionID, session.PartialOutput)
	if _, err = r.tracker.AddComment(ctx, task.Issue.Number, comment); err != nil {
		r.logger.Error("progress: failed to post", zap.Error(err))
		return
	}
	*lastHash = hash
}

// postProcess routes the provider response to the appropriate output handler.
// When a workspace has changes (CLI provider flow), it commits, pushes, and
// creates a PR directly from the worktree. Otherwise, code-gen and test agents
// open PRs when EDIT blocks are present and forge write access is configured;
// all others post comments.
func (r *Runner) postProcess(ctx context.Context, task Task, response, workDir string) error {
	// CLI-based flow: if workspace has changes, commit and create PR.
	if workDir != "" {
		changed, err := r.commitWorkspaceChanges(ctx, task, workDir)
		if err != nil {
			return err
		}
		if changed {
			// Check doc gate: warn if code changed without doc updates.
			r.checkDocGate(ctx, task, workDir)
			// Post the agent's response as an informational comment.
			_ = r.postComment(ctx, task, response)
			return nil
		}
		// No workspace changes — fall through to standard post-processing.
	}

	switch task.AgentType {
	case models.AgentTypeCodeGen, models.AgentTypeTest:
		if r.repoWriter != nil && r.prManager != nil {
			parsed := ParseEditBlocks(response)
			// Dedup edits against prior checkpoint from SQLite (not
			// issue comments -- see #516).
			if checkpoint, cpErr := r.store.GetLatestCheckpoint(ctx, task.Issue.Number); cpErr == nil && checkpoint != "" {
				parsed.Edits = DeduplicateEdits(parsed.Edits, checkpoint)
			}
			if len(parsed.Edits) > 0 {
				return r.openPR(ctx, task, parsed)
			}
		}
		// No EDIT blocks or no write access — fall back to comment.
		return r.postComment(ctx, task, response)
	default:
		return r.postComment(ctx, task, response)
	}
}

// checkDocGate posts a warning comment if the session modified code/infra
// files without updating documentation. Best-effort: errors are logged but
// do not block the task.
func (r *Runner) checkDocGate(ctx context.Context, task Task, workDir string) {
	files, err := ChangedFiles(workDir)
	if err != nil {
		r.logger.Debug("doc gate: could not list changed files", zap.Error(err))
		return
	}
	needsDoc, codeFiles := CheckDocGate(files)
	if !needsDoc {
		return
	}
	warning := fmt.Sprintf(
		"DOC_GATE [dispatcher] [%s]\nThis session modified %d infrastructure/code file(s) without updating documentation:\n%s\nConsider updating docs/ or .samverk/status.md.",
		time.Now().UTC().Format(time.RFC3339),
		len(codeFiles),
		strings.Join(codeFiles, "\n"),
	)
	if _, commentErr := r.tracker.AddComment(ctx, task.Issue.Number, warning); commentErr != nil {
		r.logger.Warn("doc gate: failed to post warning", zap.Error(commentErr))
	}
}

// checkTier evaluates the autonomy policy for the given action. Returns nil
// if the action is allowed, ErrTierBlocked if blocked. When no policy is set,
// all actions are allowed (backward compatible).
func (r *Runner) checkTier(task Task, action autonomy.ActionType) error {
	if r.policy == nil {
		return nil
	}

	decision := autonomy.Evaluate(r.policy, action)

	switch decision.Verdict {
	case autonomy.VerdictAllow:
		r.logger.Debug("tier: allow",
			zap.Int("issue", task.Issue.Number),
			zap.String("action", string(action)),
			zap.String("tier", decision.Tier.String()),
		)
	case autonomy.VerdictAllowWithLog:
		r.logger.Info("tier: allow with audit log",
			zap.Int("issue", task.Issue.Number),
			zap.String("session", task.SessionID),
			zap.String("action", string(action)),
			zap.String("tier", decision.Tier.String()),
			zap.String("agent", string(task.AgentType)),
		)
	case autonomy.VerdictBlock:
		r.logger.Warn("tier: BLOCKED",
			zap.Int("issue", task.Issue.Number),
			zap.String("session", task.SessionID),
			zap.String("action", string(action)),
			zap.String("tier", decision.Tier.String()),
			zap.String("agent", string(task.AgentType)),
		)
		return fmt.Errorf("%w: %s requires %s", autonomy.ErrTierBlocked, action, decision.Tier)
	}

	return nil
}

// postComment posts the response as an issue comment.
func (r *Runner) postComment(ctx context.Context, task Task, response string) error {
	if err := r.checkTier(task, autonomy.ActionCommentIssue); err != nil {
		return err
	}
	if strings.TrimSpace(response) == "" {
		r.logger.Warn("skipping empty comment", zap.Int("issue", task.Issue.Number))
		return nil
	}
	if _, err := r.tracker.AddComment(ctx, task.Issue.Number, response); err != nil {
		return fmt.Errorf("add comment: %w", err)
	}
	return nil
}

// openPR creates a branch, writes files, and opens a pull request.
// Each step is gated by the autonomy policy tier check.
func (r *Runner) openPR(ctx context.Context, task Task, parsed *ParseResponse) error {
	branch := BranchSlug(task.Issue.Number, task.Issue.Title)

	// Tier check: create_branch (Tier 1 default).
	if err := r.checkTier(task, autonomy.ActionCreateBranch); err != nil {
		return err
	}
	if err := r.repoWriter.CreateBranch(ctx, branch); err != nil {
		return fmt.Errorf("create branch %q: %w", branch, err)
	}

	// Tier check: edit_file (Tier 2 default) — checked once for the batch.
	if err := r.checkTier(task, autonomy.ActionEditFile); err != nil {
		return err
	}
	for _, edit := range parsed.Edits {
		msg := fmt.Sprintf("agent: update %s for issue #%d", edit.Path, task.Issue.Number)
		if err := r.repoWriter.CreateOrUpdateFile(ctx, branch, edit.Path, edit.Content, msg); err != nil {
			return fmt.Errorf("write file %q: %w", edit.Path, err)
		}
	}

	// Tier check: create_pr (Tier 2 default).
	if err := r.checkTier(task, autonomy.ActionCreatePR); err != nil {
		return err
	}
	prTitle := parsed.PRTitle
	if prTitle == "" {
		prTitle = fmt.Sprintf("agent: resolve issue #%d", task.Issue.Number)
	}
	pr, err := r.prManager.CreatePullRequest(ctx, &forge.CreatePRRequest{
		Title: prTitle,
		Body:  fmt.Sprintf("Closes #%d\n\nAgent-generated implementation.", task.Issue.Number),
		Head:  branch,
		Base:  "main",
	})
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	// Post comment on issue with PR link (tier check inside postComment).
	comment := fmt.Sprintf("PR opened: #%d", pr.Number)
	if _, err = r.tracker.AddComment(ctx, task.Issue.Number, comment); err != nil {
		r.logger.Error("failed to post PR link comment",
			zap.String("session_id", task.SessionID),
			zap.Int("issue", task.Issue.Number),
			zap.String("error", err.Error()),
		)
	}

	return nil
}

// commitWorkspaceChanges checks for changes in the workspace, commits them,
// pushes the branch, and creates a PR. Returns true if changes were committed.
func (r *Runner) commitWorkspaceChanges(ctx context.Context, task Task, workDir string) (bool, error) {
	// Tier checks for branch and file operations.
	if err := r.checkTier(task, autonomy.ActionCreateBranch); err != nil {
		return false, err
	}
	if err := r.checkTier(task, autonomy.ActionEditFile); err != nil {
		return false, err
	}

	msg := fmt.Sprintf("agent: resolve issue #%d", task.Issue.Number)
	changed, err := CommitAndPush(workDir, msg)
	if err != nil {
		return false, fmt.Errorf("workspace commit: %w", err)
	}
	if !changed {
		return false, nil
	}

	r.logger.Info("workspace changes committed and pushed",
		zap.Int("issue", task.Issue.Number),
		zap.String("session", task.SessionID),
	)

	// Tier check for creating PR.
	if prErr := r.checkTier(task, autonomy.ActionCreatePR); prErr != nil {
		return true, prErr
	}

	if r.prManager != nil {
		branch := fmt.Sprintf("agent/%d", task.Issue.Number)
		pr, prErr := r.prManager.CreatePullRequest(ctx, &forge.CreatePRRequest{
			Title: fmt.Sprintf("agent: resolve issue #%d", task.Issue.Number),
			Body:  fmt.Sprintf("Closes #%d\n\nAgent-generated implementation.", task.Issue.Number),
			Head:  branch,
			Base:  "main",
		})
		if prErr != nil {
			return true, fmt.Errorf("create PR: %w", prErr)
		}
		comment := fmt.Sprintf("PR opened: #%d", pr.Number)
		if _, commentErr := r.tracker.AddComment(ctx, task.Issue.Number, comment); commentErr != nil {
			r.logger.Error("failed to post PR link",
				zap.Int("issue", task.Issue.Number),
				zap.Error(commentErr),
			)
		}
	}

	return true, nil
}

// filePathRe matches file paths in issue bodies that look like project source files.
// It handles paths in prose (preceded by whitespace) and in YAML frontmatter lists
// (preceded by "- "). Recognized directory prefixes cover all standard project paths.
var filePathRe = regexp.MustCompile(`(?:^|[\s\-])((?:internal|cmd|pkg|docs|scripts|web|overlay|\.samverk|\.github|\.gitea)/[\w/.\-]+\.\w+)`)

// extractFileContext scans the issue body for file paths matching project
// source directories and returns a map of path to content. When a workspace
// or repo directory is available, actual file contents are read (up to the
// maxFileContextBytes cap). Otherwise, paths are recorded with empty content.
func (r *Runner) extractFileContext(body, workDir string) map[string]string {
	matches := filePathRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate paths.
	seen := make(map[string]bool, len(matches))
	var paths []string
	for _, m := range matches {
		path := strings.TrimSpace(m[1])
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}

	// If we have a directory, read actual file contents.
	dir := workDir
	if dir == "" {
		dir = r.repoDir
	}
	if dir != "" {
		return ReadWorkspaceFiles(dir, paths, maxFileContextBytes)
	}

	// No directory available — return paths with empty content.
	result := make(map[string]string, len(paths))
	for _, p := range paths {
		result[p] = ""
	}
	return result
}

// agentCommentPrefixes identifies comments posted by samverk agents (dispatcher,
// QC, correction engine, etc.) rather than humans. These are filtered out when
// building the human feedback prompt section.
var agentCommentPrefixes = []string{
	"EDIT ",           // agent code output
	"PR_TITLE:",       // agent PR title
	"## QC Review:",   // QC agent verdict
	"ESCALATE",        // dispatcher escalation
	"RELEASE",         // dispatcher release
	"PROGRESS",        // runner progress update
	"TIMEOUT",         // heartbeat timeout
	"CORRECTION",      // correction engine
	"[dispatcher]",    // dispatcher system comments
	"[auto-apply]",    // auto-apply comments
}

// filterHumanComments returns only comments that were written by humans,
// excluding agent-generated comments (dispatcher, QC, correction engine, etc.).
func filterHumanComments(comments []*forge.Comment) []*forge.Comment {
	var human []*forge.Comment
	for _, c := range comments {
		if c.Body == "" {
			continue
		}
		isAgent := false
		for _, prefix := range agentCommentPrefixes {
			if strings.HasPrefix(strings.TrimSpace(c.Body), prefix) {
				isAgent = true
				break
			}
		}
		if !isAgent {
			human = append(human, c)
		}
	}
	return human
}

// detectCheckpoint queries SQLite for the most recent checkpoint on this issue.
// Zero allocations from issue comments -- only a single string from the DB.
func (r *Runner) detectCheckpoint(ctx context.Context, task Task) string {
	checkpoint, err := r.store.GetLatestCheckpoint(ctx, task.Issue.Number)
	if err != nil {
		r.logger.Warn("failed to query checkpoint from store",
			zap.Int("issue", task.Issue.Number),
			zap.Error(err),
		)
		return ""
	}
	if checkpoint != "" {
		r.logger.Info("resuming from checkpoint",
			zap.Int("issue", task.Issue.Number),
			zap.String("session", task.SessionID),
		)
		return BuildResumePrompt(checkpoint)
	}
	return ""
}

// updateSessionStatus fetches and updates a session's status in the store.
func (r *Runner) updateSessionStatus(ctx context.Context, sessionID string, status models.SessionStatus, errMsg string) error {
	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Status = status
	session.Error = errMsg
	session.UpdatedAt = time.Now()

	return r.store.UpdateSession(ctx, session)
}

// storePartialOutput saves agent output to the session for quality gate evaluation.
// Output is capped to prevent unbounded SQLite growth and in-memory bloat when
// sessions are loaded.
const maxPartialOutputBytes = 256 * 1024 // 256 KB per session

func (r *Runner) storePartialOutput(ctx context.Context, sessionID, output string) error {
	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(output) > maxPartialOutputBytes {
		output = output[len(output)-maxPartialOutputBytes:]
	}
	session.PartialOutput = output
	session.UpdatedAt = time.Now()
	return r.store.UpdateSession(ctx, session)
}

// estimateMaxTurns computes an adaptive max-turns value based on task signals.
// Each agentic turn is roughly one tool call + response. More complex tasks
// need more turns for reading files, making edits, running builds, and fixing
// errors. The base of 25 matches simple issues; complex ones can get up to 75.
func estimateMaxTurns(task Task) int {
	turns := 25 // base for simple tasks

	if task.Frontmatter != nil {
		// More files to read/modify = more tool calls needed.
		fc := len(task.Frontmatter.FileContext)
		if fc > 3 {
			turns += 10
		}
		if fc > 6 {
			turns += 10
		}

		// Higher token estimate = more complex work.
		if task.Frontmatter.EstimatedTokens > 15000 {
			turns += 10
		}
		if task.Frontmatter.EstimatedTokens > 30000 {
			turns += 10
		}
	}

	// Provider chain hints at complexity.
	switch task.ProviderKey {
	case "complex":
		turns += 15
	case "code-gen":
		turns += 5
	}

	// Cap at 75 to prevent runaway sessions.
	if turns > 75 {
		turns = 75
	}
	return turns
}

// completeSession marks a session as completed with a finish timestamp and stores
// max-turns metadata from the provider response.
func (r *Runner) completeSession(ctx context.Context, sessionID string, maxTurnsHit bool, turnsUsed int) error {
	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	now := time.Now()
	session.Status = models.SessionStatusCompleted
	session.FinishedAt = &now
	session.UpdatedAt = now
	session.MaxTurnsHit = maxTurnsHit
	session.TurnsUsed = turnsUsed

	return r.store.UpdateSession(ctx, session)
}

// safeCtx returns the cleanup context if set, otherwise the provided context.
// This ensures session updates and failure comments are not cancelled by the
// task timeout deadline.
func (r *Runner) safeCtx(ctx context.Context) context.Context {
	if r.cleanupCtx != nil {
		return r.cleanupCtx
	}
	return ctx
}

// failTask marks the session as failed and posts an error comment on the issue.
// If the session has partial output, a CHECKPOINT comment is posted first so
// that a retry can resume from where the previous attempt left off.
func (r *Runner) failTask(ctx context.Context, task Task, errMsg string) {
	r.failTaskWithClass(ctx, task, errMsg, string(models.FailureClassUnknown))
}

// failTaskWithClass marks the session as failed, records a FailureEvent with
// the given failure class, and posts an error comment on the issue.
func (r *Runner) failTaskWithClass(ctx context.Context, task Task, errMsg, failureClass string) {
	ctx = r.safeCtx(ctx)

	// Attempt to save checkpoint from partial output.
	r.saveCheckpoint(ctx, task)

	if err := r.updateSessionStatus(ctx, task.SessionID, models.SessionStatusFailed, errMsg); err != nil {
		r.logger.Error("failed to update session on error",
			zap.String("session_id", task.SessionID),
			zap.String("error", err.Error()),
		)
	}

	// Record structured failure event for analysis.
	fe := &models.FailureEvent{
		ID:           task.SessionID + "-fail",
		IssueNumber:  task.Issue.Number,
		SessionID:    task.SessionID,
		FailureClass: models.FailureClass(failureClass),
		ErrorMessage: errMsg,
		AgentType:    string(task.AgentType),
		Provider:     r.provider.Name(),
		Timestamp:    time.Now(),
	}
	if saveErr := r.store.SaveFailureEvent(ctx, fe); saveErr != nil {
		r.logger.Error("failed to save failure event",
			zap.String("session_id", task.SessionID),
			zap.String("error", saveErr.Error()),
		)
	}

	comment := fmt.Sprintf("Agent error: %s", errMsg)
	if _, err := r.tracker.AddComment(ctx, task.Issue.Number, comment); err != nil {
		r.logger.Error("failed to post error comment",
			zap.String("session_id", task.SessionID),
			zap.Int("issue", task.Issue.Number),
			zap.String("error", err.Error()),
		)
	}
}

// logTimeoutAccuracy compares the estimated timeout against actual session
// duration and logs the ratio. This data feeds into timeout calibration (#246).
func (r *Runner) logTimeoutAccuracy(ctx context.Context, task Task) {
	session, err := r.store.GetSession(ctx, task.SessionID)
	if err != nil {
		return
	}
	if session.EstimatedTimeout <= 0 || session.FinishedAt == nil {
		return
	}
	actual := session.FinishedAt.Sub(session.StartedAt)
	ratio := float64(actual) / float64(session.EstimatedTimeout)
	r.logger.Info("timeout accuracy",
		zap.Int("issue", task.Issue.Number),
		zap.Duration("estimated", session.EstimatedTimeout),
		zap.Duration("actual", actual),
		zap.Float64("ratio", ratio),
	)
}

// maxMemoryResults is the number of Synapset results to include in the prompt.
const maxMemoryResults = 5

// fetchMemoryContext queries Synapset for relevant learnings and returns a
// prompt section. Returns empty string if Synapset is not configured or the
// search fails.
func (r *Runner) fetchMemoryContext(ctx context.Context, task Task) string {
	if r.synapset == nil {
		return ""
	}

	query := task.Issue.Title
	if query == "" {
		return ""
	}

	// Search the devkit pool for patterns/gotchas relevant to this task.
	memories, err := r.synapset.SearchMemory(ctx, "devkit", query, maxMemoryResults)
	if err != nil {
		r.logger.Warn("synapset: devkit search failed",
			zap.Int("issue", task.Issue.Number),
			zap.Error(err),
		)
		return ""
	}

	// Also search the agent's own pool for prior learnings.
	agentMemories, err := r.synapset.SearchMemory(ctx, r.synapset.Pool(), query, maxMemoryResults)
	if err != nil {
		r.logger.Warn("synapset: agent pool search failed",
			zap.Int("issue", task.Issue.Number),
			zap.Error(err),
		)
		// Continue with devkit results only.
	} else {
		memories = append(memories, agentMemories...)
	}

	if len(memories) == 0 {
		return ""
	}

	// Cap at maxMemoryResults total.
	if len(memories) > maxMemoryResults {
		memories = memories[:maxMemoryResults]
	}

	var b strings.Builder
	b.WriteString("## Relevant Learnings from Past Sessions\n\n")
	for i := range memories {
		m := &memories[i]
		fmt.Fprintf(&b, "- [%s] %s\n", m.Category, m.Content)
	}
	return b.String()
}

// storeTaskMemory saves a summary of the completed task to Synapset for
// future reference. Best-effort: failures are logged but do not affect
// the task outcome.
func (r *Runner) storeTaskMemory(ctx context.Context, task Task, response string) {
	if r.synapset == nil {
		return
	}

	// Build a concise summary from the task and response.
	summary := fmt.Sprintf("Issue #%d (%s): %s",
		task.Issue.Number, task.AgentType, task.Issue.Title)
	if len(response) > 200 {
		summary += "\nOutcome: " + response[:200] + "..."
	} else if response != "" {
		summary += "\nOutcome: " + response
	}

	category := "completion"
	tags := []string{
		string(task.AgentType),
		fmt.Sprintf("issue-%d", task.Issue.Number),
	}
	source := fmt.Sprintf("session:%s", task.SessionID)

	if err := r.synapset.StoreMemory(ctx, r.synapset.Pool(), summary, category, tags, source); err != nil {
		r.logger.Warn("synapset: store memory failed",
			zap.Int("issue", task.Issue.Number),
			zap.String("session", task.SessionID),
			zap.Error(err),
		)
	}
}

// saveCheckpoint posts a CHECKPOINT comment on the issue if the session has
// non-empty partial output and the checkpoint hash differs from the previous one.
func (r *Runner) saveCheckpoint(ctx context.Context, task Task) {
	session, err := r.store.GetSession(ctx, task.SessionID)
	if err != nil {
		r.logger.Error("checkpoint: failed to get session",
			zap.String("session_id", task.SessionID),
			zap.Error(err),
		)
		return
	}

	if session.PartialOutput == "" {
		return
	}

	hash := HashCheckpoint(session.PartialOutput)
	if hash == session.CheckpointHash {
		// Same content as the last checkpoint — skip posting a duplicate.
		return
	}

	comment := FormatCheckpoint(task.SessionID, session.PartialOutput)
	if _, err = r.tracker.AddComment(ctx, task.Issue.Number, comment); err != nil {
		r.logger.Error("checkpoint: failed to post comment",
			zap.String("session_id", task.SessionID),
			zap.Int("issue", task.Issue.Number),
			zap.Error(err),
		)
		return
	}

	// Persist the hash so future retries can detect duplicates.
	session.CheckpointHash = hash
	if err = r.store.UpdateSession(ctx, session); err != nil {
		r.logger.Error("checkpoint: failed to update session hash",
			zap.String("session_id", task.SessionID),
			zap.Error(err),
		)
	}
}
