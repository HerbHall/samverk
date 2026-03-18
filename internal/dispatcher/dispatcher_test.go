package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"time"

	"github.com/herbhall/samverk/internal/agent"
	"github.com/herbhall/samverk/internal/autonomy"
	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// mockTracker implements forge.IssueTracker for testing.
type mockTracker struct {
	mu       sync.Mutex
	issues   map[int]*forge.Issue
	comments map[int][]*forge.Comment
	calls    []string
}

func newMockTracker() *mockTracker {
	return &mockTracker{
		issues:   make(map[int]*forge.Issue),
		comments: make(map[int][]*forge.Comment),
	}
}

func (m *mockTracker) record(method string) {
	m.mu.Lock()
	m.calls = append(m.calls, method)
	m.mu.Unlock()
}

func (m *mockTracker) CreateIssue(_ context.Context, req *forge.CreateIssueRequest) (*forge.Issue, error) {
	m.record("CreateIssue")
	m.mu.Lock()
	defer m.mu.Unlock()
	num := len(m.issues) + 1
	issue := &forge.Issue{
		Number:    num,
		Title:     req.Title,
		Body:      req.Body,
		State:     forge.StateOpen,
		Labels:    req.Labels,
		Assignees: req.Assignees,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.issues[num] = issue
	return issue, nil
}

func (m *mockTracker) GetIssue(_ context.Context, number int) (*forge.Issue, error) {
	m.record("GetIssue")
	m.mu.Lock()
	defer m.mu.Unlock()
	issue, ok := m.issues[number]
	if !ok {
		return nil, fmt.Errorf("issue #%d not found", number)
	}
	return issue, nil
}

func (m *mockTracker) UpdateIssue(_ context.Context, number int, req *forge.UpdateIssueRequest) (*forge.Issue, error) {
	m.record("UpdateIssue")
	m.mu.Lock()
	defer m.mu.Unlock()
	issue, ok := m.issues[number]
	if !ok {
		return nil, fmt.Errorf("issue #%d not found", number)
	}
	if req.Title != nil {
		issue.Title = *req.Title
	}
	if req.Body != nil {
		issue.Body = *req.Body
	}
	if req.State != nil {
		issue.State = *req.State
	}
	issue.UpdatedAt = time.Now()
	return issue, nil
}

func (m *mockTracker) ListIssues(_ context.Context, opts *forge.ListOptions) ([]*forge.Issue, error) {
	m.record("ListIssues")
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*forge.Issue, 0)
	for _, issue := range m.issues {
		if opts != nil {
			if opts.State != "" && issue.State != opts.State {
				continue
			}
			if len(opts.Labels) > 0 && !hasAllLabels(issue.Labels, opts.Labels) {
				continue
			}
		}
		result = append(result, issue)
	}
	return result, nil
}

func hasAllLabels(issueLabels, required []string) bool {
	set := make(map[string]bool, len(issueLabels))
	for _, l := range issueLabels {
		set[l] = true
	}
	for _, r := range required {
		if !set[r] {
			return false
		}
	}
	return true
}

func (m *mockTracker) AddComment(_ context.Context, number int, body string) (*forge.Comment, error) {
	m.record("AddComment")
	m.mu.Lock()
	defer m.mu.Unlock()
	c := &forge.Comment{
		ID:        int64(len(m.comments[number]) + 1),
		Body:      body,
		Author:    "dispatcher",
		CreatedAt: time.Now(),
	}
	m.comments[number] = append(m.comments[number], c)
	return c, nil
}

func (m *mockTracker) ListComments(_ context.Context, number int) ([]*forge.Comment, error) {
	m.record("ListComments")
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.comments[number], nil
}

func (m *mockTracker) SetLabels(_ context.Context, number int, labels []string) error {
	m.record("SetLabels")
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue, ok := m.issues[number]; ok {
		issue.Labels = labels
	}
	return nil
}

func (m *mockTracker) AddLabel(_ context.Context, number int, label string) error {
	m.record("AddLabel")
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue, ok := m.issues[number]; ok {
		for _, l := range issue.Labels {
			if l == label {
				return nil
			}
		}
		issue.Labels = append(issue.Labels, label)
	}
	return nil
}

func (m *mockTracker) RemoveLabel(_ context.Context, number int, label string) error {
	m.record("RemoveLabel")
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue, ok := m.issues[number]; ok {
		filtered := make([]string, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			if l != label {
				filtered = append(filtered, l)
			}
		}
		issue.Labels = filtered
	}
	return nil
}

func (m *mockTracker) Assign(_ context.Context, number int, assignee string) error {
	m.record("Assign")
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue, ok := m.issues[number]; ok {
		issue.Assignees = append(issue.Assignees, assignee)
	}
	return nil
}

func (m *mockTracker) Unassign(_ context.Context, number int, assignee string) error {
	m.record("Unassign")
	m.mu.Lock()
	defer m.mu.Unlock()
	if issue, ok := m.issues[number]; ok {
		filtered := make([]string, 0, len(issue.Assignees))
		for _, a := range issue.Assignees {
			if a != assignee {
				filtered = append(filtered, a)
			}
		}
		issue.Assignees = filtered
	}
	return nil
}

