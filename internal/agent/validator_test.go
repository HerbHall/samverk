package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/pkg/models"
)

func TestValidateBeforePost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentType models.AgentType
		workDir   string
		response  string
		wantNil   bool
		wantPass  bool
	}{
		{
			name:      "code-gen with empty workDir returns nil",
			agentType: models.AgentTypeCodeGen,
			workDir:   "",
			response:  "some code output",
			wantNil:   true,
		},
		{
			name:      "test with empty workDir returns nil",
			agentType: models.AgentTypeTest,
			workDir:   "",
			response:  "some test output",
			wantNil:   true,
		},
		{
			name:      "docs agent validates as analytical with long response",
			agentType: models.AgentTypeDocs,
			response:  strings.Repeat("a", 60),
			wantPass:  true,
		},
		{
			name:      "research agent validates as analytical with long response",
			agentType: models.AgentTypeResearch,
			response:  strings.Repeat("a", 60),
			wantPass:  true,
		},
		{
			name:      "qc agent validates as analytical with short response",
			agentType: models.AgentTypeQC,
			response:  "short",
			wantPass:  false,
		},
		{
			name:      "human agent validates as analytical with empty response",
			agentType: models.AgentTypeHuman,
			response:  "",
			wantPass:  false,
		},
		{
			name:      "orchestrator validates as analytical",
			agentType: models.AgentTypeOrchestrator,
			response:  strings.Repeat("x", 50),
			wantPass:  true,
		},
		{
			name:      "dispatcher validates as analytical",
			agentType: models.AgentTypeDispatcher,
			response:  strings.Repeat("x", 50),
			wantPass:  true,
		},
		{
			name:      "infra validates as analytical",
			agentType: models.AgentTypeInfra,
			response:  strings.Repeat("x", 50),
			wantPass:  true,
		},
		{
			name:      "pc validates as analytical",
			agentType: models.AgentTypePC,
			response:  strings.Repeat("x", 50),
			wantPass:  true,
		},
		{
			name:      "unknown agent type returns nil",
			agentType: models.AgentType("custom"),
			response:  "anything",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ValidateBeforePost(context.Background(), tt.agentType, tt.workDir, tt.response, nil)
			if tt.wantNil {
				if result != nil {
					t.Fatalf("expected nil result, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			}
			if result.Pass != tt.wantPass {
				t.Errorf("Pass = %v, want %v", result.Pass, tt.wantPass)
			}
		})
	}
}

func TestValidateAnalytical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
		wantPass bool
		wantErr  string
	}{
		{
			name:     "empty response fails",
			response: "",
			wantPass: false,
			wantErr:  "response too short (0 chars, minimum 50)",
		},
		{
			name:     "whitespace-only response fails",
			response: "   \n\t  ",
			wantPass: false,
			wantErr:  "response too short (0 chars, minimum 50)",
		},
		{
			name:     "short response fails",
			response: "too short",
			wantPass: false,
			wantErr:  "response too short (9 chars, minimum 50)",
		},
		{
			name:     "exactly 49 chars fails",
			response: strings.Repeat("a", 49),
			wantPass: false,
		},
		{
			name:     "exactly 50 chars passes",
			response: strings.Repeat("a", 50),
			wantPass: true,
		},
		{
			name:     "long response passes",
			response: strings.Repeat("a", 500),
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := validateAnalytical(tt.response)
			if result.Pass != tt.wantPass {
				t.Errorf("Pass = %v, want %v", result.Pass, tt.wantPass)
			}
			if !tt.wantPass && tt.wantErr != "" {
				if len(result.Errors) == 0 {
					t.Fatal("expected errors, got none")
				}
				if result.Errors[0].Message != tt.wantErr {
					t.Errorf("error message = %q, want %q", result.Errors[0].Message, tt.wantErr)
				}
				if result.Errors[0].Phase != "format" {
					t.Errorf("error phase = %q, want %q", result.Errors[0].Phase, "format")
				}
			}
			if !tt.wantPass && !result.Retryable {
				t.Error("analytical failures should be retryable")
			}
		})
	}
}

func TestValidateWorktree_ValidCode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFiles(t, dir, "module testmod\ngo 1.22\n",
		"package main\nfunc main() {}\n",
		"package main\nimport \"testing\"\nfunc TestMain(m *testing.M) { m.Run() }\n",
	)

	result := validateWorktree(context.Background(), dir, zap.NewNop())
	if !result.Pass {
		t.Errorf("expected pass, got errors: %+v", result.Errors)
	}
}

