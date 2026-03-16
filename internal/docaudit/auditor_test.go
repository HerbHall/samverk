package docaudit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckStaleStatus(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantCount  int
		wantSev    Severity
		wantSubstr string
	}{
		{
			name: "fresh status",
			content: "---\nupdated: " + time.Now().UTC().Format(time.RFC3339) + "\n---\n# Title\n",
			wantCount: 0,
		},
		{
			name: "stale status",
			content: "---\nupdated: 2020-01-01T00:00:00Z\n---\n# Title\n",
			wantCount:  1,
			wantSev:    SeverityWarning,
			wantSubstr: "days old",
		},
		{
			name:       "unparseable timestamp",
			content:    "---\nupdated: not-a-date\n---\n# Title\n",
			wantCount:  1,
			wantSev:    SeverityWarning,
			wantSubstr: "could not parse",
		},
		{
			name:      "no frontmatter",
			content:   "# Just a heading\nSome content.\n",
			wantCount: 0,
		},
		{
			name:      "no updated field",
			content:   "---\nphase: test\n---\n# Title\n",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			samverkDir := filepath.Join(root, ".samverk")
			if err := os.MkdirAll(samverkDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(samverkDir, "status.md"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			a := New(root)
			findings, err := a.checkStaleStatus()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}

			if tt.wantCount > 0 {
				f := findings[0]
				if f.Severity != tt.wantSev {
					t.Errorf("severity = %q, want %q", f.Severity, tt.wantSev)
				}
				if tt.wantSubstr != "" && !contains(f.Message, tt.wantSubstr) {
					t.Errorf("message %q does not contain %q", f.Message, tt.wantSubstr)
				}
			}
		})
	}
}

func TestCheckMissingReferences(t *testing.T) {
	tests := []struct {
		name      string
		docBody   string
		targets   map[string]string // relative path from docs dir -> content
		wantCount int
	}{
		{
			name:      "valid relative link",
			docBody:   "[arch](architecture.md)\n",
			targets:   map[string]string{"architecture.md": "# Arch"},
			wantCount: 0,
		},
		{
			name:      "broken relative link",
			docBody:   "[missing](nonexistent.md)\n",
			targets:   nil,
			wantCount: 1,
		},
		{
			name:      "external link ignored",
			docBody:   "[Google](https://google.com)\n",
			targets:   nil,
			wantCount: 0,
		},
		{
			name:      "anchor-only link ignored",
			docBody:   "[section](#heading)\n",
			targets:   nil,
			wantCount: 0,
		},
		{
			name:      "relative link with anchor to existing file",
			docBody:   "[arch](architecture.md#section)\n",
			targets:   map[string]string{"architecture.md": "# Arch"},
			wantCount: 0,
		},
		{
			name:      "subdirectory link",
			docBody:   "[adr](decisions/ADR-001.md)\n",
			targets:   map[string]string{"decisions/ADR-001.md": "# ADR"},
			wantCount: 0,
		},
		{
			name:      "broken subdirectory link",
			docBody:   "[adr](decisions/ADR-999.md)\n",
			targets:   nil,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			docsDir := filepath.Join(root, "docs")
			if err := os.MkdirAll(docsDir, 0o755); err != nil {
				t.Fatal(err)
			}

			// Write the test doc file.
			if err := os.WriteFile(filepath.Join(docsDir, "test.md"), []byte(tt.docBody), 0o644); err != nil {
				t.Fatal(err)
			}

			// Write target files.
			for rel, content := range tt.targets {
				targetPath := filepath.Join(docsDir, rel)
				if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			a := New(root)
			findings, err := a.checkMissingReferences()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}
		})
	}
}

func TestCheckRequiredDocs(t *testing.T) {
	tests := []struct {
		name      string
		files     []string // paths to create relative to root
		wantCount int
	}{
		{
			name: "all present",
			files: []string{
				"docs/architecture.md",
				"docs/tech-stack.md",
				".samverk/status.md",
			},
			wantCount: 0,
		},
		{
			name:      "all missing",
			files:     nil,
			wantCount: 3,
		},
		{
			name: "one missing",
			files: []string{
				"docs/architecture.md",
				".samverk/status.md",
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()

			for _, rel := range tt.files {
				absPath := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absPath, []byte("# placeholder"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			a := New(root)
			findings := a.checkRequiredDocs()

			if len(findings) != tt.wantCount {
				t.Fatalf("got %d findings, want %d: %+v", len(findings), tt.wantCount, findings)
			}

			for _, f := range findings {
				if f.Severity != SeverityError {
					t.Errorf("expected error severity, got %q", f.Severity)
				}
			}
		})
	}
}

func TestRunIntegration(t *testing.T) {
	root := t.TempDir()

	// Create required docs.
	for _, rel := range []string{"docs/architecture.md", "docs/tech-stack.md"} {
		absPath := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte("# placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a doc with a broken link.
	if err := os.WriteFile(
		filepath.Join(root, "docs", "architecture.md"),
		[]byte("[broken](nonexistent.md)\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create stale status.md.
	samverkDir := filepath.Join(root, ".samverk")
	if err := os.MkdirAll(samverkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(samverkDir, "status.md"),
		[]byte("---\nupdated: 2020-01-01T00:00:00Z\n---\n# Status\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	a := New(root)
	findings, err := a.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: 1 stale + 1 broken link = 2 findings.
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