func (m *mockTracker) Watch(ctx context.Context, handler func(forge.Event)) error {
	// The Watch method does nothing by default; tests send events directly.
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockTracker) SearchIssues(_ context.Context, _ *forge.SearchOptions) ([]*forge.Issue, error) {
	return nil, nil
}

// --- Mock autonomy policy ---

type mockPolicy struct {
	costThreshold float64
}

func (p *mockPolicy) TierFor(_ autonomy.ActionType) autonomy.Tier {
	return autonomy.Tier1
}

func (p *mockPolicy) RequiresConfirmation(_ autonomy.ActionType) bool {
	return false
}

func (p *mockPolicy) CostThreshold() float64 {
	return p.costThreshold
}

// --- Helper to build issue body with frontmatter ---

func issueBody(agentType string, dependsOn []int) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: \"1.0.0\"\n")
	b.WriteString("type: task\n")
	if agentType != "" {
		fmt.Fprintf(&b, "agent_type: %s\n", agentType)
	}
	b.WriteString("priority: normal\n")
	if len(dependsOn) > 0 {
		b.WriteString("depends_on:\n")
		for _, dep := range dependsOn {
			fmt.Fprintf(&b, "  - %d\n", dep)
		}
	}
	b.WriteString("---\n\n## Summary\n\nTest issue.\n")
	return b.String()
}

func newTestDispatcher(tracker *mockTracker) *Dispatcher {
	cfg := DefaultConfig()
	// Use short intervals for tests.
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond
	return New([]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: tracker}}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())
}

// --- Tests ---

func TestNewDispatcher(t *testing.T) {
	tracker := newMockTracker()
	d := New([]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: tracker}}, &mockPolicy{costThreshold: 5.0}, nil, nil, nil, zap.NewNop())
	if d == nil {
		t.Fatal("expected non-nil dispatcher")
	}
	if d.config == nil {
		t.Fatal("expected non-nil config")
	}
	if d.config.MaxConsecutiveFailures != 3 {
		t.Errorf("MaxConsecutiveFailures: got %d, want 3", d.config.MaxConsecutiveFailures)
	}
	if d.claimed == nil {
		t.Fatal("expected non-nil claimed map")
	}
	if d.issueFailures == nil {
		t.Fatal("expected non-nil issueFailures map")
	}
}

func TestHandleOpened_ValidFrontmatter(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 1,
		Title:  "Add widget",
		Body:   issueBody("code-gen", nil),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should be claimed and assigned.
	if !hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed label")
	}
	if hasLabel(issue.Labels, "status:queued") {
		t.Error("did not expect status:queued label after routing")
	}
	if len(issue.Assignees) == 0 || issue.Assignees[0] != "code-gen" {
		t.Errorf("expected assignee code-gen, got %v", issue.Assignees)
	}
}

func TestHandleOpened_InvalidAgentType(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 1,
		Title:  "Bad type",
		Body:   issueBody("nonexistent-type", nil),
		State:  forge.StateOpen,
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasLabel(issue.Labels, "status:needs-human") {
		t.Error("expected status:needs-human label for invalid agent type")
	}
}

func TestHandleOpened_NoFrontmatter(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 1,
		Title:  "Plain issue",
		Body:   "No frontmatter here, just a plain issue body.",
		State:  forge.StateOpen,
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasLabel(issue.Labels, "status:needs-human") {
		t.Error("expected status:needs-human label for missing frontmatter")
	}
}