func TestValidateWorktree_BuildFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFiles(t, dir, "module testmod\ngo 1.22\n",
		"package main\nfunc main() { invalid syntax here }\n",
		"",
	)

	result := validateWorktree(context.Background(), dir, zap.NewNop())
	if result.Pass {
		t.Fatal("expected failure, got pass")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected errors, got none")
	}
	if result.Errors[0].Phase != "build" {
		t.Errorf("phase = %q, want %q", result.Errors[0].Phase, "build")
	}
	if !result.Retryable {
		t.Error("build failures should be retryable")
	}
}

func TestValidateWorktree_TestFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFiles(t, dir, "module testmod\ngo 1.22\n",
		"package main\nfunc main() {}\n",
		"package main\nimport \"testing\"\nfunc TestFail(t *testing.T) { t.Fatal(\"intentional failure\") }\n",
	)

	result := validateWorktree(context.Background(), dir, zap.NewNop())
	if result.Pass {
		t.Fatal("expected failure, got pass")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected errors, got none")
	}
	if result.Errors[0].Phase != "test" {
		t.Errorf("phase = %q, want %q", result.Errors[0].Phase, "test")
	}
	if !result.Retryable {
		t.Error("test failures should be retryable")
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string truncated",
			input:  "hello world",
			maxLen: 5,
			want:   "hello\n... (truncated)",
		},
		{
			name:   "empty string unchanged",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestIsBadOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{
			name:  "empty file list is not bad output",
			files: nil,
			want:  false,
		},
		{
			name:  "only CLAUDE.md is bad output",
			files: []string{"CLAUDE.md"},
			want:  true,
		},
		{
			name:  "only .claude/ files is bad output",
			files: []string{".claude/settings.json", ".claude/CLAUDE.md"},
			want:  true,
		},
		{
			name:  "only .samverk/ files is bad output",
			files: []string{".samverk/status.md"},
			want:  true,
		},
		{
			name:  "mixed CLAUDE.md and real code is not bad output",
			files: []string{"CLAUDE.md", "internal/server/handler.go"},
			want:  false,
		},
		{
			name:  "real code files only is not bad output",
			files: []string{"internal/agent/runner.go", "cmd/samverk/main.go"},
			want:  false,
		},
		{
			name:  "config files mix is bad output",
			files: []string{"CLAUDE.md", ".claude/settings.json", ".github/workflows/ci.yml", ".gitignore"},
			want:  true,
		},
		{
			name:  "nested CLAUDE.md is bad output",
			files: []string{"subdir/CLAUDE.md"},
			want:  true,
		},
		{
			name:  ".editorconfig only is bad output",
			files: []string{".editorconfig"},
			want:  true,
		},
		{
			name:  "go file alongside config is not bad output",
			files: []string{"CLAUDE.md", "main.go"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsBadOutput(tt.files)
			if got != tt.want {
				t.Errorf("IsBadOutput(%v) = %v, want %v", tt.files, got, tt.want)
			}
		})
	}
}

func TestValidateWorkspaceOutput_NilForNoWorkDir(t *testing.T) {
	t.Parallel()
	result := ValidateWorkspaceOutput("", nil)
	if result != nil {
		t.Errorf("expected nil for empty workDir, got %+v", result)
	}
}

func TestValidateWorkspaceOutput_CLAUDEMDOnlyIsNonRetryable(t *testing.T) {
	t.Parallel()

	// Build a git repo where the latest commit only touches CLAUDE.md.
	// This simulates an Ollama model overwriting project config instead of
	// implementing the assigned feature -- the runtime guard that allows
	// qwen3-coder:30b in the "default" routing chain.
	dir := setupTestRepo(t)

	// Second commit: only modify CLAUDE.md (config-only output).
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("# overwritten by agent\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	mustGit(t, dir, "add", "CLAUDE.md")
	mustGit(t, dir, "commit", "-m", "bad: overwrite CLAUDE.md only")

	result := ValidateWorkspaceOutput(dir, zap.NewNop())

	if result == nil {
		t.Fatal("expected non-nil result for CLAUDE.md-only output, got nil")
	}
	if result.Pass {
		t.Error("Pass = true, want false")
	}
	if result.Retryable {
		t.Error("Retryable = true, want false (CLAUDE.md-only is a non-retryable failure)")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error, got none")
	}
	if result.Errors[0].Phase != "output-quality" {
		t.Errorf("error phase = %q, want %q", result.Errors[0].Phase, "output-quality")
	}
}

// writeGoFiles creates a minimal Go module in dir with go.mod, main.go, and
// optionally main_test.go (when testContent is non-empty).
func writeGoFiles(t *testing.T, dir, goMod, mainContent, testContent string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if mainContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0o644); err != nil {
			t.Fatalf("write main.go: %v", err)
		}
	}
	if testContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testContent), 0o644); err != nil {
			t.Fatalf("write main_test.go: %v", err)
		}
	}
}
