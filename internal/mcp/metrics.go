package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/herbhall/samverk/internal/logstore"
	"github.com/herbhall/samverk/internal/metrics"
	"github.com/herbhall/samverk/internal/provider"
	"github.com/herbhall/samverk/pkg/models"
)

// poolMetricsSource provides pool stats for the digest.
type poolMetricsSource interface {
	Snapshot() metrics.PoolSnapshot
}

// dispatcherMetricsSource provides dispatcher stats for the digest.
type dispatcherMetricsSource interface {
	Snapshot() metrics.DispatcherSnapshot
}

// systemMetricsSource provides system stats for the digest.
type systemMetricsSource interface {
	Collect() metrics.SystemSnapshot
}

// scalingEventReader reads recent scaling events from durable storage.
// store.Store satisfies this interface.
type scalingEventReader interface {
	ListScalingEvents(ctx context.Context, limit int) ([]*models.ScalingEvent, error)
}

// providerHealthSource provides cached health status for all providers.
// provider.HealthMonitor satisfies this interface.
type providerHealthSource interface {
	AllHealth() []provider.ProviderHealth
}

// logQuerier queries structured logs from the log store.
// logstore.LogStore satisfies this interface.
type logQuerier interface {
	Query(ctx context.Context, f logstore.QueryFilter) ([]logstore.LogEntry, error)
}

// WorkerInfo is a summary of a registered PC agent worker for digest output.
type WorkerInfo struct {
	AgentID         string
	Hostname        string
	Status          string
	CurrentTask     *int
	ActiveWorktrees int
	CPUPercent      float64
	MemoryPercent   float64
}

// workerLister provides registered worker snapshots for the digest.
// api.API satisfies this interface via ListWorkers.
type workerLister interface {
	ListWorkers() []WorkerInfo
}

// SetMetrics attaches runtime metrics sources to the handler.
// Sources may be nil; only non-nil sources appear in the digest.
func (h *Handler) SetMetrics(pool poolMetricsSource, disp dispatcherMetricsSource, sys systemMetricsSource) {
	h.poolM = pool
	h.dispM = disp
	h.sysM = sys
}

// SetScalingEventReader attaches a scaling event reader for the digest.
// When set, recent scaling activity appears in the System Health section.
func (h *Handler) SetScalingEventReader(r scalingEventReader) {
	h.scalingEvents = r
}

// derivePressure computes a pressure level and reasons from raw pool and system
// snapshots using the same thresholds as internal/api/pressure.go.
// Both sources may be nil (zero value snapshots contribute no signal).
func derivePressure(pool poolMetricsSource, sys systemMetricsSource) (level string, reasons []string) {
	type pLevel int
	const (
		pLow      pLevel = iota
		pModerate pLevel = iota
		pHigh     pLevel = iota
		pCritical pLevel = iota
	)
	names := []string{"low", "moderate", "high", "critical"}
	best := pLow

	raise := func(lv pLevel, reason string) {
		reasons = append(reasons, reason)
		if lv > best {
			best = lv
		}
	}

	if sys != nil {
		snap := sys.Collect()
		if snap.SysBytesTotal > 0 {
			ratio := float64(snap.HeapAllocBytes) / float64(snap.SysBytesTotal)
			switch {
			case ratio > 0.90:
				raise(pCritical, "memory above 90%")
			case ratio > 0.80:
				raise(pHigh, "memory above 80%")
			case ratio > 0.60:
				raise(pModerate, "memory above 60%")
			}
		}
	}

	if pool != nil {
		snap := pool.Snapshot()
		allBusy := snap.IdleWorkers == 0
		queued := snap.QueueDepth > 0
		switch {
		case allBusy && queued:
			raise(pHigh, "all workers busy with queued tasks")
		case allBusy:
			raise(pModerate, "all workers busy")
		case queued:
			raise(pModerate, "tasks queued")
		}
	}

	if len(reasons) == 0 {
		reasons = nil
	}
	return names[best], reasons
}