func TestClassifyByHeuristic(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		labels   []string
		wantType models.AgentType
	}{
		{
			name:     "agent:human label",
			title:    "Some issue",
			labels:   []string{"agent:human"},
			wantType: models.AgentTypeHuman,
		},
		{
			name:     "type:spike label",
			title:    "Investigate options",
			labels:   []string{"type:spike"},
			wantType: models.AgentTypeResearch,
		},
		{
			name:     "type:research label",
			title:    "Explore alternatives",
			labels:   []string{"type:research"},
			wantType: models.AgentTypeResearch,
		},
		{
			name:     "bug label",
			title:    "Something is broken",
			labels:   []string{"bug"},
			wantType: models.AgentTypeCodeGen,
		},
		{
			name:     "fix: title prefix",
			title:    "fix: null pointer in handler",
			labels:   nil,
			wantType: models.AgentTypeCodeGen,
		},
		{
			name:     "feat: title prefix",
			title:    "feat: add dark mode",
			labels:   nil,
			wantType: models.AgentTypeCodeGen,
		},
		{
			name:     "feature: title prefix",
			title:    "feature: new dashboard",
			labels:   nil,
			wantType: models.AgentTypeCodeGen,
		},
		{
			name:     "docs: title prefix",
			title:    "docs: update README",
			labels:   nil,
			wantType: models.AgentTypeDocs,
		},
		{
			name:     "chore: title prefix",
			title:    "chore: bump dependencies",
			labels:   nil,
			wantType: models.AgentTypeDocs,
		},
		{
			name:     "test: title prefix",
			title:    "test: add unit tests for router",
			labels:   nil,
			wantType: models.AgentTypeTest,
		},
		{
			name:     "agent:human takes priority over bug",
			title:    "Something broken",
			labels:   []string{"agent:human", "bug"},
			wantType: models.AgentTypeHuman,
		},
		{
			name:     "no match returns empty",
			title:    "Random issue title",
			labels:   []string{"enhancement"},
			wantType: "",
		},
		{
			name:     "case insensitive title prefix",
			title:    "FIX: uppercase prefix",
			labels:   nil,
			wantType: models.AgentTypeCodeGen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &forge.Issue{
				Number: 99,
				Title:  tt.title,
				Labels: tt.labels,
			}
			got := classifyByHeuristic(issue)
			if got != tt.wantType {
				t.Errorf("classifyByHeuristic() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestHandleOpened_NoFrontmatter_BugLabel(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 1,
		Title:  "Something is broken",
		Body:   "No frontmatter here, just a plain issue body.",
		State:  forge.StateOpen,
		Labels: []string{"bug"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasLabel(issue.Labels, "status:needs-human") {
		t.Error("did not expect status:needs-human for issue with bug label")
	}
	if !hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed after heuristic routing")
	}
}

func TestHandleOpened_NoFrontmatter_AgentHumanLabel(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number: 1,
		Title:  "Design review needed",
		Body:   "Plain issue without frontmatter.",
		State:  forge.StateOpen,
		Labels: []string{"agent:human"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// agent:human issues are now intercepted by the route() human gate:
	// status:needs-human is added and no pool submission occurs.
	if !hasLabel(issue.Labels, "status:needs-human") {
		t.Error("expected status:needs-human for agent:human classified issue")
	}
	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("did not expect status:claimed — human issues must not be dispatched")
	}
}

func TestHandleOpened_WithDependencies_AllDone(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Create a closed dependency with status:done.
	closedAt := time.Now()
	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	issue := &forge.Issue{
		Number: 1,
		Title:  "Depends on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be routed, not blocked.
	if hasLabel(issue.Labels, "status:blocked") {
		t.Error("did not expect status:blocked -- dependency is done")
	}
	if !hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed after routing")
	}
}

func TestHandleOpened_WithDependencies_Blocked(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Dependency is open (not done).
	tracker.issues[10] = &forge.Issue{
		Number: 10,
		State:  forge.StateOpen,
		Labels: []string{"status:in-progress"},
		Body:   issueBody("code-gen", nil),
	}

	issue := &forge.Issue{
		Number: 1,
		Title:  "Depends on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{"status:queued"},
	}
	tracker.issues[1] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       issue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked for unsatisfied dependency")
	}
}

func TestHandleClosed_UnblocksDependents(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Dependency issue that will be closed.
	closedAt := time.Now()
	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{"status:done"},
		ClosedAt: &closedAt,
	}

	// Blocked issue waiting on #10.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	}

	err := d.handleClosed(context.Background(), forge.Event{
		Type:        forge.EventIssueClosed,
		IssueNumber: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue #1 should be unblocked.
	issue := tracker.issues[1]
	if hasLabel(issue.Labels, "status:blocked") {
		t.Error("expected status:blocked to be removed")
	}
	if !hasLabel(issue.Labels, "status:queued") {
		t.Error("expected status:queued after unblock")
	}
}

func TestDetectCycle_NoCycle(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Linear chain: 1 -> 2 -> 3 (no cycle).
	tracker.issues[1] = &forge.Issue{Number: 1, State: forge.StateOpen, Body: issueBody("code-gen", []int{2})}
	tracker.issues[2] = &forge.Issue{Number: 2, State: forge.StateOpen, Body: issueBody("code-gen", []int{3})}
	tracker.issues[3] = &forge.Issue{Number: 3, State: forge.StateOpen, Body: issueBody("code-gen", nil)}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle != nil {
		t.Errorf("expected no cycle, got %v", cycle)
	}
}

func TestDetectCycle_WithCycle(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Cycle: 1 -> 2 -> 3 -> 1.
	tracker.issues[1] = &forge.Issue{Number: 1, State: forge.StateOpen, Body: issueBody("code-gen", []int{2})}
	tracker.issues[2] = &forge.Issue{Number: 2, State: forge.StateOpen, Body: issueBody("code-gen", []int{3})}
	tracker.issues[3] = &forge.Issue{Number: 3, State: forge.StateOpen, Body: issueBody("code-gen", []int{1})}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected cycle, got nil")
	}
	// Cycle should contain all three nodes.
	if len(cycle) < 3 {
		t.Errorf("expected cycle of length >= 3, got %v", cycle)
	}
}

func TestParseHeartbeat_Valid(t *testing.T) {
	body := "HEARTBEAT [agent-codegen-1] [2026-02-28T10:30:00Z]\nprogress: 45%\nstatus: Running test suite"
	hb := parseHeartbeat(body)
	if hb == nil {
		t.Fatal("expected non-nil heartbeat")
	}
	if hb.AgentID != "agent-codegen-1" {
		t.Errorf("AgentID: got %q, want %q", hb.AgentID, "agent-codegen-1")
	}
	if hb.Progress != 45 {
		t.Errorf("Progress: got %d, want 45", hb.Progress)
	}
	if hb.Status != "Running test suite" {
		t.Errorf("Status: got %q, want %q", hb.Status, "Running test suite")
	}
	expectedTS, _ := time.Parse(time.RFC3339, "2026-02-28T10:30:00Z")
	if !hb.Timestamp.Equal(expectedTS) {
		t.Errorf("Timestamp: got %v, want %v", hb.Timestamp, expectedTS)
	}
}

func TestParseHeartbeat_Invalid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "plain comment", body: "This is just a regular comment."},
		{name: "partial match", body: "HEARTBEAT without brackets"},
		{name: "bad timestamp", body: "HEARTBEAT [agent-1] [not-a-timestamp]\nprogress: 50%\nstatus: working"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hb := parseHeartbeat(tt.body)
			if hb != nil {
				t.Errorf("expected nil heartbeat for %q, got %+v", tt.body, hb)
			}
		})
	}
}

