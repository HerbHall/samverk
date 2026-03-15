// Package hostmetrics collects host-level metrics (disk, RAM, CPU load, swap)
// via /proc reads and syscall.Statfs on Linux, with zero-value stubs on other
// platforms.
package hostmetrics

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// AlertLevel classifies the severity of a resource alert.
type AlertLevel string

const (
	// AlertWarn indicates a resource is approaching its limit.
	AlertWarn AlertLevel = "warning"
	// AlertCritical indicates a resource is at or near its limit.
	AlertCritical AlertLevel = "critical"
)

// Threshold constants for alert evaluation.
const (
	DiskWarnPct      = 70.0
	DiskCritPct      = 85.0
	RAMWarnPct       = 80.0
	RAMCritPct       = 90.0
	SwapWarnPct      = 25.0
	SwapCritPct      = 50.0
	CPULoadWarnRatio = 0.7
	CPULoadCritRatio = 0.9
)

// Alert represents a threshold breach for a monitored resource.
type Alert struct {
	Resource  string     `json:"resource"`
	Level     AlertLevel `json:"level"`
	Message   string     `json:"message"`
	Value     float64    `json:"value"`
	Threshold float64    `json:"threshold"`
}

// Snapshot captures host-level metrics at a point in time.
type Snapshot struct {
	CollectedAt time.Time `json:"collected_at"`

	// Disk
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskUsedBytes  uint64  `json:"disk_used_bytes"`
	DiskPercent    float64 `json:"disk_percent"`

	// RAM
	RAMTotalBytes uint64  `json:"ram_total_bytes"`
	RAMUsedBytes  uint64  `json:"ram_used_bytes"`
	RAMAvailBytes uint64  `json:"ram_avail_bytes"`
	RAMPercent    float64 `json:"ram_percent"`

	// Swap
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	SwapPercent    float64 `json:"swap_percent"`

	// CPU
	LoadAvg1  float64 `json:"load_avg_1"`
	LoadAvg5  float64 `json:"load_avg_5"`
	LoadAvg15 float64 `json:"load_avg_15"`
	NumCPU    int     `json:"num_cpu"`

	// Alerts
	Alerts []Alert `json:"alerts,omitempty"`
}

// historyCapacity is the ring buffer size: 24h at 30s intervals = 2880 entries.
const historyCapacity = 2880

// Collector gathers host-level metrics on a 30-second interval.
type Collector struct {
	mu       sync.RWMutex
	current  Snapshot
	history  []Snapshot
	hIdx     int
	hFull    bool
	rootPath string
}

// NewCollector creates a Collector that reads disk stats from rootPath.
func NewCollector(rootPath string) *Collector {
	return &Collector{
		rootPath: rootPath,
		history:  make([]Snapshot, historyCapacity),
	}
}

// Run starts the background collection loop. It blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) error {
	// Collect once immediately so Snapshot() returns data right away.
	c.collect()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collect()
		}
	}
}

// Snapshot returns the most recent metrics snapshot (thread-safe).
func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// History returns snapshots collected since the given time, oldest first.
func (c *Collector) History(since time.Time) []Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []Snapshot
	count := historyCapacity
	if !c.hFull {
		count = c.hIdx
	}

	// Walk the ring buffer in chronological order.
	start := 0
	if c.hFull {
		start = c.hIdx
	}
	for i := range count {
		idx := (start + i) % historyCapacity
		s := c.history[idx]
		if !s.CollectedAt.Before(since) {
			out = append(out, s)
		}
	}
	return out
}

func (c *Collector) collect() {
	now := time.Now()

	snap := Snapshot{
		CollectedAt: now,
		NumCPU:      runtime.NumCPU(),
	}

	// Read disk usage.
	if diskTotal, diskUsed, err := readDiskUsage(c.rootPath); err == nil {
		snap.DiskTotalBytes = diskTotal
		snap.DiskUsedBytes = diskUsed
		if diskTotal > 0 {
			snap.DiskPercent = float64(diskUsed) / float64(diskTotal) * 100.0
		}
	}

	// Read memory info.
	if memTotal, memAvail, swapTotal, swapFree, err := readMemInfo(); err == nil {
		snap.RAMTotalBytes = memTotal
		snap.RAMAvailBytes = memAvail
		if memTotal > memAvail {
			snap.RAMUsedBytes = memTotal - memAvail
		}
		if memTotal > 0 {
			snap.RAMPercent = float64(snap.RAMUsedBytes) / float64(memTotal) * 100.0
		}
		snap.SwapTotalBytes = swapTotal
		if swapTotal > swapFree {
			snap.SwapUsedBytes = swapTotal - swapFree
		}
		if swapTotal > 0 {
			snap.SwapPercent = float64(snap.SwapUsedBytes) / float64(swapTotal) * 100.0
		}
	}

	// Read load averages.
	if l1, l5, l15, err := readLoadAvg(); err == nil {
		snap.LoadAvg1 = l1
		snap.LoadAvg5 = l5
		snap.LoadAvg15 = l15
	}

	// Evaluate alerts.
	snap.Alerts = EvaluateAlerts(snap)

	// Store snapshot.
	c.mu.Lock()
	c.current = snap
	c.history[c.hIdx] = snap
	c.hIdx++
	if c.hIdx >= historyCapacity {
		c.hIdx = 0
		c.hFull = true
	}
	c.mu.Unlock()
}

// EvaluateAlerts checks the snapshot against threshold constants and returns
// any triggered alerts. Exported for testing.
func EvaluateAlerts(s Snapshot) []Alert {
	var alerts []Alert

	// Disk alerts.
	if s.DiskTotalBytes > 0 {
		alerts = checkThreshold(alerts, "disk", s.DiskPercent, DiskWarnPct, DiskCritPct)
	}

	// RAM alerts.
	if s.RAMTotalBytes > 0 {
		alerts = checkThreshold(alerts, "ram", s.RAMPercent, RAMWarnPct, RAMCritPct)
	}

	// Swap alerts.
	if s.SwapTotalBytes > 0 {
		alerts = checkThreshold(alerts, "swap", s.SwapPercent, SwapWarnPct, SwapCritPct)
	}

	// CPU load alerts (1-min load relative to CPU count).
	if s.NumCPU > 0 {
		loadRatio := s.LoadAvg1 / float64(s.NumCPU)
		warnThreshold := CPULoadWarnRatio * 100.0
		critThreshold := CPULoadCritRatio * 100.0
		ratioPercent := loadRatio * 100.0
		alerts = checkThreshold(alerts, "cpu", ratioPercent, warnThreshold, critThreshold)
	}

	return alerts
}

// checkThreshold appends a warning or critical alert if value exceeds thresholds.
func checkThreshold(alerts []Alert, resource string, value, warn, crit float64) []Alert {
	if value >= crit {
		return append(alerts, Alert{
			Resource:  resource,
			Level:     AlertCritical,
			Message:   resource + " usage critical",
			Value:     value,
			Threshold: crit,
		})
	}
	if value >= warn {
		return append(alerts, Alert{
			Resource:  resource,
			Level:     AlertWarn,
			Message:   resource + " usage elevated",
			Value:     value,
			Threshold: warn,
		})
	}
	return alerts
}
