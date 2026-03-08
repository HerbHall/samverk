package agent

import (
	"strings"
	"testing"
)

func TestParseEditBlocks(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEdits int
		wantTitle string
		wantPaths []string
	}{
		{
			name: "single edit block",
			input: `EDIT internal/foo.go
package foo

func Hello() string { return "hello" }
END

PR_TITLE: feat: add hello function`,
			wantEdits: 1,
			wantTitle: "feat: add hello function",
			wantPaths: []string{"internal/foo.go"},
		},
		{
			name: "multiple edit blocks",
			input: `EDIT internal/foo.go
package foo
END

EDIT internal/foo_test.go
package foo

import "testing"
END

PR_TITLE: test: add foo tests`,
			wantEdits: 2,
			wantTitle: "test: add foo tests",
			wantPaths: []string{"internal/foo.go", "internal/foo_test.go"},
		},
		{
			name:      "no edit blocks - prose response",
			input:     "I analyzed the issue and here are my findings...",
			wantEdits: 0,
			wantTitle: "",
		},
		{
			name: "edit blocks without PR_TITLE",
			input: `EDIT cmd/main.go
package main
END`,
			wantEdits: 1,
			wantTitle: "",
			wantPaths: []string{"cmd/main.go"},
		},
		{
			name: "PR_TITLE without edit blocks",
			input: `Some prose response.
PR_TITLE: fix: handle edge case`,
			wantEdits: 0,
			wantTitle: "fix: handle edge case",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEditBlocks(tt.input)

			if len(got.Edits) != tt.wantEdits {
				t.Errorf("got %d edits, want %d", len(got.Edits), tt.wantEdits)
			}

			if got.PRTitle != tt.wantTitle {
				t.Errorf("PRTitle = %q, want %q", got.PRTitle, tt.wantTitle)
			}

			for i, wantPath := range tt.wantPaths {
				if i >= len(got.Edits) {
					break
				}
				if got.Edits[i].Path != wantPath {
					t.Errorf("edit[%d].Path = %q, want %q", i, got.Edits[i].Path, wantPath)
				}
			}
		})
	}
}

func TestParseEditBlocks_ContentPreservation(t *testing.T) {
	input := `EDIT internal/foo.go
package foo

// Hello returns a greeting.
func Hello() string {
	return "hello"
}
END`

	got := ParseEditBlocks(input)
	if len(got.Edits) != 1 {
		t.Fatalf("got %d edits, want 1", len(got.Edits))
	}

	content := got.Edits[0].Content
	if !strings.Contains(content, "package foo") {
		t.Error("content missing package declaration")
	}
	if !strings.Contains(content, "// Hello returns a greeting.") {
		t.Error("content missing comment")
	}
	if !strings.Contains(content, `return "hello"`) {
		t.Error("content missing return statement")
	}
}

func TestBranchSlug(t *testing.T) {
	tests := []struct {
		name   string
		number int
		title  string
		want   string
	}{
		{
			name:   "simple title",
			number: 42,
			title:  "fix widget rendering",
			want:   "agent/issue-42-fix-widget-rendering",
		},
		{
			name:   "long title truncated",
			number: 100,
			title:  "implement the entire authentication subsystem with OAuth support",
			want:   "agent/issue-100-implement-the-entire-authenticatio",
		},
		{
			name:   "special characters stripped",
			number: 5,
			title:  "fix: handle edge-case (null values)",
			want:   "agent/issue-5-fix-handle-edge-case-null-values",
		},
		{
			name:   "single word",
			number: 1,
			title:  "bug",
			want:   "agent/issue-1-bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BranchSlug(tt.number, tt.title)
			if got != tt.want {
				t.Errorf("BranchSlug(%d, %q) = %q, want %q", tt.number, tt.title, got, tt.want)
			}
			if len(got) > 50 {
				t.Errorf("branch name too long: %d chars", len(got))
			}
		})
	}
}