func TestCheckTimeouts_NoTimeout(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Claim an issue with a recent heartbeat.
	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 1)] = &claimedIssue{
		AgentID:       "code-gen",
		ClaimedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}
	d.mu.Unlock()

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}

	err := d.checkTimeouts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should still be claimed.
	d.mu.Lock()
	_, stillClaimed := d.claimed[issueKey("test", "repo", 1)]
	d.mu.Unlock()
	if !stillClaimed {
		t.Error("expected issue to remain claimed (no timeout)")
	}
}

func TestCheckTimeouts_TimedOut(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Claim an issue with an old heartbeat.
	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 1)] = &claimedIssue{
		AgentID:       "code-gen",
		ClaimedAt:     time.Now().Add(-time.Hour),
		LastHeartbeat: time.Now().Add(-time.Hour),
	}
	d.mu.Unlock()

	tracker.issues[1] = &forge.Issue{
		Number:    1,
		State:     forge.StateOpen,
		Labels:    []string{"status:claimed"},
		Assignees: []string{"code-gen"},
	}

	err := d.checkTimeouts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Issue should be released.
	d.mu.Lock()
	_, stillClaimed := d.claimed[issueKey("test", "repo", 1)]
	d.mu.Unlock()
	if stillClaimed {
		t.Error("expected issue to be released after timeout")
	}

	issue := tracker.issues[1]
	if !hasLabel(issue.Labels, "status:queued") {
		t.Error("expected status:queued after timeout release")
	}
}

func TestCheckTimeouts_ThreeFailures(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Issue with 2 prior failures -- next timeout will be the 3rd.
	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 1)] = &claimedIssue{
		AgentID:       "code-gen",
		ClaimedAt:     time.Now().Add(-time.Hour),
		LastHeartbeat: time.Now().Add(-time.Hour),
		FailureCount:  2,
	}
	d.mu.Unlock()

	tracker.issues[1] = &forge.Issue{
		Number:    1,
		State:     forge.StateOpen,
		Labels:    []string{"status:claimed"},
		Assignees: []string{"code-gen"},
	}

	err := d.checkTimeouts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	issue := tracker.issues[1]
	if !hasLabel(issue.Labels, "status:needs-human") {
		t.Error("expected status:needs-human after 3 consecutive failures")
	}
}

