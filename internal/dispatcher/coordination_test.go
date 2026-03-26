package dispatcher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// errorTracker wraps mockTracker and makes GetIssue return an error for
// specific issue numbers, simulating an unreachable forge.
type errorTracker struct {
	*mockTracker
	mu           sync.Mutex
	failNumbers  map[int]error
	failGetIssue bool // when true, ALL GetIssue calls fail
	failErr      error
}

func newErrorTracker() *errorTracker {
	return &errorTracker{
		mockTracker: newMockTracker(),
		failNumbers: make(map[int]error),
	}
}

func (e *errorTracker) GetIssue(ctx context.Context, number int) (*forge.Issue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failGetIssue {
		return nil, e.failErr
	}
	if err, ok := e.failNumbers[number]; ok {
		return nil, err
	}
	e.mu.Unlock()
	result, err := e.mockTracker.GetIssue(ctx, number)
	e.mu.Lock()
	return result, err
}

// --- Integration tests for cross-agent coordination (#435) ---

// TestCoordination_CrossProjectDependencyBlocking verifies that an issue
// with depends_on referencing another repo's issue stays blocked until
// the dependency closes.
func TestCoordination_CrossProjectDependencyBlocking(t *testing.T) {
	tests := []struct {
		name        string
		crossState  forge.State
		crossLabels []string
		wantBlocked bool
	}{
		{
			name:        "blocks when cross-project dep is open",
			crossState:  forge.StateOpen,
			crossLabels: []string{models.LabelStatusInProgress},
			wantBlocked: true,
		},
		{
			name:        "blocks when cross-project dep is closed without done label",
			crossState:  forge.StateClosed,
			crossLabels: []string{models.LabelStatusNeedsHuman},
			wantBlocked: true,
		},
		{
			name:        "unblocked when cross-project dep is closed with done label",
			crossState:  forge.StateClosed,
			crossLabels: []string{models.LabelStatusDone},
			wantBlocked: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			localTracker := newMockTracker()
			d := newTestDispatcher(localTracker)

			crossTracker := newMockTracker()
			closedAt := time.Now()
			crossIssue := &forge.Issue{
				Number:   10,
				State:    tc.crossState,
				Labels:   tc.crossLabels,
				ClosedAt: &closedAt,
			}
			crossTracker.issues[10] = crossIssue

			resolver := newMockProjectResolver()
			resolver.addProject("org", "other-repo", crossTracker)
			d.SetProjectResolver(resolver)

			fm := &models.IssueFrontmatter{
				DependsOn: models.DependencyList{
					{Owner: "org", Repo: "other-repo", Number: 10},
				},
			}

			blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if blocked != tc.wantBlocked {
				t.Errorf("blocked = %v, want %v", blocked, tc.wantBlocked)
			}
			if tc.wantBlocked && len(blockers) == 0 {
				t.Error("expected non-empty blockers when blocked")
			}
			if !tc.wantBlocked && len(blockers) != 0 {
				t.Errorf("expected empty blockers when not blocked, got %v", blockers)
			}
		})
	}
}

