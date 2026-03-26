package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/synapset"
	"github.com/herbhall/samverk/pkg/models"
)

func TestGenerateAgentCLAUDEMD_Go(t *testing.T) {
	got := GenerateAgentCLAUDEMD(ProjectTypeGo, "Fix the widget parser")

	if !strings.Contains(got, "go build ./...") {
		t.Error("Go CLAUDE.md missing go build command")
	}
	if !strings.Contains(got, "go test ./...") {
		t.Error("Go CLAUDE.md missing go test command")
	}
	if !strings.Contains(got, "GOOS=linux") {
		t.Error("Go CLAUDE.md missing cross-compile check")
	}
	if !strings.Contains(got, "gosec G101") {
		t.Error("Go CLAUDE.md missing gosec G101 warning")
	}
}

func TestGenerateAgentCLAUDEMD_Frontend(t *testing.T) {
	got := GenerateAgentCLAUDEMD(ProjectTypeFrontend, "Update dashboard")

	if !strings.Contains(got, "npx tsc --noEmit") {
		t.Error("Frontend CLAUDE.md missing tsc command")
	}
	if !strings.Contains(got, "npx eslint") {
		t.Error("Frontend CLAUDE.md missing eslint command")
	}
	if !strings.Contains(got, "pnpm install") {
		t.Error("Frontend CLAUDE.md missing pnpm install")
	}
	// Should NOT contain Go-specific commands.
	if strings.Contains(got, "go build") {
		t.Error("Frontend CLAUDE.md should not contain go build")
	}
}

func TestGenerateAgentCLAUDEMD_Fullstack(t *testing.T) {
	got := GenerateAgentCLAUDEMD(ProjectTypeFullstack, "Add API endpoint and UI")

	if !strings.Contains(got, "Backend:") {
		t.Error("Fullstack CLAUDE.md missing Backend section")
	}
	if !strings.Contains(got, "Frontend:") {
		t.Error("Fullstack CLAUDE.md missing Frontend section")
	}
	if !strings.Contains(got, "go build") {
		t.Error("Fullstack CLAUDE.md missing go build")
	}
	if !strings.Contains(got, "npx tsc") {
		t.Error("Fullstack CLAUDE.md missing tsc")
	}
}

func TestGenerateAgentCLAUDEMD_CorePrinciples(t *testing.T) {
	types := []string{ProjectTypeGo, ProjectTypeFrontend, ProjectTypeFullstack, "unknown"}
	for _, pt := range types {
		got := GenerateAgentCLAUDEMD(pt, "some task")
		if !strings.Contains(got, "Core Principles -- UNCONDITIONAL") {
			t.Errorf("CLAUDE.md for project type %q missing core principles", pt)
		}
		if !strings.Contains(got, "Once found, always fix") {
			t.Errorf("CLAUDE.md for project type %q missing principle #1", pt)
		}
	}
}

func TestGenerateAgentCLAUDEMD_TaskContext(t *testing.T) {
	body := "Implement feature X with tests"
	got := GenerateAgentCLAUDEMD(ProjectTypeGo, body)

	if !strings.Contains(got, "## Task Context") {
		t.Error("CLAUDE.md missing Task Context section")
	}
	if !strings.Contains(got, body) {
		t.Error("CLAUDE.md missing issue body in task context")
	}
}

func TestGenerateAgentCLAUDEMD_EmptyBody(t *testing.T) {
	got := GenerateAgentCLAUDEMD(ProjectTypeGo, "")

	if strings.Contains(got, "## Task Context") {
		t.Error("CLAUDE.md should not have Task Context when body is empty")
	}
}

func TestGenerateAgentCLAUDEMD_GitSafety(t *testing.T) {
	types := []string{ProjectTypeGo, ProjectTypeFrontend, ProjectTypeFullstack}
	for _, pt := range types {
		got := GenerateAgentCLAUDEMD(pt, "some task")
		if !strings.Contains(got, "## Git Workflow") {
			t.Errorf("CLAUDE.md for project type %q missing git safety rules", pt)
		}
		if !strings.Contains(got, "Never force-push") {
			t.Errorf("CLAUDE.md for project type %q missing force-push warning", pt)
		}
	}
}

