package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

// ValidationResult reports the outcome of pre-posting validation.
type ValidationResult struct {
	Pass      bool
	Retryable bool // false for infrastructure failures; true for code errors
	Errors    []ValidationError
	Duration  time.Duration
}

// ValidationError describes a single validation failure.
type ValidationError struct {
	Phase   string // "build", "test", "format"
	Message string
	Output  string // truncated command output (max 2000 chars)
}

// maxValidationOutput is the maximum length of command output stored in a ValidationError.
const maxValidationOutput = 2000

// ValidateBeforePost checks agent output quality before posting.
// For code-gen/test agents with a worktree (CLI flow): runs build/test on the worktree.
// For code-gen/test agents without a worktree (API flow): skips (no local context to validate).
// For analytical agents: checks format only.
// Returns nil when no validation is needed.
func ValidateBeforePost(ctx context.Context, agentType models.AgentType, workDir, response string, logger *zap.Logger) *ValidationResult {
	if logger == nil {
		logger = zap.NewNop()
	}

	start := time.Now()

	switch agentType {
	case models.AgentTypeCodeGen, models.AgentTypeTest:
		if workDir != "" {
			// Check for bad output (config-only changes) before running build/test.
			// This is a non-retryable failure -- retrying won't help if the model
			// fundamentally misunderstood the task (e.g., Ollama overwriting CLAUDE.md).
			if badResult := ValidateWorkspaceOutput(workDir, logger); badResult != nil {
				badResult.Duration = time.Since(start)
				return badResult
			}
			result := validateWorktree(ctx, workDir, logger)
			result.Duration = time.Since(start)
			return result
		}
		// API flow without worktree: no local context to validate.
		// This is an accepted degradation -- validation requires a worktree.
		return nil

	case models.AgentTypeDocs, models.AgentTypeResearch, models.AgentTypeQC,
		models.AgentTypeHuman, models.AgentTypeOrchestrator, models.AgentTypeDispatcher,
		models.AgentTypeInfra, models.AgentTypePC:
		result := validateAnalytical(response)
		result.Duration = time.Since(start)
		return result

	default:
		return nil
	}
}

// validateWorktree runs `go build ./...` and `go test ./...` in the worktree.
// If the `go` tool is not installed on the host, validation is skipped gracefully.
func validateWorktree(ctx context.Context, workDir string, logger *zap.Logger) *ValidationResult {
	result := &ValidationResult{Pass: true, Retryable: true}

	// Check if 'go' is available. On deployment hosts (CT 202/203) Go may not
	// be installed since the binary is cross-compiled on the dev machine.
	if _, err := exec.LookPath("go"); err != nil {
		logger.Info("validation: skipping worktree checks (go not available)",
			zap.String("workDir", workDir),
		)
		return result // Pass by default when tool is missing.
	}

	// Build check. -buildvcs=false prevents "error obtaining VCS status" when
	// the worktree directory is not inside a git repository (e.g. temp dirs in tests).
	buildOut, buildErr := runInDir(ctx, workDir, "go", "build", "-buildvcs=false", "./...")
	if buildErr != nil {
		logger.Warn("validation: build failed",
			zap.String("workDir", workDir),
			zap.String("output", truncate(buildOut, maxValidationOutput)),
		)
		result.Pass = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "build",
			Message: fmt.Sprintf("go build failed: %v", buildErr),
			Output:  truncate(buildOut, maxValidationOutput),
		})
		return result // Don't run tests if build fails.
	}

	// Test check.
	testOut, testErr := runInDir(ctx, workDir, "go", "test", "./...")
	if testErr != nil {
		logger.Warn("validation: tests failed",
			zap.String("workDir", workDir),
			zap.String("output", truncate(testOut, maxValidationOutput)),
		)
		result.Pass = false
		result.Errors = append(result.Errors, ValidationError{
			Phase:   "test",
			Message: fmt.Sprintf("go test failed: %v", testErr),
			Output:  truncate(testOut, maxValidationOutput),
		})
	}

	return result
}

// validateAnalytical checks that analytical agent output is non-empty and reasonable.
func validateAnalytical(response string) *ValidationResult {
	trimmed := strings.TrimSpace(response)
	if len(trimmed) < 50 {
		return &ValidationResult{
			Pass:      false,
			Retryable: true,
			Errors: []ValidationError{
				{
					Phase:   "format",
					Message: fmt.Sprintf("response too short (%d chars, minimum 50)", len(trimmed)),
				},
			},
		}
	}
	return &ValidationResult{Pass: true}
}