// TestCoordination_UnblockingViaClose verifies the full unblocking lifecycle:
// a blocked issue transitions to queued when its dependency is closed.
func TestCoordination_UnblockingViaClose(t *testing.T) {
	localTracker := newMockTracker()
	d := newTestDispatcher(localTracker)

	// Create the dependency issue (open).
	localTracker.issues[10] = &forge.Issue{
		Number: 10,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	// Create the dependent issue (blocked on #10).
	localTracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Depends on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Verify it's blocked initially.
	if !hasLabel(localTracker.issues[20].Labels, models.LabelStatusBlocked) {
		t.Fatal("precondition: issue #20 should be blocked")
	}

	// Close the dependency.
	closedAt := time.Now()
	localTracker.issues[10].State = forge.StateClosed
	localTracker.issues[10].Labels = []string{models.LabelStatusDone}
	localTracker.issues[10].ClosedAt = &closedAt

	// Trigger unblock via handleClosed event.
	err := d.handleClosed(context.Background(), forge.Event{
		Type:        forge.EventIssueClosed,
		IssueNumber: 10,
		Owner:       "test",
		Repo:        "repo",
	})
	if err != nil {
		t.Fatalf("handleClosed error: %v", err)
	}

	// Verify #20 is unblocked.
	issue20 := localTracker.issues[20]
	if hasLabel(issue20.Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked to be removed from #20")
	}
	if !hasLabel(issue20.Labels, models.LabelStatusQueued) {
		t.Error("expected status:queued on #20 after unblocking")
	}
}

// TestCoordination_CrossProjectUnblockingViaRecheck verifies that
// recheckCrossProjectDeps unblocks issues when a cross-project
// dependency becomes done.
func TestCoordination_CrossProjectUnblockingViaRecheck(t *testing.T) {
	localTracker := newMockTracker()

	cfg := DefaultConfig()
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond
	d := New(
		[]TrackerEntry{{Owner: "org", Repo: "alpha", Tracker: localTracker}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	// Blocked issue depending on cross-project dep.
	localTracker.issues[5] = &forge.Issue{
		Number: 5,
		Title:  "Depends on beta#20",
		Body:   issueBodyMixed("code-gen", nil, []string{"org/beta#20"}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Cross-project dep starts open.
	crossTracker := newMockTracker()
	crossTracker.issues[20] = &forge.Issue{
		Number: 20,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("org", "beta", crossTracker)
	d.SetProjectResolver(resolver)

	// First recheck: should stay blocked.
	if err := d.recheckCrossProjectDeps(context.Background()); err != nil {
		t.Fatalf("recheck error: %v", err)
	}
	if !hasLabel(localTracker.issues[5].Labels, models.LabelStatusBlocked) {
		t.Error("expected issue to stay blocked while cross-project dep is open")
	}

	// Now close the cross-project dep.
	closedAt := time.Now()
	crossTracker.issues[20].State = forge.StateClosed
	crossTracker.issues[20].Labels = []string{models.LabelStatusDone}
	crossTracker.issues[20].ClosedAt = &closedAt

	// Second recheck: should unblock.
	if err := d.recheckCrossProjectDeps(context.Background()); err != nil {
		t.Fatalf("recheck error: %v", err)
	}

	issue5 := localTracker.issues[5]
	if hasLabel(issue5.Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked to be removed after cross-project dep done")
	}
	if !hasLabel(issue5.Labels, models.LabelStatusQueued) {
		t.Error("expected status:queued after cross-project unblock")
	}
}

// TestCoordination_CycleDetection_TwoNodes verifies that a simple A<->B
// cycle is detected and escalated.
func TestCoordination_CycleDetection_TwoNodes(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Issue #1 depends on #2, issue #2 depends on #1.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{2}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{1}),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected a cycle to be detected")
	}

	// Cycle should contain both issues.
	has1, has2 := false, false
	for _, n := range cycle {
		if n == 1 {
			has1 = true
		}
		if n == 2 {
			has2 = true
		}
	}
	if !has1 || !has2 {
		t.Errorf("cycle = %v, expected to contain both 1 and 2", cycle)
	}
}

// TestCoordination_CycleDetection_ThreeNodes verifies A->B->C->A cycle.
func TestCoordination_CycleDetection_ThreeNodes(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{2}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{3}),
	}
	tracker.issues[3] = &forge.Issue{
		Number: 3,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{1}),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected a 3-node cycle to be detected")
	}
	if len(cycle) < 3 {
		t.Errorf("cycle length = %d, expected at least 3 nodes", len(cycle))
	}
}

// TestCoordination_CycleDetection_NoCycle verifies linear chains are not
// reported as cycles.
func TestCoordination_CycleDetection_NoCycle(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Linear: #1 -> #2 -> #3 (no cycle).
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{2}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{3}),
	}
	tracker.issues[3] = &forge.Issue{
		Number: 3,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", nil),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle != nil {
		t.Errorf("expected no cycle, got %v", cycle)
	}
}

