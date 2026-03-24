package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExploreContext_CLAUDEMDIncluded(t *testing.T) {
	dir := t.TempDir()
	claudeContent := "# My Project\n\nBuild with `make build`.\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ExploreContext(dir, "Fix something.")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["CLAUDE.md"] != claudeContent {
		t.Errorf("CLAUDE.md content = %q, want %q", result["CLAUDE.md"], claudeContent)
	}
}

func TestExploreContext_CLAUDEMDAbsent(t *testing.T) {
	dir := t.TempDir()

	result := ExploreContext(dir, "Fix something.")
	if _, ok := result["CLAUDE.md"]; ok {
		t.Error("CLAUDE.md should not be present when file does not exist")
	}
}

func TestExploreContext_ReferencedFilesRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	runnerContent := "package agent\n\nfunc Run() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "agent", "runner.go"), []byte(runnerContent), 0o644); err != nil {
		t.Fatal(err)
	}

	body := "Please update internal/agent/runner.go to add explore support."
	result := ExploreContext(dir, body)

	if result["internal/agent/runner.go"] != runnerContent {
		t.Errorf("runner.go content = %q, want %q", result["internal/agent/runner.go"], runnerContent)
	}
}

func TestExploreContext_SiblingDiscovery(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "internal", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create several sibling .go files.
	files := map[string]string{
		"runner.go":  "package agent\n\nfunc Run() {}\n",
		"pool.go":    "package agent\n\nfunc Pool() {}\n",
		"prompts.go": "package agent\n\nfunc Prompt() {}\n",
		"util.txt":   "not a go file",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(agentDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Issue references only runner.go, but siblings should also be discovered.
	body := "Fix the bug in internal/agent/runner.go."
	result := ExploreContext(dir, body)

	// Referenced file must be present.
	if _, ok := result["internal/agent/runner.go"]; !ok {
		t.Error("runner.go should be in result")
	}

	// Siblings .go files should be discovered.
	if _, ok := result["internal/agent/pool.go"]; !ok {
		t.Error("pool.go sibling should be discovered")
	}
	if _, ok := result["internal/agent/prompts.go"]; !ok {
		t.Error("prompts.go sibling should be discovered")
	}

	// Non-Go files should NOT be discovered.
	if _, ok := result["internal/agent/util.txt"]; ok {
		t.Error("util.txt should not be discovered as a Go sibling")
	}
}

func TestExploreContext_TotalCapEnforced(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "internal", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create files that together exceed maxExploreBytes (64KB).
	// Each file is 8KB (maxSingleFileBytes), so 10 files = 80KB > 64KB.
	bigContent := strings.Repeat("x", maxSingleFileBytes)
	for i := 0; i < 10; i++ {
		name := filepath.Join(agentDir, "file"+string(rune('a'+i))+".go")
		if err := os.WriteFile(name, []byte(bigContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Reference one file so siblings are discovered.
	body := "Check internal/agent/filea.go"
	result := ExploreContext(dir, body)

	var totalSize int
	for _, content := range result {
		totalSize += len(content)
	}

	if totalSize > maxExploreBytes {
		t.Errorf("total explored content = %d bytes, exceeds cap of %d", totalSize, maxExploreBytes)
	}

	// Should have read some files but not all 10.
	if len(result) >= 10 {
		t.Errorf("expected fewer than 10 files due to cap, got %d", len(result))
	}
}

func TestExploreContext_SingleFileCapEnforced(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "internal", "big")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a single file larger than maxSingleFileBytes.
	hugeContent := strings.Repeat("y", maxSingleFileBytes*2)
	if err := os.WriteFile(filepath.Join(pkgDir, "huge.go"), []byte(hugeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	body := "Check internal/big/huge.go"
	result := ExploreContext(dir, body)

	content := result["internal/big/huge.go"]
	if len(content) > maxSingleFileBytes {
		t.Errorf("single file content = %d bytes, exceeds single file cap of %d", len(content), maxSingleFileBytes)
	}
	if content == "" {
		t.Error("huge.go should have truncated content, not empty")
	}
}

func TestExploreContext_EmptyRepoDir(t *testing.T) {
	result := ExploreContext("", "internal/agent/runner.go mentioned here")
	if result != nil {
		t.Errorf("expected nil for empty repoDir, got %v", result)
	}
}

func TestExploreContext_NoMatchesInBody(t *testing.T) {
	dir := t.TempDir()
	claudeContent := "# Project\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ExploreContext(dir, "Just a plain description with no file paths.")
	if result == nil {
		t.Fatal("expected non-nil result (CLAUDE.md should be included)")
	}
	if len(result) != 1 {
		t.Errorf("expected only CLAUDE.md, got %d files", len(result))
	}
	if _, ok := result["CLAUDE.md"]; !ok {
		t.Error("CLAUDE.md should be present even without issue body matches")
	}
}

func TestExploreFileList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(dir, "internal", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"runner.go", "pool.go", "prompts.go"} {
		if err := os.WriteFile(filepath.Join(agentDir, name), []byte("package agent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	body := "Fix internal/agent/runner.go"
	paths := ExploreFileList(dir, body)

	// Should include CLAUDE.md, the referenced file, and siblings.
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	if !pathSet["CLAUDE.md"] {
		t.Error("CLAUDE.md should be in file list")
	}
	if !pathSet["internal/agent/runner.go"] {
		t.Error("runner.go should be in file list")
	}
	if !pathSet["internal/agent/pool.go"] {
		t.Error("pool.go sibling should be in file list")
	}
}

func TestExploreFileList_EmptyRepoDir(t *testing.T) {
	result := ExploreFileList("", "internal/agent/runner.go")
	if result != nil {
		t.Errorf("expected nil for empty repoDir, got %v", result)
	}
}

func TestDiscoverGoSiblings(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "internal", "server")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"server.go", "handler.go", "config.yaml", "README.md"} {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{"internal/server/server.go": true}
	siblings := discoverGoSiblings(dir, "internal/server", seen)

	if len(siblings) != 1 {
		t.Fatalf("expected 1 sibling, got %d: %v", len(siblings), siblings)
	}
	if siblings[0] != "internal/server/handler.go" {
		t.Errorf("expected handler.go, got %q", siblings[0])
	}
}

func TestReadFileCapped(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("a", 1000)
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Read with cap larger than file.
	got := readFileCapped(path, 2000)
	if got != content {
		t.Errorf("readFileCapped full read: got %d bytes, want %d", len(got), len(content))
	}

	// Read with cap smaller than file.
	got = readFileCapped(path, 500)
	if len(got) != 500 {
		t.Errorf("readFileCapped capped read: got %d bytes, want 500", len(got))
	}

	// Read nonexistent file.
	got = readFileCapped(filepath.Join(dir, "nope.txt"), 500)
	if got != "" {
		t.Errorf("readFileCapped nonexistent: got %q, want empty", got)
	}
}

func TestExploreContext_YAMLFrontmatterFileContext(t *testing.T) {
	dir := t.TempDir()

	// Create files in various project directories.
	dirs := map[string]string{
		"web/src/pages":        "Agents.tsx",
		"scripts/pc-agent":     "registration.psm1",
		"internal/api":         "api.go",
		"overlay":              "labels.json",
		".samverk":             "project.yaml",
		".github/workflows":    "ci.yml",
	}
	for d, f := range dirs {
		p := filepath.Join(dir, filepath.FromSlash(d))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, f), []byte("content of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Issue body with YAML frontmatter file_context list.
	body := `---
schema_version: "1.1.0"
type: task
agent_type: code-gen
file_context:
  - web/src/pages/Agents.tsx
  - scripts/pc-agent/registration.psm1
  - internal/api/api.go
  - overlay/labels.json
  - .samverk/project.yaml
  - .github/workflows/ci.yml
---

## Summary

Update the agents page.
`

	result := ExploreContext(dir, body)

	expected := []string{
		"web/src/pages/Agents.tsx",
		"scripts/pc-agent/registration.psm1",
		"internal/api/api.go",
		"overlay/labels.json",
		".samverk/project.yaml",
		".github/workflows/ci.yml",
	}
	for _, path := range expected {
		if _, ok := result[path]; !ok {
			t.Errorf("expected %q in explore result, not found", path)
		}
	}
}

func TestFilePathRe_YAMLListMarker(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "YAML list with dash prefix",
			input: "  - internal/api/api.go\n  - web/src/pages/Agents.tsx",
			want:  []string{"internal/api/api.go", "web/src/pages/Agents.tsx"},
		},
		{
			name:  "prose reference",
			input: "Update internal/dispatcher/router.go to fix the bug.",
			want:  []string{"internal/dispatcher/router.go"},
		},
		{
			name:  "scripts prefix",
			input: "  - scripts/pc-agent/registration.psm1",
			want:  []string{"scripts/pc-agent/registration.psm1"},
		},
		{
			name:  "overlay prefix",
			input: "  - overlay/labels.json",
			want:  []string{"overlay/labels.json"},
		},
		{
			name:  "dotfile prefixes",
			input: "  - .samverk/project.yaml\n  - .github/workflows/ci.yml\n  - .gitea/workflows/security.yml",
			want:  []string{".samverk/project.yaml", ".github/workflows/ci.yml", ".gitea/workflows/security.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := filePathRe.FindAllStringSubmatch(tt.input, -1)
			var got []string
			for _, m := range matches {
				got = append(got, strings.TrimSpace(m[1]))
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d matches %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("match[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