func TestGenerateAgentCLAUDEMD_KnownGotchas(t *testing.T) {
	got := GenerateAgentCLAUDEMD(ProjectTypeGo, "some task")
	if !strings.Contains(got, "## Known Gotchas") {
		t.Error("CLAUDE.md missing Known Gotchas section")
	}
	if !strings.Contains(got, "rangeValCopy") {
		t.Error("CLAUDE.md missing rangeValCopy gotcha")
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{name: "no labels", labels: nil, want: ProjectTypeGo},
		{name: "empty labels", labels: []string{}, want: ProjectTypeGo},
		{name: "backend label", labels: []string{"area:agent", models.LabelPriorityHigh}, want: ProjectTypeGo},
		{name: "frontend epic", labels: []string{"epic:frontend", models.LabelPriorityHigh}, want: ProjectTypeFrontend},
		{name: "web label", labels: []string{"area:web"}, want: ProjectTypeFrontend},
		{name: "ui label", labels: []string{"area:ui"}, want: ProjectTypeFrontend},
		{name: "explicit fullstack", labels: []string{"fullstack"}, want: ProjectTypeFullstack},
		{name: "full-stack hyphenated", labels: []string{"full-stack"}, want: ProjectTypeFullstack},
		{name: "frontend+backend", labels: []string{"epic:frontend", "area:api"}, want: ProjectTypeFullstack},
		{name: "frontend+server", labels: []string{"area:web", "area:server"}, want: ProjectTypeFullstack},
		{name: "case insensitive", labels: []string{"EPIC:FRONTEND"}, want: ProjectTypeFrontend},
		{name: "backend only", labels: []string{"area:api", models.LabelPriorityHigh}, want: ProjectTypeGo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectProjectType(tt.labels)
			if got != tt.want {
				t.Errorf("DetectProjectType(%v) = %q, want %q", tt.labels, got, tt.want)
			}
		})
	}
}

func TestWriteMCPConfig(t *testing.T) {
	dir := t.TempDir()

	if err := WriteMCPConfig(dir); err != nil {
		t.Fatalf("WriteMCPConfig: %v", err)
	}

	path := filepath.Join(dir, ".claude", ".mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mcp config: %v", err)
	}

	var config map[string]interface{}
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal mcp config: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("mcpServers key missing or wrong type")
	}

	samverk, ok := servers["samverk"].(map[string]interface{})
	if !ok {
		t.Fatal("samverk server missing")
	}
	if samverk["url"] != "https://samverk.herbhall.net/mcp" {
		t.Errorf("samverk url: got %v", samverk["url"])
	}

	syn, ok := servers["synapset"].(map[string]interface{})
	if !ok {
		t.Fatal("synapset server missing")
	}
	if syn["url"] != "https://synapset.herbhall.net/mcp" {
		t.Errorf("synapset url: got %v", syn["url"])
	}
}

func TestFetchRelevantPatterns_NilClient(t *testing.T) {
	patterns, err := FetchRelevantPatterns(context.Background(), nil, "test query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected empty slice, got %d patterns", len(patterns))
	}
}

func TestFetchRelevantPatterns_EmptyQuery(t *testing.T) {
	// Even with a non-nil client, empty query should return nil.
	client := synapset.New(synapset.Config{URL: "http://localhost:0", Pool: "test"}, nil)
	patterns, err := FetchRelevantPatterns(context.Background(), client, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patterns != nil {
		t.Errorf("expected nil for empty query, got %v", patterns)
	}
}

func TestBuildSystemPrompt_WithPatterns(t *testing.T) {
	task := Task{
		Issue: &forge.Issue{
			Number: 99,
			Title:  "add patterns",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess_99_1",
	}

	patterns := []string{
		"Always run golangci-lint before committing",
		"Use table-driven tests in Go",
	}

	got := BuildSystemPrompt(task, nil, patterns...)

	if !strings.Contains(got, "## Known Patterns") {
		t.Error("prompt missing Known Patterns section")
	}
	for _, p := range patterns {
		if !strings.Contains(got, p) {
			t.Errorf("prompt missing pattern: %q", p)
		}
	}
}

func TestBuildSystemPrompt_WithoutPatterns(t *testing.T) {
	task := Task{
		Issue: &forge.Issue{
			Number: 99,
			Title:  "no patterns",
		},
		AgentType: models.AgentTypeCodeGen,
		SessionID: "sess_99_1",
	}

	got := BuildSystemPrompt(task, nil)

	if strings.Contains(got, "## Known Patterns") {
		t.Error("prompt should not have Known Patterns when no patterns provided")
	}
}