// TestCheckTimeouts_MultiCycleRetryEscalation verifies that the failure counter
// increments correctly across re-queue cycles (1 → 2 → 3) and that the issue
// escalates to status:needs-human only after MaxConsecutiveFailures timeouts,
// not on the first one.
func TestCheckTimeouts_MultiCycleRetryEscalation(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)
	ctx := context.Background()

	tracker.issues[1] = &forge.Issue{
		Number:    1,
		State:     forge.StateOpen,
		Labels:    []string{"status:claimed"},
		Assignees: []string{"code-gen"},
	}

	// Simulate maxRetries timeout cycles. Each cycle:
	//   1. Set claimed entry with an old heartbeat (mimics agent silence).
	//   2. Call checkTimeouts — releases and increments failure count.
	//   3. Verify escalation only happens on the final cycle.
	maxRetries := d.config.MaxConsecutiveFailures // 3
	for cycle := 1; cycle <= maxRetries; cycle++ {
		d.mu.Lock()
		// Restore the claimed entry as if the issue was re-queued and re-dispatched,
		// carrying forward the failure count from issueFailures (what route() does).
		priorFailures := d.issueFailures[issueKey("test", "repo", 1)]
		d.claimed[issueKey("test", "repo", 1)] = &claimedIssue{
			AgentID:       "code-gen",
			ClaimedAt:     time.Now().Add(-time.Hour),
			LastHeartbeat: time.Now().Add(-time.Hour),
			FailureCount:  priorFailures,
		}
		d.mu.Unlock()

		if err := d.checkTimeouts(ctx); err != nil {
			t.Fatalf("cycle %d: unexpected error: %v", cycle, err)
		}

		// Verify the persisted failure count matches the cycle number.
		d.mu.Lock()
		persisted := d.issueFailures[issueKey("test", "repo", 1)]
		d.mu.Unlock()
		if persisted != cycle {
			t.Errorf("cycle %d: issueFailures[1] = %d, want %d", cycle, persisted, cycle)
		}

		issue := tracker.issues[1]
		if cycle < maxRetries {
			// Should NOT be escalated yet.
			if hasLabel(issue.Labels, "status:needs-human") {
				t.Errorf("cycle %d: premature escalation to status:needs-human", cycle)
			}
			if !hasLabel(issue.Labels, "status:queued") {
				t.Errorf("cycle %d: expected status:queued after release", cycle)
			}
		} else if !hasLabel(issue.Labels, "status:needs-human") {
			// Final cycle — must escalate.
			t.Errorf("cycle %d: expected status:needs-human after %d failures", cycle, maxRetries)
		}
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HeartbeatInterval != 20*time.Minute {
		t.Errorf("HeartbeatInterval: got %v, want 20m", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatTimeoutMultiplier != 1.5 {
		t.Errorf("HeartbeatTimeoutMultiplier: got %v, want 1.5", cfg.HeartbeatTimeoutMultiplier)
	}
	if cfg.MaxConsecutiveFailures != 3 {
		t.Errorf("MaxConsecutiveFailures: got %d, want 3", cfg.MaxConsecutiveFailures)
	}
	if cfg.DependencyRecheckInterval != 2*time.Minute {
		t.Errorf("DependencyRecheckInterval: got %v, want 2m", cfg.DependencyRecheckInterval)
	}
	if cfg.HeartbeatCheckInterval != 60*time.Second {
		t.Errorf("HeartbeatCheckInterval: got %v, want 60s", cfg.HeartbeatCheckInterval)
	}
}

// --- Helpers ---

func TestHandleOpened_SkipsPullRequest(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number:        99,
		Title:         "feat: add widget",
		Body:          "PR body without frontmatter.",
		State:         forge.StateOpen,
		IsPullRequest: true,
	}
	tracker.issues[99] = issue

	err := d.handleOpened(context.Background(), forge.Event{
		Type:          forge.EventIssueOpened,
		IssueNumber:   99,
		Issue:         issue,
		IsPullRequest: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PR should NOT be escalated or routed.
	if hasLabel(issue.Labels, "status:needs-human") {
		t.Error("PR should not be escalated to needs-human")
	}
	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("PR should not be routed (status:claimed)")
	}
	if hasLabel(issue.Labels, "status:queued") {
		t.Error("PR should not be queued")
	}
}

func TestHandleEvent_AssignedDoesNotError(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	issue := &forge.Issue{
		Number:    5,
		Title:     "some issue",
		Body:      "body",
		State:     forge.StateOpen,
		Assignees: []string{"octocat"},
	}
	tracker.issues[5] = issue

	err := d.handleAssigned(context.Background(), forge.Event{
		Type:        forge.EventIssueAssigned,
		IssueNumber: 5,
		Issue:       issue,
		Assignee:    "octocat",
	})
	if err != nil {
		t.Fatalf("handleAssigned returned error: %v", err)
	}
}

func TestHandleEvent_UnknownTypeStillLogs(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Verify issue.assigned is no longer treated as unknown.
	d.handleEvent(context.Background(), forge.Event{
		Type:        forge.EventIssueAssigned,
		IssueNumber: 1,
	})
	// No panic, no error — the handler exists now.
}

func TestHandleOpened_SkipsNeedsHuman(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[10] = &forge.Issue{
		Number: 10,
		Title:  "Needs human review",
		Body:   "Some body",
		State:  forge.StateOpen,
		Labels: []string{"status:needs-human"},
	}
	tracker.mu.Unlock()

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 10,
	})
	if err != nil {
		t.Fatalf("handleOpened returned error: %v", err)
	}

	tracker.mu.Lock()
	issue := tracker.issues[10]
	tracker.mu.Unlock()

	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected no status:claimed label — issue should be skipped")
	}
}

func TestHandleOpened_SkipsBlocked(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[11] = &forge.Issue{
		Number: 11,
		Title:  "Blocked issue",
		Body:   "Some body",
		State:  forge.StateOpen,
		Labels: []string{"status:blocked"},
	}
	tracker.mu.Unlock()

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 11,
	})
	if err != nil {
		t.Fatalf("handleOpened returned error: %v", err)
	}

	tracker.mu.Lock()
	issue := tracker.issues[11]
	tracker.mu.Unlock()

	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected no status:claimed label — issue should be skipped")
	}
}

func TestHandleOpened_SkipsClaimed(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[12] = &forge.Issue{
		Number: 12,
		Title:  "Already claimed",
		Body:   "Some body",
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}
	tracker.mu.Unlock()

	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 12,
	})
	if err != nil {
		t.Fatalf("handleOpened returned error: %v", err)
	}

	tracker.mu.Lock()
	calls := make([]string, len(tracker.calls))
	copy(calls, tracker.calls)
	tracker.mu.Unlock()

	// Verify Assign was never called (no routing occurred).
	for _, c := range calls {
		if c == "Assign" {
			t.Error("Assign was called — issue should have been skipped before routing")
		}
	}
}

func TestRoute_HumanAgentNotDispatched(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.mu.Lock()
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Human task",
		Body:   "Requires human decision",
		State:  forge.StateOpen,
		Labels: []string{"agent:human"},
	}
	tracker.mu.Unlock()

	issue := &forge.Issue{
		Number: 20,
		Title:  "Human task",
		Body:   "Requires human decision",
		State:  forge.StateOpen,
		Labels: []string{"agent:human"},
	}

	err := d.route(context.Background(), "test", "repo", issue, models.AgentTypeHuman, nil)
	if err != nil {
		t.Fatalf("route returned error: %v", err)
	}

	tracker.mu.Lock()
	routedIssue := tracker.issues[20]
	calls := make([]string, len(tracker.calls))
	copy(calls, tracker.calls)
	tracker.mu.Unlock()

	// status:needs-human must be set.
	if !hasLabel(routedIssue.Labels, "status:needs-human") {
		t.Error("expected status:needs-human label to be added")
	}

	// Assign must NOT be called — no agent pool submission.
	for _, c := range calls {
		if c == "Assign" {
			t.Error("Assign was called — human issues must not be dispatched to agent pool")
		}
	}

	// claimed map must not contain the issue.
	d.mu.Lock()
	_, inClaimed := d.claimed[issueKey("test", "repo", 20)]
	d.mu.Unlock()
	if inClaimed {
		t.Error("issue #20 should not be in claimed map — human issues are not routed")
	}
}

