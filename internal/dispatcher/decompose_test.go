package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
	"go.uber.org/zap"
)

// mockDecomposer is a test double for the Decomposer interface.
type mockDecomposer struct {
	result []SubTask
	err    error
	called bool
}

func (m *mockDecomposer) Decompose(_ context.Context, _ *forge.Issue, _, _ time.Duration) ([]SubTask, error) {
	m.called = true
	return m.result, m.err
}

func TestShouldDecompose(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		threshold time.Duration
		fm        *models.IssueFrontmatter
		labels    []string
		want      bool
	}{
		{
			name:      "disabled when threshold is zero",
			timeout:   90 * time.Minute,
			threshold: 0,
			want:      false,
		},
		{
			name:      "below threshold",
			timeout:   30 * time.Minute,
			threshold: 60 * time.Minute,
			want:      false,
		},
		{
			name:      "above threshold triggers decomposition",
			timeout:   90 * time.Minute,
			threshold: 60 * time.Minute,
			want:      true,
		},
		{
			name:      "child issue skipped",
			timeout:   90 * time.Minute,
			threshold: 60 * time.Minute,
			fm:        &models.IssueFrontmatter{ParentIssue: 42},
			want:      false,
		},
		{
			name:      "no-decompose label skipped",
			timeout:   90 * time.Minute,
			threshold: 60 * time.Minute,
			labels:    []string{"no-decompose"},
			want:      false,
		},
		{
			name:      "nil frontmatter allowed",
			timeout:   90 * time.Minute,
			threshold: 60 * time.Minute,
			fm:        nil,
			want:      true,
		},
		{
			name:      "exact threshold not triggered",
			timeout:   60 * time.Minute,
			threshold: 60 * time.Minute,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DecompositionThreshold: tt.threshold}
			issue := &forge.Issue{Number: 1, Title: "test", Labels: tt.labels}
			got := shouldDecompose(tt.timeout, cfg, tt.fm, issue)
			if got != tt.want {
				t.Errorf("shouldDecompose() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecomposeAndCreateChildren_DisabledWhenNoDecomposer(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{Number: 1, Title: "test", Body: "body"}
	decomposed, err := d.decomposeAndCreateChildren(
		context.Background(), "test", "repo", issue, nil, models.AgentTypeCodeGen, 90*time.Minute,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decomposed {
		t.Error("expected no decomposition without decomposer")
	}
}

func TestDecomposeAndCreateChildren_SkippedBelowThreshold(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)
	d.config.DecompositionThreshold = 60 * time.Minute

	dec := &mockDecomposer{}
	d.decomposer = dec

	issue := &forge.Issue{Number: 1, Title: "test", Body: "body"}
	tracker.issues[1] = issue

	decomposed, err := d.decomposeAndCreateChildren(
		context.Background(), "test", "repo", issue, nil, models.AgentTypeCodeGen, 30*time.Minute,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decomposed {
		t.Error("expected no decomposition below threshold")
	}
	if dec.called {
		t.Error("decomposer should not have been called")
	}
}

func TestDecomposeAndCreateChildren_CreatesChildren(t *testing.T) {
	tracker := newMockTracker()
	cfg := DefaultConfig()
	cfg.DecompositionThreshold = 30 * time.Minute
	d := New([]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: tracker}}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	parent := &forge.Issue{Number: 100, Title: "big task", Body: "lots of work"}
	tracker.issues[100] = parent

	dec := &mockDecomposer{
		result: []SubTask{
			{Title: "child 1", Body: "do part 1", AgentType: models.AgentTypeCodeGen},
			{Title: "child 2", Body: "do part 2", AgentType: models.AgentTypeTest},
		},
	}
	d.decomposer = dec

	decomposed, err := d.decomposeAndCreateChildren(
		context.Background(), "test", "repo", parent, nil, models.AgentTypeCodeGen, 60*time.Minute,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decomposed {
		t.Fatal("expected decomposition to occur")
	}
	if !dec.called {
		t.Error("decomposer should have been called")
	}

	// Verify child issues were created.
	if len(tracker.issues) != 3 { // parent + 2 children
		t.Errorf("expected 3 issues, got %d", len(tracker.issues))
	}

	// Verify children have correct labels.
	// mockTracker assigns num = len(issues)+1, so children get 2 and 3
	// (parent at 100 makes len=1 before first child).
	for _, num := range []int{2, 3} {
		child := tracker.issues[num]
		if child == nil {
			t.Errorf("child #%d not found", num)
			continue
		}
		hasDecomposed := false
		hasQueued := false
		for _, l := range child.Labels {
			if l == "decomposed" {
				hasDecomposed = true
			}
			if l == models.LabelStatusQueued {
				hasQueued = true
			}
		}
		if !hasDecomposed {
			t.Errorf("child #%d missing 'decomposed' label", num)
		}
		if !hasQueued {
			t.Errorf("child #%d missing 'status:queued' label", num)
		}
	}

	// Verify parent was blocked (AddLabel and AddComment called).
	hasBlockLabel := false
	hasBlockComment := false
	for _, call := range tracker.calls {
		if call == "AddLabel" {
			hasBlockLabel = true
		}
		if call == "AddComment" {
			hasBlockComment = true
		}
	}
	if !hasBlockLabel {
		t.Error("expected AddLabel call for blocking parent")
	}
	if !hasBlockComment {
		t.Error("expected AddComment call for blocking parent")
	}
}

func TestDecomposeAndCreateChildren_SingleSubtaskSkipped(t *testing.T) {
	tracker := newMockTracker()
	cfg := DefaultConfig()
	cfg.DecompositionThreshold = 30 * time.Minute
	d := New([]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: tracker}}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	issue := &forge.Issue{Number: 1, Title: "test", Body: "body"}
	tracker.issues[1] = issue

	dec := &mockDecomposer{
		result: []SubTask{
			{Title: "only child", Body: "still one task"},
		},
	}
	d.decomposer = dec

	decomposed, err := d.decomposeAndCreateChildren(
		context.Background(), "test", "repo", issue, nil, models.AgentTypeCodeGen, 60*time.Minute,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decomposed {
		t.Error("expected no decomposition with single subtask")
	}
}

func TestBuildChildBody(t *testing.T) {
	body := buildChildBody(42, models.AgentTypeCodeGen, "implement feature X", nil)
	if body == "" {
		t.Fatal("expected non-empty body")
	}

	// Verify frontmatter contains parent reference.
	if got := body; !contains(got, "parent_issue: 42") {
		t.Errorf("body missing parent_issue: %s", got)
	}
	if !contains(body, "agent_type: code-gen") {
		t.Errorf("body missing agent_type: %s", body)
	}
	if !contains(body, "implement feature X") {
		t.Errorf("body missing description: %s", body)
	}
}

func TestBuildChildBody_WithDependsOn(t *testing.T) {
	body := buildChildBody(42, models.AgentTypeTest, "write tests", []int{10, 11})
	if !contains(body, "depends_on:") {
		t.Errorf("body missing depends_on: %s", body)
	}
	if !contains(body, "- 10") || !contains(body, "- 11") {
		t.Errorf("body missing dependency numbers: %s", body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
