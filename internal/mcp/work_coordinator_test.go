package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/herbhall/samverk/internal/forge"
	internalmcp "github.com/herbhall/samverk/internal/mcp"
)

// newTestRegistry creates a project registry with a mock tracker for testing.
func newTestRegistry(t *testing.T, tracker forge.IssueTracker) *internalmcp.ProjectRegistry {
	t.Helper()
	reg := internalmcp.NewProjectRegistry()
	if err := reg.Register(&internalmcp.Project{
		Name:    "testproject",
		Owner:   "testowner",
		Repo:    "testrepo",
		Phase:   "development",
		Tracker: tracker,
	}); err != nil {
		t.Fatalf("register project: %v", err)
	}
	return reg
}

func TestClaimIssue(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 42, Title: "test issue", Labels: []string{"status:queued"}, State: forge.StateOpen},
		},
	}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	result, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test")
	if err != nil {
		t.Fatalf("ClaimIssue: %v", err)
	}

	if result.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", result.IssueNumber)
	}
	if result.WorkerID != "interactive:test" {
		t.Errorf("WorkerID = %q, want %q", result.WorkerID, "interactive:test")
	}

	// Verify labels were transitioned.
	if len(tracker.removeLabelCalls) == 0 {
		t.Error("expected RemoveLabel call for status:queued")
	}
	foundRemove := false
	for _, c := range tracker.removeLabelCalls {
		if c.Number == 42 && c.Label == "status:queued" {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Error("expected RemoveLabel(42, status:queued)")
	}

	foundAdd := false
	for _, c := range tracker.addLabelCalls {
		if c.Number == 42 && c.Label == "status:claimed" {
			foundAdd = true
		}
	}
	if !foundAdd {
		t.Error("expected AddLabel(42, status:claimed)")
	}

	// Verify comment was posted.
	if len(tracker.addCommentCalls) == 0 {
		t.Error("expected claim comment")
	} else if !strings.Contains(tracker.addCommentCalls[0].Body, "CLAIMED") {
		t.Errorf("claim comment should contain CLAIMED, got: %s", tracker.addCommentCalls[0].Body)
	}
}

func TestClaimIssue_AlreadyClaimed(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	// Claim once.
	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "worker-1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// Claim again -- should fail.
	_, err = wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "worker-2")
	if err == nil {
		t.Fatal("expected error for double claim")
	}
	if !strings.Contains(err.Error(), "already claimed") {
		t.Errorf("error should mention 'already claimed', got: %v", err)
	}
}

func TestCompleteIssue(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	// Claim first.
	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Complete.
	err = wc.CompleteIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test", 100, "Fixed the bug")
	if err != nil {
		t.Fatalf("CompleteIssue: %v", err)
	}

	// Verify needs-qc label was added.
	foundQC := false
	for _, c := range tracker.addLabelCalls {
		if c.Number == 42 && c.Label == "status:needs-qc" {
			foundQC = true
		}
	}
	if !foundQC {
		t.Error("expected AddLabel(42, status:needs-qc)")
	}

	// Verify completion comment contains PR and summary.
	foundComplete := false
	for _, c := range tracker.addCommentCalls {
		if strings.Contains(c.Body, "COMPLETE") && strings.Contains(c.Body, "PR: #100") && strings.Contains(c.Body, "Fixed the bug") {
			foundComplete = true
		}
	}
	if !foundComplete {
		t.Error("expected completion comment with PR and summary")
	}

	// Verify claim is released.
	info := wc.GetClaimInfo("testowner", "testrepo", 42)
	if info != nil {
		t.Error("expected claim to be released after completion")
	}
}

func TestCompleteIssue_WrongWorker(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "worker-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	err = wc.CompleteIssue(context.Background(), "testowner", "testrepo", 42, "worker-2", 0, "")
	if err == nil {
		t.Fatal("expected error for wrong worker")
	}
	if !strings.Contains(err.Error(), "claimed by worker-1") {
		t.Errorf("error should mention current holder, got: %v", err)
	}
}

func TestReleaseIssue(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	err = wc.ReleaseIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test", "wrong approach")
	if err != nil {
		t.Fatalf("ReleaseIssue: %v", err)
	}

	// Verify queued label restored.
	foundQueued := false
	for _, c := range tracker.addLabelCalls {
		if c.Number == 42 && c.Label == "status:queued" {
			foundQueued = true
		}
	}
	if !foundQueued {
		t.Error("expected AddLabel(42, status:queued)")
	}

	// Verify release comment.
	foundRelease := false
	for _, c := range tracker.addCommentCalls {
		if strings.Contains(c.Body, "RELEASE") && strings.Contains(c.Body, "wrong approach") {
			foundRelease = true
		}
	}
	if !foundRelease {
		t.Error("expected release comment with reason")
	}

	// Verify claim is cleared.
	info := wc.GetClaimInfo("testowner", "testrepo", 42)
	if info != nil {
		t.Error("expected claim to be cleared after release")
	}
}