func TestHandleTaskComplete_Success(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[5] = &forge.Issue{
		Number: 5,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed", "status:in-progress"},
	}

	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 5)] = &claimedIssue{AgentID: "code-gen", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.issueFailures[issueKey("test", "repo", 5)] = 1
	d.mu.Unlock()

	d.handleTaskComplete(agent.TaskResult{
		Owner: "test", Repo: "repo",
		IssueNumber: 5,
		SessionID:   "sess-5",
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	})

	// claimed map must be cleared.
	d.mu.Lock()
	_, inClaimed := d.claimed[issueKey("test", "repo", 5)]
	_, inFailures := d.issueFailures[issueKey("test", "repo", 5)]
	d.mu.Unlock()
	if inClaimed {
		t.Error("expected issue #5 removed from claimed map")
	}
	if inFailures {
		t.Error("expected issueFailures cleared on success")
	}

	issue := tracker.issues[5]
	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed removed")
	}
	if hasLabel(issue.Labels, "status:in-progress") {
		t.Error("expected status:in-progress removed")
	}
	if !hasLabel(issue.Labels, "status:needs-qc") {
		t.Error("expected status:needs-qc added on success")
	}
}

func TestHandleTaskComplete_Failure(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[6] = &forge.Issue{
		Number: 6,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}

	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 6)] = &claimedIssue{AgentID: "code-gen", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.issueFailures[issueKey("test", "repo", 6)] = 1
	d.mu.Unlock()

	d.handleTaskComplete(agent.TaskResult{
		Owner: "test", Repo: "repo",
		IssueNumber: 6,
		SessionID:   "sess-6",
		AgentType:   models.AgentTypeCodeGen,
		Success:     false,
		Error:       "runner error",
	})

	// claimed map must be cleared but failure count preserved.
	d.mu.Lock()
	_, inClaimed := d.claimed[issueKey("test", "repo", 6)]
	failures := d.issueFailures[issueKey("test", "repo", 6)]
	d.mu.Unlock()
	if inClaimed {
		t.Error("expected issue #6 removed from claimed map")
	}
	if failures != 1 {
		t.Errorf("expected issueFailures preserved on failure, got %d", failures)
	}

	issue := tracker.issues[6]
	if hasLabel(issue.Labels, "status:claimed") {
		t.Error("expected status:claimed removed")
	}
	if !hasLabel(issue.Labels, "status:queued") {
		t.Error("expected status:queued added on failure")
	}
}

func TestNoDoubleDispatch_AfterCompletion(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[7] = &forge.Issue{
		Number: 7,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}

	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 7)] = &claimedIssue{AgentID: "code-gen", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.mu.Unlock()

	// Simulate successful completion removing issue from claimed map.
	d.handleTaskComplete(agent.TaskResult{
		Owner: "test", Repo: "repo",
		IssueNumber: 7,
		SessionID:   "sess-7",
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	})

	// Give issue a stale heartbeat to trigger timeout sweep.
	d.mu.Lock()
	_, stillClaimed := d.claimed[issueKey("test", "repo", 7)]
	d.mu.Unlock()

	if stillClaimed {
		t.Fatal("issue #7 should not be in claimed map after completion callback")
	}

	// checkTimeouts must not re-queue an already-completed (not claimed) issue.
	if err := d.checkTimeouts(context.Background()); err != nil {
		t.Fatalf("checkTimeouts: %v", err)
	}

	// Status should remain needs-qc (set by handleTaskComplete), not be overwritten.
	issue := tracker.issues[7]
	if hasLabel(issue.Labels, "status:queued") {
		t.Error("checkTimeouts re-queued a completed issue — double-dispatch bug")
	}
}

func TestHandleTaskComplete_SignalsWakeup(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[8] = &forge.Issue{
		Number: 8,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}

	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 8)] = &claimedIssue{AgentID: "code-gen", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.mu.Unlock()

	d.handleTaskComplete(agent.TaskResult{
		Owner: "test", Repo: "repo",
		IssueNumber: 8,
		SessionID:   "sess-8",
		AgentType:   models.AgentTypeCodeGen,
		Success:     true,
	})

	// The wakeup channel should have a signal.
	select {
	case <-d.wakeup:
		// Expected: wakeup was signaled.
	default:
		t.Error("expected wakeup channel to be signaled after task completion")
	}
}

