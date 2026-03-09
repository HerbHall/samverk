package api

// pressureDTO is the JSON body for the pressure indicator in /api/v1/metrics and /healthz.
type pressureDTO struct {
	Level   string   `json:"level"`    // "low", "moderate", "high", or "critical"
	Reasons []string `json:"reasons"`  // human-readable explanations; nil when level is "low"
}

// pressureLevel encodes the ordered severity of a pressure level.
type pressureLevel int

const (
	pressureLow      pressureLevel = iota // 0
	pressureModerate                      // 1
	pressureHigh                          // 2
	pressureCritical                      // 3
)

var pressureLevelNames = []string{"low", "moderate", "high", "critical"}

// computePressure derives a pressure level from current pool and system snapshots.
// Both inputs may be nil; nil sources contribute no signal.
//
// Thresholds (Phase 2 — configuration-driven tuning deferred to Phase 3):
//   - critical : memory ratio > 90 %
//   - high     : memory ratio > 80 % OR all workers busy with queued tasks
//   - moderate : memory ratio > 60 % OR all workers busy OR queue depth > 0
//   - low      : everything else
//
// "memory ratio" is HeapAllocBytes/SysBytesTotal — a proxy for Go runtime
// memory pressure. OS-level CPU% requires cgroup/procfs reads not yet wired.
func computePressure(pool *poolMetricsDTO, sys *systemMetricsDTO) pressureDTO {
	max := pressureLow
	var reasons []string

	raise := func(level pressureLevel, reason string) {
		reasons = append(reasons, reason)
		if level > max {
			max = level
		}
	}

	// Memory pressure (HeapAlloc / Sys as a proxy for Go memory utilization).
	if sys != nil && sys.SysBytesTotal > 0 {
		ratio := float64(sys.HeapAllocBytes) / float64(sys.SysBytesTotal)
		switch {
		case ratio > 0.90:
			raise(pressureCritical, "memory above 90%")
		case ratio > 0.80:
			raise(pressureHigh, "memory above 80%")
		case ratio > 0.60:
			raise(pressureModerate, "memory above 60%")
		}
	}

	// Pool pressure.
	if pool != nil {
		allBusy := pool.IdleWorkers == 0
		queued := pool.QueueDepth > 0
		switch {
		case allBusy && queued:
			raise(pressureHigh, "all workers busy with queued tasks")
		case allBusy:
			raise(pressureModerate, "all workers busy")
		case queued:
			raise(pressureModerate, "tasks queued")
		}
	}

	if len(reasons) == 0 {
		reasons = nil
	}
	return pressureDTO{Level: pressureLevelNames[max], Reasons: reasons}
}
