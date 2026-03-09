package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/herbhall/samverk/internal/metrics"
)

// stubPool implements poolMetricsSource for tests.
type stubPool struct {
	snap metrics.PoolSnapshot
}

func (s *stubPool) Snapshot() metrics.PoolSnapshot { return s.snap }

// stubSys implements systemMetricsSource for tests.
type stubSys struct {
	snap metrics.SystemSnapshot
}

func (s *stubSys) Collect() metrics.SystemSnapshot { return s.snap }

func TestDerivePressure_NilSources(t *testing.T) {
	level, reasons := derivePressure(nil, nil)
	if level != "low" {
		t.Errorf("level = %q, want low", level)
	}
	if len(reasons) != 0 {
		t.Errorf("reasons = %v, want nil", reasons)
	}
}

func TestDerivePressure_MemoryCritical(t *testing.T) {
	sys := &stubSys{snap: metrics.SystemSnapshot{
		HeapAllocBytes: 950,
		SysBytesTotal:  1000, // 95%
	}}
	level, reasons := derivePressure(nil, sys)
	if level != "critical" {
		t.Errorf("level = %q, want critical", level)
	}
	if len(reasons) == 0 {
		t.Error("expected reasons, got none")
	}
}

func TestDerivePressure_PoolBusyAndQueued(t *testing.T) {
	pool := &stubPool{snap: metrics.PoolSnapshot{
		IdleWorkers: 0,
		QueueDepth:  2,
	}}
	level, reasons := derivePressure(pool, nil)
	if level != "high" {
		t.Errorf("level = %q, want high", level)
	}
	if len(reasons) == 0 {
		t.Error("expected reasons, got none")
	}
}

func TestDerivePressure_HighestLevelWins(t *testing.T) {
	// pool = moderate, memory = high → result must be high
	pool := &stubPool{snap: metrics.PoolSnapshot{IdleWorkers: 0, QueueDepth: 0}}
	sys := &stubSys{snap: metrics.SystemSnapshot{
		HeapAllocBytes: 850,
		SysBytesTotal:  1000, // 85% → high
	}}
	level, reasons := derivePressure(pool, sys)
	if level != "high" {
		t.Errorf("level = %q, want high", level)
	}
	if len(reasons) < 2 {
		t.Errorf("expected >= 2 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestFormatMetricsSection_IncludesPressure(t *testing.T) {
	h := &Handler{}
	h.poolM = &stubPool{snap: metrics.PoolSnapshot{
		TotalWorkers:  3,
		ActiveWorkers: 3,
		IdleWorkers:   0,
		QueueDepth:    2,
		CollectedAt:   time.Now(),
	}}

	out := h.formatMetricsSection()

	if !strings.Contains(out, "Pressure:") {
		t.Errorf("expected Pressure line in output, got:\n%s", out)
	}
	if !strings.Contains(out, "high") {
		t.Errorf("expected high pressure in output, got:\n%s", out)
	}
}

func TestFormatMetricsSection_OmittedWhenNoSources(t *testing.T) {
	h := &Handler{}
	if out := h.formatMetricsSection(); out != "" {
		t.Errorf("expected empty string when no sources, got %q", out)
	}
}

func TestFormatMetricsSection_SystemMemoryPercent(t *testing.T) {
	h := &Handler{}
	// 50% heap ratio → below moderate threshold (60%), so pressure = low
	h.sysM = &stubSys{snap: metrics.SystemSnapshot{
		HeapAllocBytes: 500,
		SysBytesTotal:  1000,
		Goroutines:     5,
	}}

	out := h.formatMetricsSection()

	if !strings.Contains(out, "50%") {
		t.Errorf("expected 50%% memory percent in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Pressure: low") {
		t.Errorf("expected low pressure (50%% < 60%% threshold), got:\n%s", out)
	}
}
