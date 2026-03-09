package metrics

import (
	"runtime"
	"time"
)

// SystemSnapshot captures OS-level metrics at a point in time.
type SystemSnapshot struct {
	// CollectedAt is the time the snapshot was taken.
	CollectedAt time.Time

	// Goroutines is the goroutine count from runtime.NumGoroutine().
	Goroutines int

	// HeapAllocBytes is bytes of allocated heap objects.
	HeapAllocBytes uint64
	// SysBytesTotal is total OS memory obtained by the Go runtime.
	SysBytesTotal uint64
	// GCCycles is the number of GC cycles since program start.
	GCCycles uint32
	// GCPauseTotalNs is total time in GC stop-the-world pauses.
	GCPauseTotalNs uint64
	// NextGCBytes is the target heap size for the next GC cycle.
	NextGCBytes uint64
}

// SystemCollector collects system-level metrics on demand.
type SystemCollector struct{}

// NewSystemCollector creates a SystemCollector.
func NewSystemCollector() *SystemCollector { return &SystemCollector{} }

// Collect returns a fresh snapshot of system metrics.
func (s *SystemCollector) Collect() SystemSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return SystemSnapshot{
		CollectedAt:    time.Now(),
		Goroutines:     runtime.NumGoroutine(),
		HeapAllocBytes: ms.HeapAlloc,
		SysBytesTotal:  ms.Sys,
		GCCycles:       ms.NumGC,
		GCPauseTotalNs: ms.PauseTotalNs,
		NextGCBytes:    ms.NextGC,
	}
}
