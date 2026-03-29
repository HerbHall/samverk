package dispatcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

func TestRecoverOrphanedIssues_RequeuesClaimedAndInProgress(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Seed two orphaned issues: one claimed, one in-progress.
	tracker.issues[10] = &forge.Issue{
		Number: 10,
		Title:  "Orphan claimed",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusClaimed},
	}
	tracker.issues[11] = &forge.Issue{
		Number: 11,
		Title:  "Orphan in-progress",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	d.RecoverOrphanedIssues(context.Background())

	// Both issues should now have status:queued and no claimed/in-progress.
	for _, num := range []int{10, 11} {
		issue := tracker.issues[num]
		if hasLabel(issue.Labels, models.LabelStatusClaimed) {
			t.Errorf("issue #%d should not have status:claimed after recovery", num)
		}
		if hasLabel(issue.Labels, models.LabelStatusInProgress) {
			t.Errorf("issue #%d should not have status:in-progress after recovery", num)
		}
		if !hasLabel(issue.Labels, models.LabelStatusQueued) {
			t.Errorf("issue #%d should have status:queued after recovery", num)
		}

		// Check RECOVER comment was posted.
		comments := tracker.comments[num]
		if len(comments) == 0 {
			t.Fatalf("issue #%d should have a RECOVER comment", num)
		}
		if !strings.Contains(comments[0].Body, "RECOVER [dispatcher]") {
			t.Errorf("issue #%d comment should contain RECOVER header, got: %s", num, comments[0].Body)
		}
	}
}

func TestRecoverOrphanedIssues_SkipsActivelyClaimed(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Seed an issue that is claimed AND in the live claimed map.
	tracker.issues[20] = &forge.Issue{
		Number: 20,
		Title:  "Actively claimed",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusClaimed},
	}

	key := issueKey("test", "repo", 20)
	d.claimed[key] = &claimedIssue{
		AgentID:       "code-gen",
		Owner:         "test",
		Repo:          "repo",
		ClaimedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}

	d.RecoverOrphanedIssues(context.Background())

	// Issue should still be claimed — not re-queued.
	issue := tracker.issues[20]
	if !hasLabel(issue.Labels, models.LabelStatusClaimed) {
		t.Error("actively claimed issue should keep status:claimed")
	}
	if hasLabel(issue.Labels, models.LabelStatusQueued) {
		t.Error("actively claimed issue should not get status:queued")
	}
	if len(tracker.comments[20]) > 0 {
		t.Error("actively claimed issue should not get a RECOVER comment")
	}
}

func TestRecoverOrphanedIssues_SkipsQueuedAndOtherStatuses(t *testing.T) {
	tracker := newMockTracker()
	d := newTestDispatcher(tracker)

	// Seed issues with non-orphan labels.
	tracker.issues[30] = &forge.Issue{
		Number: 30,
		Title:  "Already queued",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusQueued},
	}
	tracker.issues[31] = &forge.Issue{
		Number: 31,
		Title:  "Needs human",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusNeedsHuman},
	}
	tracker.issues[32] = &forge.Issue{
		Number: 32,
		Title:  "No status",
		State:  forge.StateOpen,
		Labels: []string{},
	}

	d.RecoverOrphanedIssues(context.Background())

	// None of these should be modified.
	for _, num := range []int{30, 31, 32} {
		if len(tracker.comments[num]) > 0 {
			t.Errorf("issue #%d should not have any comments", num)
		}
	}
}

func TestRecoverOrphanedIssues_MultiRepo(t *testing.T) {
	tracker1 := newMockTracker()
	tracker2 := newMockTracker()

	cfg := DefaultConfig()
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatCheckInterval = 50 * time.Millisecond

	d := New([]TrackerEntry{
		{Owner: "org1", Repo: "alpha", Tracker: tracker1},
		{Owner: "org2", Repo: "beta", Tracker: tracker2},
	}, &mockPolicy{costThreshold: 5.0}, nil, nil, cfg, zap.NewNop())

	// One orphan in each repo.
	tracker1.issues[1] = &forge.Issue{
		Number: 1,
		Title:  "Repo1 orphan",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusClaimed},
	}
	tracker2.issues[2] = &forge.Issue{
		Number: 2,
		Title:  "Repo2 orphan",
		State:  forge.StateOpen,
		Labels: []string{models.LabelStatusInProgress},
	}

	d.RecoverOrphanedIssues(context.Background())

	// Verify both repos were scanned and orphans recovered.
	if !hasLabel(tracker1.issues[1].Labels, models.LabelStatusQueued) {
		t.Error("repo1 issue should have been re-queued")
	}
	if !hasLabel(tracker2.issues[2].Labels, models.LabelStatusQueued) {
		t.Error("repo2 issue should have been re-queued")
	}
	if len(tracker1.comments[1]) == 0 {
		t.Error("repo1 issue should have RECOVER comment")
	}
	if len(tracker2.comments[2]) == 0 {
		t.Error("repo2 issue should have RECOVER comment")
	}
}