// TestCoordination_CycleEscalation verifies that detected cycles
// produce needs-human labels and escalation comments on all cycle members.
func TestCoordination_CycleEscalation(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusQueued},
		Body:   issueBody("code-gen", []int{2}),
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusQueued},
		Body:   issueBody("code-gen", []int{1}),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("detectCycle error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected cycle")
	}

	// Escalate the cycle.
	err = d.escalateCycle(context.Background(), "test", "repo", cycle)
	if err != nil {
		t.Fatalf("escalateCycle error: %v", err)
	}

	// Both issues should have needs-human label and an escalation comment.
	for _, num := range []int{1, 2} {
		issue := tracker.issues[num]
		if !hasLabel(issue.Labels, models.LabelStatusNeedsHuman) {
			t.Errorf("issue #%d: expected status:needs-human label", num)
		}

		comments := tracker.comments[num]
		if len(comments) == 0 {
			t.Errorf("issue #%d: expected escalation comment", num)
			continue
		}
		found := false
		for _, c := range comments {
			if strings.Contains(c.Body, "dependency_cycle") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("issue #%d: expected comment mentioning dependency_cycle", num)
		}
	}
}

// TestCoordination_MixedDependencies_ResolveCorrectly verifies that an
// issue with both same-project and cross-project deps correctly
// evaluates each.
func TestCoordination_MixedDependencies_ResolveCorrectly(t *testing.T) {
	tests := []struct {
		name            string
		localDone       bool
		crossDone       bool
		wantBlocked     bool
		wantBlockerList []string
	}{
		{
			name:            "both done, not blocked",
			localDone:       true,
			crossDone:       true,
			wantBlocked:     false,
			wantBlockerList: nil,
		},
		{
			name:            "local done, cross open, blocked on cross",
			localDone:       true,
			crossDone:       false,
			wantBlocked:     true,
			wantBlockerList: []string{"org/other#50"},
		},
		{
			name:            "local open, cross done, blocked on local",
			localDone:       false,
			crossDone:       true,
			wantBlocked:     true,
			wantBlockerList: []string{"42"},
		},
		{
			name:        "both open, blocked on both",
			localDone:   false,
			crossDone:   false,
			wantBlocked: true,
			wantBlockerList: []string{"42", "org/other#50"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			localTracker := newMockTracker()
			d := newTestDispatcher(localTracker)

			closedAt := time.Now()
			localIssue := &forge.Issue{Number: 42}
			if tc.localDone {
				localIssue.State = forge.StateClosed
				localIssue.Labels = []string{models.LabelStatusDone}
				localIssue.ClosedAt = &closedAt
			} else {
				localIssue.State = forge.StateOpen
				localIssue.Labels = []string{models.LabelStatusInProgress}
			}
			localTracker.issues[42] = localIssue

			crossTracker := newMockTracker()
			crossIssue := &forge.Issue{Number: 50}
			if tc.crossDone {
				crossIssue.State = forge.StateClosed
				crossIssue.Labels = []string{models.LabelStatusDone}
				crossIssue.ClosedAt = &closedAt
			} else {
				crossIssue.State = forge.StateOpen
				crossIssue.Labels = []string{models.LabelStatusQueued}
			}
			crossTracker.issues[50] = crossIssue

			resolver := newMockProjectResolver()
			resolver.addProject("org", "other", crossTracker)
			d.SetProjectResolver(resolver)

			fm := &models.IssueFrontmatter{
				DependsOn: models.DependencyList{
					{Number: 42},
					{Owner: "org", Repo: "other", Number: 50},
				},
			}

			blocked, blockers, err := d.checkDependencies(context.Background(), "test", "repo", fm)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if blocked != tc.wantBlocked {
				t.Errorf("blocked = %v, want %v", blocked, tc.wantBlocked)
			}

			if tc.wantBlockerList == nil {
				if len(blockers) != 0 {
					t.Errorf("blockers = %v, want empty", blockers)
				}
			} else {
				if len(blockers) != len(tc.wantBlockerList) {
					t.Errorf("blockers = %v, want %v", blockers, tc.wantBlockerList)
				} else {
					for i, want := range tc.wantBlockerList {
						if blockers[i] != want {
							t.Errorf("blockers[%d] = %q, want %q", i, blockers[i], want)
						}
					}
				}
			}
		})
	}
}

