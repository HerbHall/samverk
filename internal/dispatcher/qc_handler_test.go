package dispatcher

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/pkg/models"
)

// newTestMetrics creates a DispatcherMetrics instance for tests.
func newTestMetrics() *metrics.DispatcherMetrics {
	return metrics.NewDispatcherMetrics()
}

func TestParseQCVerdict(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantVerdict QCVerdict
		wantFailed  int
	}{
		{
			name:        "PASS verdict",
			output:      "## QC Review\n\n### Verdict: [PASS]\n\n- [x] All criteria met",
			wantVerdict: QCVerdictPass,
			wantFailed:  0,
		},
		{
			name:        "FAIL verdict with failed items",
			output:      "## QC Review\n\n### Verdict: [FAIL]\n\n- [x] Criterion 1\n- [ ] Criterion 2 -- MISSING\n- [ ] Criterion 3 -- MISSING",
			wantVerdict: QCVerdictFail,
			wantFailed:  2,
		},
		{
			name:        "REVIEW verdict",
			output:      "### Verdict: [REVIEW]\n\nAmbiguous requirements need human judgment.",
			wantVerdict: QCVerdictReview,
			wantFailed:  0,
		},
		{
			name:        "no verdict defaults to REVIEW",
			output:      "I couldn't determine the outcome. The code looks okay but tests are missing.",
			wantVerdict: QCVerdictReview,
			wantFailed:  0,
		},
		{
			name:        "PASS standalone",
			output:      "[PASS] -- all acceptance criteria met.",
			wantVerdict: QCVerdictPass,
			wantFailed:  0,
		},
		{
			name:        "FAIL standalone",
			output:      "[FAIL] -- missing test for retry path.",
			wantVerdict: QCVerdictFail,
			wantFailed:  0,
		},
		{
			name:        "multiple verdicts picks first",
			output:      "### Verdict: [FAIL]\n\n[PASS] mentioned in body",
			wantVerdict: QCVerdictFail,
			wantFailed:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseQCVerdict(tt.output)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("Verdict: got %q, want %q", result.Verdict, tt.wantVerdict)
			}
			if len(result.FailedItems) != tt.wantFailed {
				t.Errorf("FailedItems: got %d, want %d", len(result.FailedItems), tt.wantFailed)
			}
		})
	}
}

