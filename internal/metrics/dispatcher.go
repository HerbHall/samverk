package metrics

import (
	"sync"
	"time"
)

// DispatcherSnapshot captures dispatcher state at a point in time.
type DispatcherSnapshot struct {
	// CollectedAt is the time the snapshot was taken.
	CollectedAt time.Time

	// ClaimedCount is the number of issues currently claimed by agents.
	ClaimedCount int
	// TotalRouted is the total issues routed since dispatcher start.
	TotalRouted int64
	// TotalRequeued is the total issues that failed and were requeued.
	TotalRequeued int64
	// TotalEventsProcessed is the total events processed since dispatcher start.
	TotalEventsProcessed int64
	// AvgPollLatency is the rolling average time between poll cycle completions.
	// Zero if no cycles have completed.
	AvgPollLatency time.Duration
	// LastPollAt is the time the last poll cycle completed.
	LastPollAt time.Time
}

// pollWindowSize is the sliding window size for poll latency tracking.
const pollWindowSize = 50

// DispatcherMetrics tracks live dispatcher statistics. It is safe for concurrent use.
type DispatcherMetrics struct {
	mu            sync.Mutex
	claimed       int
	totalRouted   int64
	totalRequeued int64
	totalEvents   int64
	lastPollAt    time.Time
	// pollLatencies is a ring buffer of recent poll cycle durations.
	pollLatencies []time.Duration
	pIdx          int  // next write position in the ring buffer
	pollFilled    bool // true once the ring buffer has been fully written once
}

// NewDispatcherMetrics creates a DispatcherMetrics.
func NewDispatcherMetrics() *DispatcherMetrics {
	return &DispatcherMetrics{
		pollLatencies: make([]time.Duration, pollWindowSize),
	}
}

// IssueClaimed increments the claimed issue count.
func (dm *DispatcherMetrics) IssueClaimed() {
	dm.mu.Lock()
	dm.claimed++
	dm.mu.Unlock()
}

// IssueUnclaimed decrements the claimed issue count, floored at zero.
func (dm *DispatcherMetrics) IssueUnclaimed() {
	dm.mu.Lock()
	if dm.claimed > 0 {
		dm.claimed--
	}
	dm.mu.Unlock()
}

// IssueRouted increments the total routed counter.
func (dm *DispatcherMetrics) IssueRouted() {
	dm.mu.Lock()
	dm.totalRouted++
	dm.mu.Unlock()
}

// IssueRequeued increments the requeue counter.
func (dm *DispatcherMetrics) IssueRequeued() {
	dm.mu.Lock()
	dm.totalRequeued++
	dm.mu.Unlock()
}

// EventProcessed increments the event counter.
func (dm *DispatcherMetrics) EventProcessed() {
	dm.mu.Lock()
	dm.totalEvents++
	dm.mu.Unlock()
}

// PollCompleted records a poll cycle's latency and updates LastPollAt.
func (dm *DispatcherMetrics) PollCompleted(d time.Duration) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.lastPollAt = time.Now()
	dm.pollLatencies[dm.pIdx] = d
	dm.pIdx++
	if dm.pIdx >= pollWindowSize {
		dm.pIdx = 0
		dm.pollFilled = true
	}
}

// Snapshot returns a point-in-time copy of the dispatcher metrics.
func (dm *DispatcherMetrics) Snapshot() DispatcherSnapshot {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	s := DispatcherSnapshot{
		CollectedAt:          time.Now(),
		ClaimedCount:         dm.claimed,
		TotalRouted:          dm.totalRouted,
		TotalRequeued:        dm.totalRequeued,
		TotalEventsProcessed: dm.totalEvents,
		LastPollAt:           dm.lastPollAt,
	}

	window := dm.pollLatencies
	if !dm.pollFilled {
		window = dm.pollLatencies[:dm.pIdx]
	}

	if len(window) > 0 {
		var total time.Duration
		for _, d := range window {
			total += d
		}
		s.AvgPollLatency = total / time.Duration(len(window))
	}

	return s
}