// TestCoordination_GracefulDegradation_UnreachableForge verifies that
// when a cross-project forge is unreachable, checkDependencies returns
// an error (not a panic).
func TestCoordination_GracefulDegradation_UnreachableForge(t *testing.T) {
	localTracker := newMockTracker()
	d := newTestDispatcher(localTracker)

	errTracker := newErrorTracker()
	errTracker.failGetIssue = true
	errTracker.failErr = errors.New("connection refused: forge unreachable")

	resolver := newMockProjectResolver()
	resolver.addProject("remote", "service", errTracker)
	d.SetProjectResolver(resolver)

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "remote", Repo: "service", Number: 1},
		},
	}

	_, _, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err == nil {
		t.Fatal("expected error when forge is unreachable")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want to contain 'connection refused'", err.Error())
	}
}

// TestCoordination_GracefulDegradation_UnregisteredProject verifies that
// referencing an unregistered project returns an error, not a crash.
func TestCoordination_GracefulDegradation_UnregisteredProject(t *testing.T) {
	localTracker := newMockTracker()
	d := newTestDispatcher(localTracker)

	resolver := newMockProjectResolver()
	d.SetProjectResolver(resolver)

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "ghost", Repo: "phantom", Number: 999},
		},
	}

	_, _, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err == nil {
		t.Fatal("expected error for unregistered project")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error = %q, want to contain 'not registered'", err.Error())
	}
}

// TestCoordination_GracefulDegradation_NoProjectResolver verifies that
// cross-project deps without a configured resolver return an error.
func TestCoordination_GracefulDegradation_NoProjectResolver(t *testing.T) {
	localTracker := newMockTracker()
	d := newTestDispatcher(localTracker)
	// d.projects is nil -- no resolver configured.

	fm := &models.IssueFrontmatter{
		DependsOn: models.DependencyList{
			{Owner: "org", Repo: "other", Number: 1},
		},
	}

	_, _, err := d.checkDependencies(context.Background(), "test", "repo", fm)
	if err == nil {
		t.Fatal("expected error with no project resolver")
	}
	if !strings.Contains(err.Error(), "no project resolver") {
		t.Errorf("error = %q, want to contain 'no project resolver'", err.Error())
	}
}