func TestReleaseIssue_NotClaimed(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	err := wc.ReleaseIssue(context.Background(), "testowner", "testrepo", 42, "worker", "test")
	if err == nil {
		t.Fatal("expected error for releasing unclaimed issue")
	}
	if !strings.Contains(err.Error(), "not claimed") {
		t.Errorf("error should mention 'not claimed', got: %v", err)
	}
}

func TestHeartbeatIssue(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	result, err := wc.HeartbeatIssue(context.Background(), "testowner", "testrepo", 42, "interactive:test")
	if err != nil {
		t.Fatalf("HeartbeatIssue: %v", err)
	}

	if result.Status != "active" {
		t.Errorf("Status = %q, want %q", result.Status, "active")
	}
	if result.ClaimDuration == "" {
		t.Error("ClaimDuration should not be empty")
	}
}

func TestHeartbeatIssue_WrongWorker(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "worker-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	_, err = wc.HeartbeatIssue(context.Background(), "testowner", "testrepo", 42, "worker-2")
	if err == nil {
		t.Fatal("expected error for wrong worker")
	}
}

func TestRequestWork(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "first", Labels: []string{"status:queued"}, State: forge.StateOpen},
			{Number: 20, Title: "second", Labels: []string{"status:queued"}, State: forge.StateOpen},
		},
	}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	claim, issue, err := wc.RequestWork(context.Background(), "interactive:test", nil)
	if err != nil {
		t.Fatalf("RequestWork: %v", err)
	}

	if claim.IssueNumber != 10 {
		t.Errorf("claimed issue = %d, want 10 (first queued)", claim.IssueNumber)
	}
	if issue.Number != 10 {
		t.Errorf("returned issue = %d, want 10", issue.Number)
	}
}

func TestRequestWork_SkipsInactiveProjects(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "queued", Labels: []string{"status:queued"}, State: forge.StateOpen},
		},
	}
	reg := internalmcp.NewProjectRegistry()
	if err := reg.Register(&internalmcp.Project{
		Name:    "archived",
		Owner:   "testowner",
		Repo:    "testrepo",
		Phase:   "inactive",
		Tracker: tracker,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, _, err := wc.RequestWork(context.Background(), "interactive:test", nil)
	if err == nil {
		t.Fatal("expected error -- inactive projects should be skipped")
	}
}

func TestRequestWork_FilterByProject(t *testing.T) {
	tracker1 := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "proj1", Labels: []string{"status:queued"}, State: forge.StateOpen},
		},
	}
	tracker2 := &mockTracker{
		issues: []*forge.Issue{
			{Number: 20, Title: "proj2", Labels: []string{"status:queued"}, State: forge.StateOpen},
		},
	}
	reg := internalmcp.NewProjectRegistry()
	_ = reg.Register(&internalmcp.Project{Name: "proj1", Owner: "o1", Repo: "r1", Phase: "development", Tracker: tracker1})
	_ = reg.Register(&internalmcp.Project{Name: "proj2", Owner: "o2", Repo: "r2", Phase: "development", Tracker: tracker2})

	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	claim, _, err := wc.RequestWork(context.Background(), "worker", &internalmcp.WorkRequestOptions{
		Project: "proj2",
	})
	if err != nil {
		t.Fatalf("RequestWork: %v", err)
	}
	if claim.IssueNumber != 20 {
		t.Errorf("expected issue 20 from proj2, got %d", claim.IssueNumber)
	}
}

func TestRequestWork_NoQueuedIssues(t *testing.T) {
	tracker := &mockTracker{
		issues: []*forge.Issue{
			{Number: 10, Title: "in progress", Labels: []string{"status:in-progress"}, State: forge.StateOpen},
		},
	}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	_, _, err := wc.RequestWork(context.Background(), "worker", nil)
	if err == nil {
		t.Fatal("expected error when no queued issues")
	}
}

func TestGetClaimInfo(t *testing.T) {
	tracker := &mockTracker{}
	reg := newTestRegistry(t, tracker)
	wc := internalmcp.NewForgeWorkCoordinator(reg, nil)

	// No claim yet.
	info := wc.GetClaimInfo("testowner", "testrepo", 42)
	if info != nil {
		t.Error("expected nil for unclaimed issue")
	}

	// Claim.
	_, err := wc.ClaimIssue(context.Background(), "testowner", "testrepo", 42, "test-worker")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	info = wc.GetClaimInfo("testowner", "testrepo", 42)
	if info == nil {
		t.Fatal("expected claim info")
	}
	if info.WorkerID != "test-worker" {
		t.Errorf("WorkerID = %q, want %q", info.WorkerID, "test-worker")
	}
}
