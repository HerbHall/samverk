package dispatcher

import (
	"strings"
	"testing"

	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

func TestMdFilesFromFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		fm       *models.IssueFrontmatter
		expected int
	}{
		{
			name:     "nil frontmatter",
			fm:       nil,
			expected: 0,
		},
		{
			name: "no md files",
			fm: &models.IssueFrontmatter{
				FileContext: []string{"main.go", "handler.go"},
			},
			expected: 0,
		},
		{
			name: "mixed files",
			fm: &models.IssueFrontmatter{
				FileContext: []string{"README.md", "main.go", "docs/design.md"},
			},
			expected: 2,
		},
		{
			name: "case insensitive extension",
			fm: &models.IssueFrontmatter{
				FileContext: []string{"CHANGELOG.MD", "notes.Md"},
			},
			expected: 2,
		},
		{
			name: "empty file context",
			fm: &models.IssueFrontmatter{
				FileContext: []string{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mdFilesFromFrontmatter(tt.fm)
			if len(got) != tt.expected {
				t.Errorf("mdFilesFromFrontmatter() returned %d files, want %d", len(got), tt.expected)
			}
		})
	}
}

func TestDocLabelFor(t *testing.T) {
	tests := []struct {
		category string
		expected string
	}{
		{"stale", models.LabelDocStale},
		{"inconsistent", models.LabelDocInconsistent},
		{"structure", models.LabelDocStructure},
		{"decision-review", models.LabelDocDecisionReview},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			got := docLabelFor(tt.category)
			if got != tt.expected {
				t.Errorf("docLabelFor(%q) = %q, want %q", tt.category, got, tt.expected)
			}
		})
	}
}

func TestContradictionSeverity(t *testing.T) {
	tests := []struct {
		similarity float64
		expected   string
	}{
		{0.96, "critical"},
		{0.95, "high"},
		{0.92, "high"},
		{0.90, "medium"},
		{0.87, "medium"},
		{0.85, "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := contradictionSeverity(tt.similarity)
			if got != tt.expected {
				t.Errorf("contradictionSeverity(%.2f) = %q, want %q", tt.similarity, got, tt.expected)
			}
		})
	}
}

func TestRenderDocSection(t *testing.T) {
	t.Run("nil findings", func(t *testing.T) {
		var df *DocFindings
		if got := df.RenderDocSection(); got != "" {
			t.Errorf("nil DocFindings.RenderDocSection() = %q, want empty", got)
		}
	})

	t.Run("empty findings", func(t *testing.T) {
		df := &DocFindings{Files: []string{"README.md"}}
		if got := df.RenderDocSection(); got != "" {
			t.Errorf("empty DocFindings.RenderDocSection() = %q, want empty", got)
		}
	})

	t.Run("with findings", func(t *testing.T) {
		df := &DocFindings{
			Files: []string{"README.md"},
			Findings: []DocFinding{
				{Severity: "medium", Category: "stale", File: "README.md", Detail: "last indexed 45 days ago", Tier: 2},
				{Severity: "high", Category: "inconsistent", File: "README.md", Detail: "conflicts with CLAUDE.md", Tier: 3},
			},
		}
		got := df.RenderDocSection()
		if got == "" {
			t.Fatal("expected non-empty doc section")
		}
		if !strings.Contains(got, "## Documentation Review Findings") {
			t.Error("missing section header")
		}
		if !strings.Contains(got, "[MEDIUM]") {
			t.Error("missing MEDIUM severity marker")
		}
		if !strings.Contains(got, "[HIGH]") {
			t.Error("missing HIGH severity marker")
		}
		if !strings.Contains(got, "stale") {
			t.Error("missing stale category")
		}
	})

	t.Run("truncation", func(t *testing.T) {
		df := &DocFindings{
			Files: []string{"README.md"},
		}
		for i := 0; i < 50; i++ {
			df.Findings = append(df.Findings, DocFinding{
				Severity: "medium",
				Category: "structure",
				File:     "README.md",
				Detail:   "this is a long detail string that contributes to exceeding the max section length limit",
				Tier:     2,
			})
		}
		got := df.RenderDocSection()
		if len(got) > maxDocSection+50 {
			t.Errorf("doc section too long: %d bytes (max %d)", len(got), maxDocSection)
		}
		if !strings.Contains(got, "truncated") {
			t.Error("expected truncation marker")
		}
	})
}

func TestCheckDocFindings_NilSynapset(t *testing.T) {
	d := &Dispatcher{logger: zap.NewNop()} // no synapset client
	fm := &models.IssueFrontmatter{
		FileContext: []string{"README.md"},
	}
	got := d.checkDocFindings(nil, "owner", "repo", fm) //nolint:staticcheck // nil context is intentional for this test
	if got != nil {
		t.Errorf("expected nil when synapset is nil, got %+v", got)
	}
}

func TestCheckDocFindings_NoMdFiles(t *testing.T) {
	d := &Dispatcher{logger: zap.NewNop()}
	fm := &models.IssueFrontmatter{
		FileContext: []string{"main.go", "handler.go"},
	}
	got := d.checkDocFindings(nil, "owner", "repo", fm) //nolint:staticcheck // nil context is intentional for this test
	if got != nil {
		t.Errorf("expected nil when no .md files, got %+v", got)
	}
}