// runInDir executes a command in the given directory and returns combined output.
// Uses a 120-second timeout to prevent hung builds from blocking the pipeline.
func runInDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: args are internally constructed
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// configOnlyPatterns are file path patterns that indicate an agent only modified
// project configuration files instead of implementing the actual feature.
var configOnlyPatterns = []string{
	"CLAUDE.md",
	".claude/",
	".samverk/",
	".github/",
	".gitignore",
	".editorconfig",
}

// IsBadOutput returns true when all changed files match config-only patterns,
// indicating the agent overwrote project config instead of implementing the feature.
// Returns false if changedFiles is empty (no changes is handled elsewhere).
func IsBadOutput(changedFiles []string) bool {
	if len(changedFiles) == 0 {
		return false
	}
	for _, f := range changedFiles {
		if !isConfigOnlyFile(f) {
			return false
		}
	}
	return true
}

// isConfigOnlyFile returns true if the file path matches a config-only pattern.
func isConfigOnlyFile(path string) bool {
	for _, pattern := range configOnlyPatterns {
		if strings.HasSuffix(pattern, "/") {
			// Directory prefix match.
			if strings.HasPrefix(path, pattern) {
				return true
			}
		} else {
			// Exact filename match (anywhere in path).
			if path == pattern || strings.HasSuffix(path, "/"+pattern) {
				return true
			}
		}
	}
	return false
}

// ValidateWorkspaceOutput checks whether a code-gen agent's workspace changes
// are meaningful. If the agent only modified config files (CLAUDE.md, .claude/,
// etc.), it returns a non-retryable validation failure. This catches Ollama
// models that overwrite CLAUDE.md instead of implementing features.
func ValidateWorkspaceOutput(workDir string, logger *zap.Logger) *ValidationResult {
	if workDir == "" {
		return nil
	}

	files, err := ChangedFiles(workDir)
	if err != nil {
		// If we can't list files, don't block -- other validators will catch real issues.
		if logger != nil {
			logger.Debug("bad output check: could not list changed files", zap.Error(err))
		}
		return nil
	}

	if len(files) == 0 {
		return nil // No changes is handled by the caller.
	}

	if IsBadOutput(files) {
		msg := fmt.Sprintf(
			"output rejected: agent only modified project config files (%s) instead of implementing the feature",
			strings.Join(files, ", "),
		)
		if logger != nil {
			logger.Warn("bad output detected",
				zap.Strings("changed_files", files),
				zap.String("failure_class", "bad-output"),
			)
		}
		return &ValidationResult{
			Pass:      false,
			Retryable: false,
			Errors: []ValidationError{
				{
					Phase:   "output-quality",
					Message: msg,
				},
			},
		}
	}

	return nil
}

// ValidateEditBlocks checks that a code-gen/test agent response contains at
// least one EDIT/END block. Ollama models often return prose or markdown code
// blocks instead of the required EDIT format, resulting in comment-only output
// with zero PRs. This validation catches that and triggers a format retry.
//
// Only applies when: (a) agent is code-gen or test, (b) no workspace directory
// is set (API flow). When a workspace exists, the CLI flow handles changes via
// the filesystem, not EDIT blocks. Returns nil when validation is not applicable.
func ValidateEditBlocks(agentType models.AgentType, response string, hasWorkDir bool) *ValidationResult {
	// Only validate code-gen and test agents.
	switch agentType {
	case models.AgentTypeCodeGen, models.AgentTypeTest:
		// OK, continue.
	default:
		return nil
	}

	// If a workspace directory exists, the CLI flow handles changes via
	// the filesystem, not EDIT blocks. Skip validation.
	if hasWorkDir {
		return nil
	}

	parsed := ParseEditBlocks(response)
	if len(parsed.Edits) > 0 {
		return nil // Has EDIT blocks, all good.
	}

	return &ValidationResult{
		Pass:      false,
		Retryable: true,
		Errors: []ValidationError{
			{
				Phase:   "format",
				Message: "response contains no EDIT blocks.\n\n" + FormatInstructions(),
			},
		},
	}
}

// truncate returns at most maxLen characters from s.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
