package metrics

import (
	"sync"
	"testing"
	"time"
)

// TestPoolMetrics_ConcurrentAccess verifies that all PoolMetrics methods are
// race-free under concurrent access from multiple goroutines.
func TestPoolMetrics_ConcurrentAccess(t *testing.T) {
	pm := NewPoolMetrics(8)

	var wg sync.WaitGroup
	const goroutines = 16
	const opsPerGoroutine = 50

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				pm.WorkerStarted()
				pm.WorkerFinished(time.Duration(i+1)*time.Millisecond, TaskResultSuccess)
				pm.SetQueueDepth(i % 10)
				_ = pm.Snapshot()
			}
		}()
	}

	wg.Wait()

	snap := pm.Snapshot()
	if snap.TasksCompleted == 0 {
		t.Error("TasksCompleted = 0 after concurrent work, want > 0")
	}
}

// TestPoolMetrics_ActiveWorkerAccuracy verifies active worker count transitions
// match expectations under controlled conditions.
func TestPoolMetrics_ActiveWorkerAccuracy(t *testing.T) {
	const workers = 4
	pm := NewPoolMetrics(workers)

	// Start 3 of 4 workers.
	pm.WorkerStarted()
	pm.WorkerStarted()
	pm.WorkerStarted()

	snap := pm.Snapshot()
	if snap.ActiveWorkers != 3 {
		t.Errorf("ActiveWorkers = %d after 3 starts, want 3", snap.ActiveWorkers)
	}

	if snap.IdleWorkers != 1 {
		t.Errorf("IdleWorkers = %d, want 1 (4 total - 3 active)", snap.IdleWorkers)
	}

	// Finish one worker.
	pm.WorkerFinished(50*time.Millisecond, TaskResultSuccess)

	snap = pm.Snapshot()
	if snap.ActiveWorkers != 2 {
		t.Errorf("ActiveWorkers = %d after one finish, want 2", snap.ActiveWorkers)
	}

	if snap.IdleWorkers != 2 {
		t.Errorf("IdleWorkers = %d, want 2", snap.IdleWorkers)
	}

	// Finish remaining two.
	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.WorkerFinished(150*time.Millisecond, TaskResultSuccess)

	snap = pm.Snapshot()
	if snap.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d after all finished, want 0", snap.ActiveWorkers)
	}

	if snap.IdleWorkers != workers {
		t.Errorf("IdleWorkers = %d, want %d", snap.IdleWorkers, workers)
	}
}

// TestPoolMetrics_QueueDepthUnderBackpressure simulates backpressure:
// queue depth is set high while all workers are active, then returns to 0.
func TestPoolMetrics_QueueDepthUnderBackpressure(t *testing.T) {
	pm := NewPoolMetrics(3)

	// All workers busy, queue backed up.
	pm.WorkerStarted()
	pm.WorkerStarted()
	pm.WorkerStarted()
	pm.SetQueueDepth(20)

	snap := pm.Snapshot()
	if snap.QueueDepth != 20 {
		t.Errorf("QueueDepth = %d under backpressure, want 20", snap.QueueDepth)
	}

	if snap.ActiveWorkers != 3 {
		t.Errorf("ActiveWorkers = %d, want 3 under backpressure", snap.ActiveWorkers)
	}

	// Workers drain, queue clears.
	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.WorkerFinished(100*time.Millisecond, TaskResultSuccess)
	pm.SetQueueDepth(0)

	snap = pm.Snapshot()
	if snap.QueueDepth != 0 {
		t.Errorf("QueueDepth = %d after drain, want 0", snap.QueueDepth)
	}

	if snap.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d after drain, want 0", snap.ActiveWorkers)
	}
}

// TestPoolMetrics_AvgDurationAccuracy verifies the rolling average is within 10%
// of the true mean for a known distribution of task durations.
func TestPoolMetrics_AvgDurationAccuracy(t *testing.T) {
	pm := NewPoolMetrics(1)

	// Submit 30 tasks at 100ms each — true mean = 100ms.
	const taskCount = 30
	const taskDur = 100 * time.Millisecond

	for i := 0; i < taskCount; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(taskDur, TaskResultSuccess)
	}

	snap := pm.Snapshot()
	want := taskDur

	// Allow ±10% tolerance.
	lo := time.Duration(float64(want) * 0.90)
	hi := time.Duration(float64(want) * 1.10)

	if snap.AvgTaskDuration < lo || snap.AvgTaskDuration > hi {
		t.Errorf("AvgTaskDuration = %v, want within [%v, %v] (±10%% of %v)",
			snap.AvgTaskDuration, lo, hi, want)
	}
}