func TestParseAcceptanceCriteria(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "standard acceptance criteria section",
			body: `## Problem

Something is broken.

## Acceptance Criteria

- [ ] Fix the bug
- [ ] Add tests
- [ ] Update docs

## References

- some link`,
			want: []string{"Fix the bug", "Add tests", "Update docs"},
		},
		{
			name: "mixed checked and unchecked",
			body: `## Acceptance Criteria

- [x] Already done
- [ ] Still needed
- [X] Also done`,
			want: []string{"Already done", "Still needed", "Also done"},
		},
		{
			name: "no heading fallback to all checkboxes",
			body: `Some issue body

- [ ] First thing
- [ ] Second thing`,
			want: []string{"First thing", "Second thing"},
		},
		{
			name: "empty body",
			body: "",
			want: []string{},
		},
		{
			name: "no checkboxes",
			body: "## Acceptance Criteria\n\nJust some text without checkboxes.",
			want: []string{},
		},
		{
			name: "case insensitive heading",
			body: "## acceptance criteria\n\n- [ ] Works with lowercase heading",
			want: []string{"Works with lowercase heading"},
		},
		{
			name: "stops at next heading",
			body: `## Acceptance Criteria

- [ ] In section

## Other Section

- [ ] Not in section`,
			want: []string{"In section"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAcceptanceCriteria(tt.body)
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %d, want %d\ngot: %v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractFailedItems(t *testing.T) {
	input := `### Acceptance Criteria
- [x] Criterion 1 -- verified
- [ ] Criterion 2 -- MISSING: no test
- [x] Criterion 3 -- verified
- [ ] Criterion 4 -- MISSING: not implemented`

	got := extractFailedItems(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 failed items, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "Criterion 2") {
		t.Errorf("expected criterion 2, got %q", got[0])
	}
	if !strings.Contains(got[1], "Criterion 4") {
		t.Errorf("expected criterion 4, got %q", got[1])
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		maxLen int
		want   string
	}{
		{"short text", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateOutput(tt.text, tt.maxLen)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleQCComplete_Pass(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[42] = &forge.Issue{
		Number: 42,
		Title:  "test issue",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsQc},
	}

	d := &Dispatcher{
		trackers:       map[string]forge.IssueTracker{"owner/repo": tracker},
		trackerEntries: []TrackerEntry{{Owner: "owner", Repo: "repo", Tracker: tracker}},
		claimed:        make(map[string]*claimedIssue),
		issueFailures:  make(map[string]int),
		metrics:        newTestMetrics(),
		logger:         zap.NewNop(),
	}

	// Simulate QC pass via comments.
	ctx := context.Background()
	_, _ = tracker.AddComment(ctx, 42, "**QC Review: [PASS]** (automated)\n\nAll criteria met.")

	result := agent.TaskResult{
		IssueNumber: 42,
		Owner:       "owner",
		Repo:        "repo",
		SessionID:   "sess_qc_42_1",
		AgentType:   models.AgentTypeQC,
		ProviderKey: "qc",
		Success:     true,
	}

	d.handleQCComplete(ctx, result)

	// Verify needs-qc label was removed.
	issue := tracker.issues[42]
	for _, l := range issue.Labels {
		if l == models.LabelStatusNeedsQc {
			t.Error("expected needs-qc label to be removed")
		}
	}
}

func TestHandleQCComplete_Fail(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[42] = &forge.Issue{
		Number: 42,
		Title:  "test issue",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsQc},
	}

	d := &Dispatcher{
		trackers:       map[string]forge.IssueTracker{"owner/repo": tracker},
		trackerEntries: []TrackerEntry{{Owner: "owner", Repo: "repo", Tracker: tracker}},
		claimed:        make(map[string]*claimedIssue),
		issueFailures:  make(map[string]int),
		metrics:        newTestMetrics(),
		logger:         zap.NewNop(),
	}

	ctx := context.Background()
	_, _ = tracker.AddComment(ctx, 42, "**QC Review: [FAIL]**\n\n- [ ] Missing test for retry path")

	result := agent.TaskResult{
		IssueNumber: 42,
		Owner:       "owner",
		Repo:        "repo",
		SessionID:   "sess_qc_42_1",
		AgentType:   models.AgentTypeQC,
		ProviderKey: "qc",
		Success:     true,
	}

	d.handleQCComplete(ctx, result)

	// Verify issue was re-queued (correction engine adds status:queued).
	issue := tracker.issues[42]
	hasQueued := false
	for _, l := range issue.Labels {
		if l == models.LabelStatusQueued {
			hasQueued = true
		}
	}
	if !hasQueued {
		t.Error("expected status:queued label after QC fail")
	}
}

func TestHandleQCComplete_Review(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[42] = &forge.Issue{
		Number: 42,
		Title:  "test issue",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsQc},
	}

	d := &Dispatcher{
		trackers:       map[string]forge.IssueTracker{"owner/repo": tracker},
		trackerEntries: []TrackerEntry{{Owner: "owner", Repo: "repo", Tracker: tracker}},
		claimed:        make(map[string]*claimedIssue),
		issueFailures:  make(map[string]int),
		metrics:        newTestMetrics(),
		logger:         zap.NewNop(),
	}

	ctx := context.Background()
	_, _ = tracker.AddComment(ctx, 42, "**QC Review: [REVIEW]**\n\nNeeds human judgment.")

	result := agent.TaskResult{
		IssueNumber: 42,
		Owner:       "owner",
		Repo:        "repo",
		SessionID:   "sess_qc_42_1",
		AgentType:   models.AgentTypeQC,
		ProviderKey: "qc",
		Success:     true,
	}

	d.handleQCComplete(ctx, result)

	// Verify needs-human label was added.
	issue := tracker.issues[42]
	hasNeedsHuman := false
	for _, l := range issue.Labels {
		if l == models.LabelStatusNeedsHuman {
			hasNeedsHuman = true
		}
	}
	if !hasNeedsHuman {
		t.Error("expected status:needs-human label after QC review")
	}
}

func TestHandleQCComplete_AgentFailure(t *testing.T) {
	tracker := newMockTracker()
	tracker.issues[42] = &forge.Issue{
		Number: 42,
		Title:  "test issue",
		State:  forge.StateOpen,
		Labels: []string{},
	}

	d := &Dispatcher{
		trackers:       map[string]forge.IssueTracker{"owner/repo": tracker},
		trackerEntries: []TrackerEntry{{Owner: "owner", Repo: "repo", Tracker: tracker}},
		claimed:        make(map[string]*claimedIssue),
		issueFailures:  make(map[string]int),
		metrics:        newTestMetrics(),
		logger:         zap.NewNop(),
	}

	ctx := context.Background()

	result := agent.TaskResult{
		IssueNumber: 42,
		Owner:       "owner",
		Repo:        "repo",
		SessionID:   "sess_qc_42_1",
		AgentType:   models.AgentTypeQC,
		ProviderKey: "qc",
		Success:     false,
		Error:       "provider timeout",
	}

	d.handleQCComplete(ctx, result)

	// Should fall back to needs-qc for manual review.
	issue := tracker.issues[42]
	hasNeedsQC := false
	for _, l := range issue.Labels {
		if l == models.LabelStatusNeedsQc {
			hasNeedsQC = true
		}
	}
	if !hasNeedsQC {
		t.Error("expected status:needs-qc label after QC agent failure")
	}
}

func TestSelectQCProvider_DifferentFromGenerator(t *testing.T) {
	d := &Dispatcher{
		logger: zap.NewNop(),
	}

	issue := &forge.Issue{
		Number: 42,
		Title:  "test issue",
		Labels: []string{},
	}

	// Without a pool, selectQCProvider falls back to selectProviderKey.
	key := d.selectQCProvider(issue, "code-gen")
	if key == "" {
		t.Error("expected a non-empty provider key")
	}
	// The QC chain should be "qc" for QC agent type.
	if key != "qc" {
		t.Errorf("expected 'qc' provider key, got %q", key)
	}
}