// TestCoordination_MultiRepoUnblocking verifies that closing an issue in
// repo A unblocks a dependent in repo B (cross-tracker unblock scan).
func TestCoordination_MultiRepoUnblocking(t *testing.T) {
	trackerA := newMockTracker()
	trackerB := newMockTracker()

	cfg := DefaultConfig()
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond
	d := New(
		[]TrackerEntry{
			{Owner: "org", Repo: "alpha", Tracker: trackerA},
			{Owner: "org", Repo: "beta", Tracker: trackerB},
		},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	// Repo alpha has issue #10 (open).
	trackerA.issues[10] = &forge.Issue{
		Number: 10,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	// Repo beta has issue #5 that depends on alpha#10 (local dep reference
	// since unblockDependents only checks same-number local deps).
	// We use issue number 10 in beta's dep list.
	trackerB.issues[5] = &forge.Issue{
		Number: 5,
		Title:  "Depends on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}
	// Repo beta also needs issue #10 to exist for GetIssue call during unblock check.
	closedAt := time.Now()
	trackerB.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}

	// Close issue #10 -- triggers unblockDependents across ALL trackers.
	err := d.unblockDependents(context.Background(), 10)
	if err != nil {
		t.Fatalf("unblockDependents error: %v", err)
	}

	// Verify beta's issue #5 is unblocked.
	issue5 := trackerB.issues[5]
	if hasLabel(issue5.Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked removed from beta#5")
	}
	if !hasLabel(issue5.Labels, models.LabelStatusQueued) {
		t.Error("expected status:queued on beta#5 after unblocking")
	}
}

// TestCoordination_HandleOpenedWithCycle verifies that handleOpened detects
// a cycle and escalates instead of routing.
func TestCoordination_HandleOpenedWithCycle(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Create a cycle: #1 -> #2 -> #1.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Task A",
		Body:   issueBody("code-gen", []int{2}),
		State:  forge.StateOpen,
		Labels: []string{},
	}
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		Title:  "Task B",
		Body:   issueBody("code-gen", []int{1}),
		State:  forge.StateOpen,
		Labels: []string{},
	}

	// Open issue #1 -- cycle detection should fire.
	err := d.handleOpened(context.Background(), forge.Event{
		Type:        forge.EventIssueOpened,
		IssueNumber: 1,
		Issue:       tracker.issues[1],
		Owner:       "test",
		Repo:        "repo",
	})
	if err != nil {
		t.Fatalf("handleOpened error: %v", err)
	}

	// Both issues should be escalated with needs-human.
	for _, num := range []int{1, 2} {
		if !hasLabel(tracker.issues[num].Labels, models.LabelStatusNeedsHuman) {
			t.Errorf("issue #%d: expected status:needs-human after cycle detection", num)
		}
	}
}

