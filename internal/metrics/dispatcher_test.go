package metrics

import (
	"testing"
	"time"
)

func TestDispatcherMetrics_Counters(t *testing.T) {
	dm := NewDispatcherMetrics()

	dm.IssueClaimed()
	dm.IssueClaimed()
	dm.IssueRouted()
	dm.IssueRouted()
	dm.IssueRequeued()
	dm.IssueUnclaimed()

	snap := dm.Snapshot()
	if snap.ClaimedCount != 1 {
		t.Errorf("ClaimedCount = %d, want 1", snap.ClaimedCount)
	}
	if snap.TotalRouted != 2 {
		t.Errorf("TotalRouted = %d, want 2", snap.TotalRouted)
	}
	if snap.TotalRequeued != 1 {
		t.Errorf("TotalRequeued = %d, want 1", snap.TotalRequeued)
	}
}

func TestDispatcherMetrics_PollLatency(t *testing.T) {
	dm := NewDispatcherMetrics()

	dm.PollCompleted(10 * time.Millisecond)
	dm.PollCompleted(20 * time.Millisecond)
	dm.PollCompleted(30 * time.Millisecond)

	snap := dm.Snapshot()
	if snap.AvgPollLatency == 0 {
		t.Error("AvgPollLatency = 0, want > 0")
	}
	if snap.LastPollAt.IsZero() {
		t.Error("LastPollAt is zero after PollCompleted")
	}
}

func TestDispatcherMetrics_Unclamp(t *testing.T) {
	dm := NewDispatcherMetrics()
	// Should not go below 0.
	dm.IssueUnclaimed()
	dm.IssueUnclaimed()
	snap := dm.Snapshot()
	if snap.ClaimedCount < 0 {
		t.Errorf("ClaimedCount = %d, want >= 0", snap.ClaimedCount)
	}
}

func TestDispatcherMetrics_EventProcessed(t *testing.T) {
	dm := NewDispatcherMetrics()
	dm.EventProcessed()
	dm.EventProcessed()
	dm.EventProcessed()
	snap := dm.Snapshot()
	if snap.TotalEventsProcessed != 3 {
		t.Errorf("TotalEventsProcessed = %d, want 3", snap.TotalEventsProcessed)
	}
}

func TestDispatcherMetrics_AvgPollLatency(t *testing.T) {
	dm := NewDispatcherMetrics()
	dm.PollCompleted(100 * time.Millisecond)
	dm.PollCompleted(300 * time.Millisecond)

	snap := dm.Snapshot()
	want := 200 * time.Millisecond
	if snap.AvgPollLatency != want {
		t.Errorf("AvgPollLatency = %v, want %v", snap.AvgPollLatency, want)
	}
}

func TestDispatcherMetrics_NoPollLatencyBeforeFirstCycle(t *testing.T) {
	dm := NewDispatcherMetrics()
	snap := dm.Snapshot()
	if snap.AvgPollLatency != 0 {
		t.Errorf("AvgPollLatency = %v, want 0 before any poll", snap.AvgPollLatency)
	}
	if !snap.LastPollAt.IsZero() {
		t.Error("LastPollAt should be zero before first PollCompleted")
	}
}

func TestDispatcherMetrics_PollRingBufferWrap(t *testing.T) {
	dm := NewDispatcherMetrics()
	// Write more than pollWindowSize entries to exercise ring-buffer wrap.
	for i := 0; i < pollWindowSize+5; i++ {
		dm.PollCompleted(time.Duration(i+1) * time.Millisecond)
	}
	snap := dm.Snapshot()
	if snap.AvgPollLatency == 0 {
		t.Error("AvgPollLatency = 0 after ring buffer wrap, want > 0")
	}
}

func TestDispatcherMetrics_CollectedAtSet(t *testing.T) {
	dm := NewDispatcherMetrics()
	snap := dm.Snapshot()
	if snap.CollectedAt.IsZero() {
		t.Error("CollectedAt is zero")
	}
}