// formatMetricsSection renders a brief METRICS block appended to the digest text.
func (h *Handler) formatMetricsSection() string {
	if h.poolM == nil && h.dispM == nil && h.sysM == nil && h.scalingEvents == nil && h.workersM == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n--- RUNTIME METRICS ---\n\n")

	// Pressure indicator (computed from available sources).
	if h.poolM != nil || h.sysM != nil {
		level, reasons := derivePressure(h.poolM, h.sysM)
		if len(reasons) > 0 {
			fmt.Fprintf(&b, "Pressure: %s (%s)\n", level, strings.Join(reasons, ", "))
		} else {
			fmt.Fprintf(&b, "Pressure: %s\n", level)
		}
	}

	if h.poolM != nil {
		snap := h.poolM.Snapshot()
		fmt.Fprintf(&b, "Pool: %d workers (%d active, %d idle) | queue: %d | completed: %d | failed: %d\n",
			snap.TotalWorkers, snap.ActiveWorkers, snap.IdleWorkers,
			snap.QueueDepth, snap.TasksCompleted, snap.TasksFailed)
		if snap.AvgTaskDuration > 0 {
			fmt.Fprintf(&b, "  Avg task: %dms | P95: %dms\n",
				snap.AvgTaskDuration.Milliseconds(), snap.P95TaskDuration.Milliseconds())
		}
	} else {
		b.WriteString("Pool: not running in this process\n")
	}

	if h.dispM != nil {
		snap := h.dispM.Snapshot()
		fmt.Fprintf(&b, "Dispatcher: %d claimed | %d routed | %d requeued | %d events",
			snap.ClaimedCount, snap.TotalRouted, snap.TotalRequeued, snap.TotalEventsProcessed)
		if snap.AvgPollLatency > 0 {
			fmt.Fprintf(&b, " | poll latency: %dms", snap.AvgPollLatency.Milliseconds())
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Dispatcher: not running in this process\n")
	}

	if h.sysM != nil {
		snap := h.sysM.Collect()
		memPct := 0.0
		if snap.SysBytesTotal > 0 {
			memPct = float64(snap.HeapAllocBytes) / float64(snap.SysBytesTotal) * 100
		}
		fmt.Fprintf(&b, "System: %d goroutines | heap: %s (%.0f%% of sys) | GC cycles: %d\n",
			snap.Goroutines, formatBytes(snap.HeapAllocBytes), memPct, snap.GCCycles)
	}

	if h.scalingEvents != nil {
		events, err := h.scalingEvents.ListScalingEvents(context.Background(), 3)
		if err == nil && len(events) > 0 {
			b.WriteString("Scaling (recent):")
			for _, e := range events {
				fmt.Fprintf(&b, "\n  [%s] %s %d→%d workers — %s (conf %.0f%%)",
					e.Timestamp.UTC().Format("15:04:05Z"),
					e.Action,
					e.FromWorkers, e.ToWorkers,
					e.Reason,
					e.Confidence*100,
				)
			}
			b.WriteString("\n")
		}
	}

	if h.workersM != nil {
		workers := h.workersM.ListWorkers()
		if len(workers) > 0 {
			b.WriteString("PC Workers:")
			for _, w := range workers {
				task := "idle"
				if w.CurrentTask != nil {
					task = fmt.Sprintf("issue #%d", *w.CurrentTask)
				}
				fmt.Fprintf(&b, "\n  %s (%s) — %s | worktrees: %d | cpu: %.0f%% mem: %.0f%%",
					w.AgentID, w.Hostname, task, w.ActiveWorktrees, w.CPUPercent, w.MemoryPercent)
				if w.Status == "offline" {
					b.WriteString(" [OFFLINE]")
				}
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// formatBytes returns a human-readable byte count.
func formatBytes(b uint64) string {
	switch {
	case b >= 1_073_741_824:
		return fmt.Sprintf("%.1f GB", float64(b)/1_073_741_824)
	case b >= 1_048_576:
		return fmt.Sprintf("%.1f MB", float64(b)/1_048_576)
	case b >= 1_024:
		return fmt.Sprintf("%.1f KB", float64(b)/1_024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