// TestPoolMetrics_MixedDurationAccuracy verifies average accuracy with a bimodal
// distribution: 15 tasks at 50ms and 15 tasks at 150ms — true mean = 100ms.
func TestPoolMetrics_MixedDurationAccuracy(t *testing.T) {
	pm := NewPoolMetrics(1)

	for i := 0; i < 15; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(50*time.Millisecond, TaskResultSuccess)
	}

	for i := 0; i < 15; i++ {
		pm.WorkerStarted()
		pm.WorkerFinished(150*time.Millisecond, TaskResultSuccess)
	}

	snap := pm.Snapshot()
	want := 100 * time.Millisecond
	lo := time.Duration(float64(want) * 0.90)
	hi := time.Duration(float64(want) * 1.10)

	if snap.AvgTaskDuration < lo || snap.AvgTaskDuration > hi {
		t.Errorf("AvgTaskDuration = %v, want within [%v, %v] (±10%% of %v)",
			snap.AvgTaskDuration, lo, hi, want)
	}
}

// TestDispatcherMetrics_ConcurrentAccess verifies DispatcherMetrics is race-free
// under concurrent access from multiple goroutines.
func TestDispatcherMetrics_ConcurrentAccess(t *testing.T) {
	dm := NewDispatcherMetrics()

	var wg sync.WaitGroup
	const goroutines = 16
	const opsPerGoroutine = 50

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				dm.IssueClaimed()
				dm.IssueRouted()
				dm.EventProcessed()
				dm.PollCompleted(time.Duration(i+1) * time.Millisecond)

				if i%5 == 0 {
					dm.IssueUnclaimed()
				}

				if i%7 == 0 {
					dm.IssueRequeued()
				}

				_ = dm.Snapshot()
			}
		}(g)
	}

	wg.Wait()

	snap := dm.Snapshot()
	if snap.TotalEventsProcessed == 0 {
		t.Error("TotalEventsProcessed = 0 after concurrent work, want > 0")
	}

	if snap.AvgPollLatency == 0 {
		t.Error("AvgPollLatency = 0 after concurrent polls, want > 0")
	}
}

// TestDispatcherMetrics_PollLatencyAccuracy verifies rolling average is within
// 10% of the true mean for a known distribution.
func TestDispatcherMetrics_PollLatencyAccuracy(t *testing.T) {
	dm := NewDispatcherMetrics()

	// Submit 30 polls at 40ms each — true mean = 40ms.
	const pollCount = 30
	const pollDur = 40 * time.Millisecond

	for i := 0; i < pollCount; i++ {
		dm.PollCompleted(pollDur)
	}

	snap := dm.Snapshot()
	want := pollDur
	lo := time.Duration(float64(want) * 0.90)
	hi := time.Duration(float64(want) * 1.10)

	if snap.AvgPollLatency < lo || snap.AvgPollLatency > hi {
		t.Errorf("AvgPollLatency = %v, want within [%v, %v] (±10%% of %v)",
			snap.AvgPollLatency, lo, hi, want)
	}
}

// TestSystemCollector_ConcurrentCollect verifies Collect is safe to call
// concurrently from multiple goroutines.
func TestSystemCollector_ConcurrentCollect(t *testing.T) {
	c := NewSystemCollector()

	var wg sync.WaitGroup
	const goroutines = 8

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := 0; i < 20; i++ {
				snap := c.Collect()
				if snap.Goroutines <= 0 {
					t.Errorf("Goroutines = %d in concurrent collect, want > 0", snap.Goroutines)
				}
			}
		}()
	}

	wg.Wait()
}

// TestSystemCollector_NonZeroValues verifies that a fresh collect returns
// non-zero values for fields that must always be populated on any platform.
func TestSystemCollector_NonZeroValues(t *testing.T) {
	c := NewSystemCollector()
	snap := c.Collect()

	if snap.HeapAllocBytes == 0 {
		t.Error("HeapAllocBytes = 0, want > 0 (process always has heap)")
	}

	if snap.SysBytesTotal == 0 {
		t.Error("SysBytesTotal = 0, want > 0")
	}

	if snap.NextGCBytes == 0 {
		t.Error("NextGCBytes = 0, want > 0 (runtime always sets a GC target)")
	}

	if snap.Goroutines < 1 {
		t.Errorf("Goroutines = %d, want >= 1 (at least this goroutine)", snap.Goroutines)
	}
}
