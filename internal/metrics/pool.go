package metrics

import (
	"sort"
	"sync"
	"time"
)

// TaskResult represents the outcome of a single completed task.
type TaskResult string

const (
	// TaskResultSuccess indicates the task completed without error.
	TaskResultSuccess TaskResult = "success"
	// TaskResultFailure indicates the task completed with an error.
	TaskResultFailure TaskResult = "failure"
)

// PoolSnapshot captures agent pool state at a point in time.
type PoolSnapshot struct {
	// CollectedAt is the time the snapshot was taken.
	CollectedAt time.Time

	// TotalWorkers is the total worker goroutines configured.
	TotalWorkers int
	// ActiveWorkers is the number of workers currently processing a task.
	ActiveWorkers int
	// IdleWorkers is the number of workers currently idle (TotalWorkers - ActiveWorkers).
	IdleWorkers int
	// QueueDepth is the number of tasks waiting in the queue.
	QueueDepth int
	// TasksCompleted is the total tasks completed successfully since pool start.
	TasksCompleted int64
	// TasksFailed is the total tasks that failed since pool start.
	TasksFailed int64

	// AvgTaskDuration is the rolling average task duration over the last N completed tasks.
	// Zero if no tasks have completed.
	AvgTaskDuration time.Duration
	// P95TaskDuration is the approximate P95 task duration over the last N tasks.
	// Zero if fewer than 20 tasks have completed.
	P95TaskDuration time.Duration
}

// durationWindowSize is the sliding window size for task duration tracking.
const durationWindowSize = 100

// PoolMetrics tracks live pool statistics. It is safe for concurrent use.
// The agent pool updates it via WorkerStarted, WorkerFinished, and SetQueueDepth.
type PoolMetrics struct {
	mu           sync.Mutex
	totalWorkers int
	active       int
	queueDepth   int
	completed    int64
	failed       int64
	// durations is a ring buffer of recent task durations.
	durations []time.Duration
	dIdx      int  // next write position in the ring buffer
	filled    bool // true once the ring buffer has been fully written once
}

// NewPoolMetrics creates a PoolMetrics with the given total worker count.
func NewPoolMetrics(totalWorkers int) *PoolMetrics {
	return &PoolMetrics{
		totalWorkers: totalWorkers,
		durations:    make([]time.Duration, durationWindowSize),
	}
}

// SetQueueDepth updates the current queue depth. Call this from the pool's Submit.
func (pm *PoolMetrics) SetQueueDepth(n int) {
	pm.mu.Lock()
	pm.queueDepth = n
	pm.mu.Unlock()
}

// WorkerStarted increments the active worker count.
func (pm *PoolMetrics) WorkerStarted() {
	pm.mu.Lock()
	pm.active++
	pm.mu.Unlock()
}

// WorkerFinished decrements the active worker count and records the task result and duration.
func (pm *PoolMetrics) WorkerFinished(d time.Duration, result TaskResult) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.active > 0 {
		pm.active--
	}

	switch result {
	case TaskResultSuccess:
		pm.completed++
	case TaskResultFailure:
		pm.failed++
	}

	pm.durations[pm.dIdx] = d
	pm.dIdx++
	if pm.dIdx >= durationWindowSize {
		pm.dIdx = 0
		pm.filled = true
	}
}

// Snapshot returns a point-in-time copy of the pool metrics.
func (pm *PoolMetrics) Snapshot() PoolSnapshot {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	s := PoolSnapshot{
		CollectedAt:    time.Now(),
		TotalWorkers:   pm.totalWorkers,
		ActiveWorkers:  pm.active,
		IdleWorkers:    pm.totalWorkers - pm.active,
		QueueDepth:     pm.queueDepth,
		TasksCompleted: pm.completed,
		TasksFailed:    pm.failed,
	}

	window := pm.durations
	if !pm.filled {
		window = pm.durations[:pm.dIdx]
	}

	if len(window) > 0 {
		var total time.Duration
		for _, d := range window {
			total += d
		}
		s.AvgTaskDuration = total / time.Duration(len(window))

		if len(window) >= 20 {
			sorted := make([]time.Duration, len(window))
			copy(sorted, window)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			p95idx := len(sorted) * 95 / 100
			s.P95TaskDuration = sorted[p95idx]
		}
	}

	return s
}