// TestCoordination_PartialUnblock_StillBlocked verifies that closing one
// of multiple dependencies does NOT unblock the issue.
func TestCoordination_PartialUnblock_StillBlocked(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	closedAt := time.Now()

	// Dep #10 is done.
	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}
	// Dep #11 is still open.
	tracker.issues[11] = &forge.Issue{
		Number: 11,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	// Issue #20 depends on both #10 and #11.
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Depends on #10 and #11",
		Body:   issueBody("code-gen", []int{10, 11}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Close #10 -- should NOT unblock #20 because #11 is still open.
	err := d.handleClosed(context.Background(), forge.Event{
		Type:        forge.EventIssueClosed,
		IssueNumber: 10,
		Owner:       "test",
		Repo:        "repo",
	})
	if err != nil {
		t.Fatalf("handleClosed error: %v", err)
	}

	issue20 := tracker.issues[20]
	if !hasLabel(issue20.Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked to remain (one dep still open)")
	}
	if hasLabel(issue20.Labels, models.LabelStatusQueued) {
		t.Error("did not expect status:queued (partial unblock)")
	}
}

// TestCoordination_FullUnblock_AllDepsResolved verifies that closing the
// last remaining dependency unblocks the issue.
func TestCoordination_FullUnblock_AllDepsResolved(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	closedAt := time.Now()

	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}
	tracker.issues[11] = &forge.Issue{
		Number:   11,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}

	// Issue #20 depends on both #10 and #11 -- both are now done.
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Depends on #10 and #11",
		Body:   issueBody("code-gen", []int{10, 11}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Close #11 (last dep) -- should unblock #20.
	err := d.handleClosed(context.Background(), forge.Event{
		Type:        forge.EventIssueClosed,
		IssueNumber: 11,
		Owner:       "test",
		Repo:        "repo",
	})
	if err != nil {
		t.Fatalf("handleClosed error: %v", err)
	}

	issue20 := tracker.issues[20]
	if hasLabel(issue20.Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked to be removed")
	}
	if !hasLabel(issue20.Labels, models.LabelStatusQueued) {
		t.Error("expected status:queued after full unblock")
	}
}

// TestCoordination_RecheckCrossProjectDeps_MultipleBlockedIssues verifies
// that recheckCrossProjectDeps processes multiple blocked issues in the
// same tracker, unblocking only those whose deps are satisfied.
func TestCoordination_RecheckCrossProjectDeps_MultipleBlockedIssues(t *testing.T) {
	tracker := newMockTracker()
	cfg := DefaultConfig()
	d := New(
		[]TrackerEntry{{Owner: "org", Repo: "main", Tracker: tracker}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	closedAt := time.Now()

	// Issue #1: blocked on org/lib#10 (which is done).
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on lib#10",
		Body:   issueBodyMixed("code-gen", nil, []string{"org/lib#10"}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Issue #2: blocked on org/lib#20 (which is NOT done).
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		Title:  "Blocked on lib#20",
		Body:   issueBodyMixed("code-gen", nil, []string{"org/lib#20"}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	crossTracker := newMockTracker()
	crossTracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}
	crossTracker.issues[20] = &forge.Issue{
		Number: 20,
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	resolver := newMockProjectResolver()
	resolver.addProject("org", "lib", crossTracker)
	d.SetProjectResolver(resolver)

	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("recheck error: %v", err)
	}

	// Issue #1 should be unblocked.
	if hasLabel(tracker.issues[1].Labels, models.LabelStatusBlocked) {
		t.Error("issue #1: expected status:blocked removed")
	}
	if !hasLabel(tracker.issues[1].Labels, models.LabelStatusQueued) {
		t.Error("issue #1: expected status:queued")
	}

	// Issue #2 should remain blocked.
	if !hasLabel(tracker.issues[2].Labels, models.LabelStatusBlocked) {
		t.Error("issue #2: expected status:blocked to remain")
	}
	if hasLabel(tracker.issues[2].Labels, models.LabelStatusQueued) {
		t.Error("issue #2: did not expect status:queued")
	}
}

// TestCoordination_GracefulDegradation_RecheckWithUnreachableForge verifies
// that recheckCrossProjectDeps logs a warning and continues when a
// cross-project forge is unreachable (no crash, other issues still processed).
func TestCoordination_GracefulDegradation_RecheckWithUnreachableForge(t *testing.T) {
	tracker := newMockTracker()
	cfg := DefaultConfig()
	d := New(
		[]TrackerEntry{{Owner: "org", Repo: "main", Tracker: tracker}},
		&mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop(),
	)

	// Issue blocked on an unreachable forge.
	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Blocked on unreachable dep",
		Body:   issueBodyMixed("code-gen", nil, []string{"dead/forge#1"}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	// Issue blocked on a reachable forge (done).
	tracker.issues[2] = &forge.Issue{
		Number: 2,
		Title:  "Blocked on reachable dep",
		Body:   issueBodyMixed("code-gen", nil, []string{"org/lib#10"}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	errTracker := newErrorTracker()
	errTracker.failGetIssue = true
	errTracker.failErr = errors.New("timeout: forge unreachable")

	closedAt := time.Now()
	goodTracker := newMockTracker()
	goodTracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}

	resolver := newMockProjectResolver()
	resolver.addProject("dead", "forge", errTracker)
	resolver.addProject("org", "lib", goodTracker)
	d.SetProjectResolver(resolver)

	// Should not panic or return an error -- logs warning and continues.
	err := d.recheckCrossProjectDeps(context.Background())
	if err != nil {
		t.Fatalf("recheck should not return error, got: %v", err)
	}

	// Issue #1 should remain blocked (unreachable dep).
	if !hasLabel(tracker.issues[1].Labels, models.LabelStatusBlocked) {
		t.Error("issue #1: expected to stay blocked (unreachable forge)")
	}

	// Issue #2 should be unblocked (reachable dep is done).
	if hasLabel(tracker.issues[2].Labels, models.LabelStatusBlocked) {
		t.Error("issue #2: expected status:blocked removed")
	}
	if !hasLabel(tracker.issues[2].Labels, models.LabelStatusQueued) {
		t.Error("issue #2: expected status:queued")
	}
}

// TestCoordination_SelfDependency verifies that an issue depending on itself
// is detected as a cycle.
func TestCoordination_SelfDependency(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		State:  forge.StateOpen,
		Body:   issueBody("code-gen", []int{1}),
	}

	cycle, err := d.detectCycle(context.Background(), "test", "repo", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected self-cycle to be detected")
	}
}

// TestCoordination_DependencyParsing_CrossRef verifies that cross-project
// references in frontmatter are parsed correctly via the YAML unmarshaler.
func TestCoordination_DependencyParsing_CrossRef(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		want    models.DependencyList
	}{
		{
			name: "mixed local and cross-project deps",
			body: issueBodyMixed("code-gen", []int{10}, []string{"org/repo#42"}),
			want: models.DependencyList{
				{Number: 10},
				{Owner: "org", Repo: "repo", Number: 42},
			},
		},
		{
			name: "only cross-project deps",
			body: issueBodyMixed("code-gen", nil, []string{"a/b#1", "c/d#2"}),
			want: models.DependencyList{
				{Owner: "a", Repo: "b", Number: 1},
				{Owner: "c", Repo: "d", Number: 2},
			},
		},
		{
			name: "only local deps",
			body: issueBody("code-gen", []int{1, 2, 3}),
			want: models.DependencyList{
				{Number: 1},
				{Number: 2},
				{Number: 3},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := models.ParseFrontmatter(tc.body)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Frontmatter == nil {
				t.Fatal("expected non-nil frontmatter")
			}

			got := result.Frontmatter.DependsOn
			if len(got) != len(tc.want) {
				t.Fatalf("deps length = %d, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("dep[%d] = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCoordination_UnblockComment_ContainsClosedIssue verifies that
// the unblock comment references the closed dependency number.
func TestCoordination_UnblockComment_ContainsClosedIssue(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	closedAt := time.Now()
	tracker.issues[10] = &forge.Issue{
		Number:   10,
		State:    forge.StateClosed,
		Labels:   []string{models.LabelStatusDone},
		ClosedAt: &closedAt,
	}
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Depends on #10",
		Body:   issueBody("code-gen", []int{10}),
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusBlocked},
	}

	err := d.unblockDependents(context.Background(), 10)
	if err != nil {
		t.Fatalf("unblockDependents error: %v", err)
	}

	comments := tracker.comments[20]
	if len(comments) == 0 {
		t.Fatal("expected unblock comment on issue #20")
	}

	found := false
	for _, c := range comments {
		if strings.Contains(c.Body, "#10") && strings.Contains(c.Body, "UNBLOCKED") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unblock comment referencing #10")
	}
}

// TestCoordination_BlockComment_ContainsBlockers verifies that the block
// comment lists the unsatisfied dependencies.
func TestCoordination_BlockComment_ContainsBlockers(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	tracker.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Will be blocked",
		State:  forge.StateOpen,
		Labels: []string{},
	}

	blockers := []string{"42", "org/other#50"}
	err := d.blockIssue(context.Background(), "test", "repo", 1, blockers)
	if err != nil {
		t.Fatalf("blockIssue error: %v", err)
	}

	if !hasLabel(tracker.issues[1].Labels, models.LabelStatusBlocked) {
		t.Error("expected status:blocked label")
	}

	comments := tracker.comments[1]
	if len(comments) == 0 {
		t.Fatal("expected block comment")
	}

	body := comments[0].Body
	for _, b := range blockers {
		if !strings.Contains(body, b) {
			t.Errorf("block comment missing blocker %q", b)
		}
	}
	if !strings.Contains(body, "BLOCKED") {
		t.Error("expected BLOCKED prefix in comment")
	}
}

