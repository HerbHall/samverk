package agent

import (
	"strings"
	"testing"

	"samverk.dev/samverk/internal/forge"
	"samverk.dev/samverk/pkg/models"
)

func TestBuildSystemPrompt(t *testing.T) {
	tests := []struct {
		name      string
		agentType models.AgentType
		wantEmpty bool
		contains  []string
	}{
		{
			name:      "code-gen includes edit format",
			agentType: models.AgentTypeCodeGen,
			contains:  []string{"code generation agent", "EDIT <filepath>", "PR_TITLE:", "#42", "fix widget", "Do NOT modify CLAUDE.md"},
		},
		{
			name:      "test includes table-driven",
			agentType: models.AgentTypeTest,
			contains:  []string{"test agent", "Table-driven tests", "EDIT", "END", "PR_TITLE:", "#42"},
		},
		{
			name:      "docs includes markdown rules",
			agentType: models.AgentTypeDocs,
			contains:  []string{"documentation agent", "heading hierarchy", "EDIT", "END", "PR_TITLE:", "#42"},
		},
		{
			name:      "research includes structured sections",
			agentType: models.AgentTypeResearch,
			contains:  []string{"research agent", "## Summary", "## Findings", "## Recommendation", "## Sources", "#42"},
		},
		{
			name:      "qc includes pass/fail",
			agentType: models.AgentTypeQC,
			contains:  []string{"quality control agent", "PASS or FAIL", "Correctness", "#42"},
		},
		{
			name:      "human returns empty",
			agentType: models.AgentTypeHuman,
			wantEmpty: true,
		},
		{
			name:      "orchestrator returns empty",
			agentType: models.AgentTypeOrchestrator,
			wantEmpty: true,
		},
		{
			name:      "dispatcher returns empty",
			agentType: models.AgentTypeDispatcher,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := Task{
				Issue: &forge.Issue{
					Number: 42,
					Title:  "fix widget",
				},
				AgentType: tt.agentType,
				SessionID: "sess_42_1",
			}

			got := BuildSystemPrompt(task, nil)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty prompt for %s, got %q", tt.agentType, got)
				}
				return
			}

			if got == "" {
				t.Fatalf("expected non-empty prompt for %s", tt.agentType)
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("prompt for %s missing %q", tt.agentType, want)
				}
			}
		})
	}
}

func TestBuildSystemPrompt_WithFileContext(t *testing.T) {
	task := Task{
		Issue: &forge.Issue{
			Number: 10,
			Title:  "add feature",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess_10_1",
	}

	files := map[string]string{
		"internal/agent/runner.go": "package agent\n\nfunc Run() {}\n",
	}

	got := BuildSystemPrompt(task, files)

	if !strings.Contains(got, "## Relevant Files") {
		t.Error("prompt missing Relevant Files section")
	}
	if !strings.Contains(got, "### internal/agent/runner.go") {
		t.Error("prompt missing file path header")
	}
	if !strings.Contains(got, "package agent") {
		t.Error("prompt missing file contents")
	}
}

func TestBuildSystemPrompt_FileContextTruncation(t *testing.T) {
	task := Task{
		Issue: &forge.Issue{
			Number: 10,
			Title:  "add feature",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess_10_1",
	}

	// Create a file larger than maxFileContextBytes.
	large := strings.Repeat("x", maxFileContextBytes+1000)
	files := map[string]string{
		"internal/big.go": large,
	}

	got := BuildSystemPrompt(task, files)

	if !strings.Contains(got, "... (truncated)") {
		t.Error("large file context should be truncated")
	}
	if len(got) > maxFileContextBytes+5000 {
		t.Errorf("prompt too large: %d bytes", len(got))
	}
}

func TestExtractFileContext(t *testing.T) {
	r := &Runner{}

	body := `Fix the bug in internal/agent/runner.go and update
cmd/samverk/main.go. Also check pkg/models/issue.go and docs/architecture.md.
Ignore random/path.go and node_modules/foo.js.`

	got := r.extractFileContext(body, "")

	want := []string{
		"internal/agent/runner.go",
		"cmd/samverk/main.go",
		"pkg/models/issue.go",
		"docs/architecture.md",
	}

	for _, path := range want {
		if _, ok := got[path]; !ok {
			t.Errorf("extractFileContext missing path %q", path)
		}
	}

	unwanted := []string{"random/path.go", "node_modules/foo.js"}
	for _, path := range unwanted {
		if _, ok := got[path]; ok {
			t.Errorf("extractFileContext should not include %q", path)
		}
	}
}
