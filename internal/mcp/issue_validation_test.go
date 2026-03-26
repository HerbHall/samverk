package mcp

import (
	"strings"
	"testing"

	"github.com/herbhall/samverk/pkg/models"
)

func TestValidateAndEnrichIssue_TitlePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{name: "feat", title: "feat: add thing", wantErr: false},
		{name: "fix", title: "fix: fix thing", wantErr: false},
		{name: "chore", title: "chore: update deps", wantErr: false},
		{name: "docs", title: "docs: update readme", wantErr: false},
		{name: "test", title: "test: add coverage", wantErr: false},
		{name: "ci", title: "ci: fix workflow", wantErr: false},
		{name: "style", title: "style: format code", wantErr: false},
		{name: "refactor", title: "refactor: extract helper", wantErr: false},
		{name: "feat with scope", title: "feat(auth): add oauth", wantErr: false},
		{name: "fix with breaking", title: "fix!: remove endpoint", wantErr: false},
		{name: "feat with scope and breaking", title: "feat(api)!: redesign", wantErr: false},
		{name: "no prefix", title: "add new feature", wantErr: true},
		{name: "wrong prefix", title: "update: something", wantErr: true},
		{name: "missing colon space", title: "feat:no space", wantErr: true},
		{name: "empty", title: "", wantErr: true},
		{name: "uppercase", title: "Feat: something", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := validateAndEnrichIssue(tc.title, "body", nil, nil)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for title %q, got nil", tc.title)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for title %q: %v", tc.title, err)
			}
		})
	}
}

func TestValidateAndEnrichIssue_AgentLabelInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		title          string
		labels         []string
		wantAgentLabel string
	}{
		{
			name:           "feat injects code-gen",
			title:          "feat: add thing",
			labels:         nil,
			wantAgentLabel: models.LabelAgentCodeGen,
		},
		{
			name:           "docs injects docs",
			title:          "docs: update readme",
			labels:         nil,
			wantAgentLabel: models.LabelAgentDocs,
		},
		{
			name:           "fix injects code-gen",
			title:          "fix: fix thing",
			labels:         nil,
			wantAgentLabel: models.LabelAgentCodeGen,
		},
		{
			name:           "existing agent label preserved",
			title:          "feat: add thing",
			labels:         []string{models.LabelAgentHuman},
			wantAgentLabel: models.LabelAgentHuman,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _, err := validateAndEnrichIssue(tc.title, "body", tc.labels, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var found string
			for _, l := range got {
				if strings.HasPrefix(l, "agent:") {
					if found != "" {
						t.Errorf("multiple agent labels found: %v", got)
					}
					found = l
				}
			}

			if found != tc.wantAgentLabel {
				t.Errorf("agent label: got %q, want %q", found, tc.wantAgentLabel)
			}
		})
	}
}

func TestValidateAndEnrichIssue_PriorityDefault(t *testing.T) {
	t.Parallel()

	t.Run("injects priority:normal when absent", func(t *testing.T) {
		t.Parallel()
		got, _, err := validateAndEnrichIssue("feat: add thing", "body", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hasPriority := false
		for _, l := range got {
			if strings.HasPrefix(l, "priority:") {
				hasPriority = true
				if l != models.LabelPriorityNormal {
					t.Errorf("expected priority:normal, got %q", l)
				}
			}
		}
		if !hasPriority {
			t.Error("expected a priority:* label to be injected")
		}
	})

	t.Run("preserves existing priority label", func(t *testing.T) {
		t.Parallel()
		got, _, err := validateAndEnrichIssue("feat: add thing", "body", []string{models.LabelPriorityHigh}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var priorityLabels []string
		for _, l := range got {
			if strings.HasPrefix(l, "priority:") {
				priorityLabels = append(priorityLabels, l)
			}
		}
		if len(priorityLabels) != 1 {
			t.Errorf("expected exactly 1 priority label, got %v", priorityLabels)
		}
		if priorityLabels[0] != models.LabelPriorityHigh {
			t.Errorf("expected priority:high to be preserved, got %q", priorityLabels[0])
		}
	})
}

func TestValidateAndEnrichIssue_Frontmatter(t *testing.T) {
	t.Parallel()

	t.Run("no frontmatter leaves body unchanged", func(t *testing.T) {
		t.Parallel()
		_, body, err := validateAndEnrichIssue("feat: add thing", "original body", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if body != "original body" {
			t.Errorf("expected unchanged body, got %q", body)
		}
	})

	t.Run("frontmatter prepended as YAML block", func(t *testing.T) {
		t.Parallel()
		fm := map[string]string{"sprint": "v1.2"}
		_, body, err := validateAndEnrichIssue("feat: add thing", "the body", nil, fm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(body, "---\n") {
			t.Errorf("expected body to start with YAML frontmatter delimiter, got: %q", body[:minLen(len(body), 30)])
		}
		if !strings.Contains(body, "sprint: v1.2") {
			t.Errorf("expected frontmatter key to appear in body, got: %s", body)
		}
		if !strings.HasSuffix(body, "\n\nthe body") {
			t.Errorf("expected body to end with original text after blank line, got: %q", body)
		}
	})

	t.Run("empty frontmatter map leaves body unchanged", func(t *testing.T) {
		t.Parallel()
		_, body, err := validateAndEnrichIssue("feat: add thing", "original body", nil, map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if body != "original body" {
			t.Errorf("expected unchanged body for empty frontmatter map, got %q", body)
		}
	})
}

func TestValidateAndEnrichIssue_InputNotMutated(t *testing.T) {
	t.Parallel()

	orig := []string{"bug"}
	got, _, err := validateAndEnrichIssue("feat: add thing", "body", orig, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original slice must not be modified.
	if len(orig) != 1 || orig[0] != "bug" {
		t.Errorf("original labels slice was mutated: %v", orig)
	}

	// Returned slice must have the injections.
	if len(got) <= len(orig) {
		t.Errorf("expected enriched labels to have more entries than original, got %v", got)
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