func TestHandleTaskComplete_WakeupDoesNotBlock(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Pre-fill the wakeup channel to simulate an already-pending signal.
	d.wakeup <- struct{}{}

	tracker.issues[9] = &forge.Issue{
		Number: 9,
		State:  forge.StateOpen,
		Labels: []string{"status:claimed"},
	}

	d.mu.Lock()
	d.claimed[issueKey("test", "repo", 9)] = &claimedIssue{AgentID: "code-gen", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.mu.Unlock()

	// This must not block even though the channel is already full.
	done := make(chan struct{})
	go func() {
		d.handleTaskComplete(agent.TaskResult{
			IssueNumber: 9,
			SessionID:   "sess-9",
			AgentType:   models.AgentTypeCodeGen,
			Success:     true,
		})
		close(done)
	}()

	select {
	case <-done:
		// Expected: completed without blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("handleTaskComplete blocked on full wakeup channel")
	}
}

// --- Multi-repo tests ---

func TestMultiRepo_TwoTrackers_EventsRoutedCorrectly(t *testing.T) {
	trackerA := newMockTracker()
	trackerB := newMockTracker()

	cfg := DefaultConfig()
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond
	d := New([]TrackerEntry{
		{Owner: "org", Repo: "alpha", Tracker: trackerA},
		{Owner: "org", Repo: "beta", Tracker: trackerB},
	}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	// Add issues to each tracker.
	trackerA.issues[1] = &forge.Issue{
		Number: 1, Title: "feat: alpha feature",
		Body: issueBody("code-gen", nil), State: forge.StateOpen, Labels: []string{"status:queued"},
	}
	trackerB.issues[1] = &forge.Issue{
		Number: 1, Title: "feat: beta feature",
		Body: issueBody("code-gen", nil), State: forge.StateOpen, Labels: []string{"status:queued"},
	}

	// Route event from tracker A.
	err := d.handleOpened(context.Background(), forge.Event{
		Type: forge.EventIssueOpened, Owner: "org", Repo: "alpha",
		IssueNumber: 1, Issue: trackerA.issues[1],
	})
	if err != nil {
		t.Fatalf("handleOpened alpha: %v", err)
	}

	// Route event from tracker B.
	err = d.handleOpened(context.Background(), forge.Event{
		Type: forge.EventIssueOpened, Owner: "org", Repo: "beta",
		IssueNumber: 1, Issue: trackerB.issues[1],
	})
	if err != nil {
		t.Fatalf("handleOpened beta: %v", err)
	}

	// Both issues #1 should be claimed -- no collision.
	if !hasLabel(trackerA.issues[1].Labels, "status:claimed") {
		t.Error("alpha issue #1 should be claimed")
	}
	if !hasLabel(trackerB.issues[1].Labels, "status:claimed") {
		t.Error("beta issue #1 should be claimed")
	}

	// Both should be in the claimed map with different keys.
	d.mu.Lock()
	_, aOk := d.claimed[issueKey("org", "alpha", 1)]
	_, bOk := d.claimed[issueKey("org", "beta", 1)]
	d.mu.Unlock()
	if !aOk {
		t.Error("alpha issue #1 not in claimed map")
	}
	if !bOk {
		t.Error("beta issue #1 not in claimed map")
	}
}

func TestMultiRepo_SameIssueNumber_NoCollision(t *testing.T) {
	trackerA := newMockTracker()
	trackerB := newMockTracker()

	cfg := DefaultConfig()
	d := New([]TrackerEntry{
		{Owner: "org", Repo: "alpha", Tracker: trackerA},
		{Owner: "org", Repo: "beta", Tracker: trackerB},
	}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	keyA := issueKey("org", "alpha", 42)
	keyB := issueKey("org", "beta", 42)

	if keyA == keyB {
		t.Fatal("keys should differ for same issue number in different repos")
	}

	// Set up claimed entries.
	d.mu.Lock()
	d.claimed[keyA] = &claimedIssue{AgentID: "code-gen", Owner: "org", Repo: "alpha", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.claimed[keyB] = &claimedIssue{AgentID: "docs", Owner: "org", Repo: "beta", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.mu.Unlock()

	// Delete alpha's entry -- beta should remain.
	d.mu.Lock()
	delete(d.claimed, keyA)
	_, bExists := d.claimed[keyB]
	d.mu.Unlock()

	if !bExists {
		t.Error("deleting alpha key should not affect beta key")
	}
}

func TestMultiRepo_HandleTaskComplete_ResolvesCorrectTracker(t *testing.T) {
	trackerA := newMockTracker()
	trackerB := newMockTracker()

	cfg := DefaultConfig()
	d := New([]TrackerEntry{
		{Owner: "org", Repo: "alpha", Tracker: trackerA},
		{Owner: "org", Repo: "beta", Tracker: trackerB},
	}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	// Set up issue in tracker B.
	trackerB.issues[5] = &forge.Issue{
		Number: 5, State: forge.StateOpen,
		Labels: []string{"status:claimed", "status:in-progress"},
	}

	// Claim it under beta.
	key := issueKey("org", "beta", 5)
	d.mu.Lock()
	d.claimed[key] = &claimedIssue{AgentID: "code-gen", Owner: "org", Repo: "beta", ClaimedAt: time.Now(), LastHeartbeat: time.Now()}
	d.mu.Unlock()

	d.handleTaskComplete(agent.TaskResult{
		Owner: "org", Repo: "beta",
		IssueNumber: 5, SessionID: "sess-beta-5",
		AgentType: models.AgentTypeCodeGen, Success: true,
	})

	// Verify beta's tracker got the label changes, not alpha's.
	if !hasLabel(trackerB.issues[5].Labels, "status:needs-qc") {
		t.Error("expected status:needs-qc on beta tracker issue #5")
	}

	// Alpha tracker should have no issues modified.
	trackerA.mu.Lock()
	aCalls := len(trackerA.calls)
	trackerA.mu.Unlock()
	if aCalls > 0 {
		t.Errorf("alpha tracker should not have been called, got %d calls", aCalls)
	}
}

func TestMultiRepo_TrackerFor_CaseInsensitive(t *testing.T) {
	tracker := newMockTracker()

	d := New([]TrackerEntry{
		{Owner: "MyOrg", Repo: "MyRepo", Tracker: tracker},
	}, &mockPolicy{costThreshold: 5.0}, nil, nil, nil, zap.NewNop())

	if got := d.trackerFor("myorg", "myrepo"); got != tracker {
		t.Error("trackerFor should be case-insensitive")
	}
	if got := d.trackerFor("MYORG", "MYREPO"); got != tracker {
		t.Error("trackerFor should be case-insensitive for uppercase")
	}
}

func TestMultiRepo_SingleTracker_BackwardCompat(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// With a single tracker, empty owner/repo should resolve to it.
	if got := d.trackerFor("", ""); got != tracker {
		t.Error("single tracker should be returned for empty owner/repo")
	}
}

func TestIssueKey_Format(t *testing.T) {
	tests := []struct {
		owner, repo string
		number      int
		want        string
	}{
		{"HerbHall", "Samverk", 42, "herbhall/samverk#42"},
		{"org", "repo", 1, "org/repo#1"},
		{"UPPER", "CASE", 99, "upper/case#99"},
	}
	for _, tt := range tests {
		got := issueKey(tt.owner, tt.repo, tt.number)
		if got != tt.want {
			t.Errorf("issueKey(%q, %q, %d) = %q, want %q", tt.owner, tt.repo, tt.number, got, tt.want)
		}
	}
}

func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// --- Watcher resilience tests (#577) ---

// failingTracker wraps mockTracker but makes Watch return an error a
// configurable number of times before blocking on ctx.Done().
type failingTracker struct {
	*mockTracker
	mu           sync.Mutex
	failCount    int // how many times Watch should fail before succeeding
	watchCalls   int
	failedCh     chan struct{} // closed after all failures are emitted
	succeededCtx context.Context
}

func newFailingTracker(failCount int) *failingTracker {
	return &failingTracker{
		mockTracker: newMockTracker(),
		failCount:   failCount,
		failedCh:    make(chan struct{}, failCount),
	}
}

func (f *failingTracker) Watch(ctx context.Context, handler func(forge.Event)) error {
	f.mu.Lock()
	call := f.watchCalls
	f.watchCalls++
	shouldFail := call < f.failCount
	f.mu.Unlock()

	if shouldFail {
		f.failedCh <- struct{}{}
		return fmt.Errorf("simulated watcher failure #%d", call+1)
	}
	f.succeededCtx = ctx
	<-ctx.Done()
	return ctx.Err()
}

func (f *failingTracker) getWatchCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.watchCalls
}

func TestRun_WatcherRestartsAfterError(t *testing.T) {
	ft := newFailingTracker(2) // fail twice, then succeed

	cfg := DefaultConfig()
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond

	d := New(
		[]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: ft}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	// Wait for the successful watch to start (call #3).
	deadline := time.After(4 * time.Second)
	for ft.getWatchCalls() < 3 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for watcher restart, got %d calls", ft.getWatchCalls())
		case err := <-errCh:
			t.Fatalf("Run exited unexpectedly: %v", err)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	err := <-errCh
	if err != nil && err != context.Canceled {
		t.Errorf("unexpected Run error: %v", err)
	}
}

func TestRun_ExitsAfterTooManyWatcherFailures(t *testing.T) {
	// Fail more than maxConsecutiveWatcherFailures times rapidly.
	ft := newFailingTracker(maxConsecutiveWatcherFailures + 1)

	cfg := DefaultConfig()
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond

	d := New(
		[]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: ft}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := d.Run(ctx)
	if err == nil {
		t.Fatal("expected Run to return error after too many failures")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRun_BackoffProgression(t *testing.T) {
	// Verify that backoff doubles: the second failure should wait longer
	// than the first. We test indirectly by measuring total time.
	ft := newFailingTracker(3) // 3 failures = backoffs of 1s, 2s, then succeed

	cfg := DefaultConfig()
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond

	d := New(
		[]TrackerEntry{{Owner: "test", Repo: "repo", Tracker: ft}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	// Wait for the 4th Watch call (the successful one).
	deadline := time.After(12 * time.Second)
	for ft.getWatchCalls() < 4 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for 4th watch call, got %d", ft.getWatchCalls())
		case err := <-errCh:
			t.Fatalf("Run exited unexpectedly: %v", err)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	elapsed := time.Since(start)
	// 3 failures with backoffs 1s + 2s + 4s = 7s minimum.
	// Allow some slack but verify it's not instant.
	if elapsed < 3*time.Second {
		t.Errorf("expected at least 3s of backoff, got %v", elapsed)
	}

	cancel()
	<-errCh
}
