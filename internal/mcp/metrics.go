package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/herbhall/samverk/internal/metrics"
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

// formatMetricsSection renders a brief METRICS block appended to the digest text.
func (h *Handler) formatMetricsSection() string {
	if h.poolM == nil && h.dispM == nil && h.sysM == nil && h.scalingEvents == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n--- RUNTIME METRICS ---\n\n")

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
		fmt.Fprintf(&b, "System: %d goroutines | heap: %s | GC cycles: %d\n",
			snap.Goroutines, formatBytes(snap.HeapAllocBytes), snap.GCCycles)
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
