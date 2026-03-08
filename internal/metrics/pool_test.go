package metrics

import (
	"testing"
	"time"
)

func TestPoolMetrics_WorkerTracking(t *testing.T) {
	pm := NewPoolMetrics(3)

	snap := pm.Snapshot()
	if snap.TotalWorkers != 3 {
		t.Errorf("TotalWorkers = %d, want 3", snap.TotalWorkers)
	}
	if snap.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d, want 0", snap.ActiveWorkers)
	}
	if snap.IdleWorkers != 3 {
		t.Errorf("IdleWorkers = %d, want 3", snap.IdleWorkers)
	}

	pm.WorkerStarted()
	pm.WorkerStarted()

	snap = pm.Snapshot()
	if snap.ActiveWorkers != 2 {
		t.Errorf("ActiveWorkers = %d, want 2", snap.ActiveWorkers)
	}
	if snap.IdleWorkers != 1 {
		t.Errorf("IdleWorkers = %d, want 1", snap.IdleWorkers)
	}

	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.WorkerFinished(200*time.Millisecond, TaskResultFailure)

	snap = pm.Snapshot()
	if snap.TasksCompleted != 1 {
		t.Errorf("TasksCompleted = %d, want 1", snap.TasksCompleted)
	}
	if snap.TasksFailed != 1 {
		t.Errorf("TasksFailed = %d, want 1", snap.TasksFailed)
	}
	if snap.AvgTaskDuration == 0 {
		t.Error("AvgTaskDuration = 0, want > 0")
	}
}

func TestPoolMetrics_QueueDepth(t *testing.T) {
	pm := NewPoolMetrics(2)
	pm.SetQueueDepth(5)
	snap := pm.Snapshot()
	if snap.QueueDepth != 5 {
		t.Errorf("QueueDepth = %d, want 5", snap.QueueDepth)
	}
}

func TestPoolMetrics_P95(t *testing.T) {
	pm := NewPoolMetrics(4)
	// Add 25 durations so P95 is computed (requires >= 20).
	for i := 1; i <= 25; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(time.Duration(i)*time.Millisecond, TaskResultSuccess)
	}
	snap := pm.Snapshot()
	if snap.P95TaskDuration == 0 {
		t.Error("P95TaskDuration = 0, want > 0 after 25 tasks")
	}
}

func TestPoolMetrics_P95BelowThreshold(t *testing.T) {
	pm := NewPoolMetrics(1)
	// Add fewer than 20 durations; P95 must remain 0.
	for i := 0; i < 10; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(time.Millisecond, TaskResultSuccess)
	}
	snap := pm.Snapshot()
	if snap.P95TaskDuration != 0 {
		t.Errorf("P95TaskDuration = %v, want 0 when fewer than 20 samples", snap.P95TaskDuration)
	}
}

func TestPoolMetrics_AvgDuration(t *testing.T) {
	pm := NewPoolMetrics(1)
	pm.WorkerStarted()
	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.WorkerStarted()
	pm.WorkerFinished(300*time.Millisecond, TaskResultSuccess)

	snap := pm.Snapshot()
	want := 200 * time.Millisecond
	if snap.AvgTaskDuration != want {
		t.Errorf("AvgTaskDuration = %v, want %v", snap.AvgTaskDuration, want)
	}
}

func TestPoolMetrics_ActiveFloorAtZero(t *testing.T) {
	pm := NewPoolMetrics(2)
	// WorkerFinished without a matching WorkerStarted should not underflow.
	pm.WorkerFinished(10*time.Millisecond, TaskResultSuccess)
	snap := pm.Snapshot()
	if snap.ActiveWorkers < 0 {
		t.Errorf("ActiveWorkers = %d, want >= 0", snap.ActiveWorkers)
	}
}

func TestPoolMetrics_RingBufferWrap(t *testing.T) {
	pm := NewPoolMetrics(1)
	// Write more than durationWindowSize entries to exercise ring-buffer wrap.
	for i := 0; i < durationWindowSize+10; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(time.Duration(i+1)*time.Millisecond, TaskResultSuccess)
	}
	snap := pm.Snapshot()
	if snap.AvgTaskDuration == 0 {
		t.Error("AvgTaskDuration = 0 after ring buffer wrap, want > 0")
	}
}

func TestPoolMetrics_CollectedAtSet(t *testing.T) {
	pm := NewPoolMetrics(1)
	snap := pm.Snapshot()
	if snap.CollectedAt.IsZero() {
		t.Error("CollectedAt is zero")
	}
}
