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
func validateWorktree(ctx context.Context, workDir string, logger *zap.Logger) *ValidationResult {
	result := &ValidationResult{Pass: true, Retryable: true}

	// Build check.
	buildOut, buildErr := runInDir(ctx, workDir, "go", "build", "./...")
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

// truncate returns at most maxLen characters from s.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
