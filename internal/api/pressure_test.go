package api

import (
	"testing"
)

func TestComputePressure_NilInputs(t *testing.T) {
	p := computePressure(nil, nil)
	if p.Level != "low" {
		t.Errorf("level = %q, want low", p.Level)
	}
	if len(p.Reasons) != 0 {
		t.Errorf("reasons = %v, want nil", p.Reasons)
	}
}

func TestComputePressure_MemoryCritical(t *testing.T) {
	sys := &systemMetricsDTO{
		HeapAllocBytes: 950,
		SysBytesTotal:  1000, // 95% ratio
	}
	p := computePressure(nil, sys)
	if p.Level != "critical" {
		t.Errorf("level = %q, want critical", p.Level)
	}
	if len(p.Reasons) == 0 {
		t.Error("expected reasons, got none")
	}
}

func TestComputePressure_MemoryHigh(t *testing.T) {
	sys := &systemMetricsDTO{
		HeapAllocBytes: 850,
		SysBytesTotal:  1000, // 85%
	}
	p := computePressure(nil, sys)
	if p.Level != "high" {
		t.Errorf("level = %q, want high", p.Level)
	}
}

func TestComputePressure_MemoryModerate(t *testing.T) {
	sys := &systemMetricsDTO{
		HeapAllocBytes: 700,
		SysBytesTotal:  1000, // 70%
	}
	p := computePressure(nil, sys)
	if p.Level != "moderate" {
		t.Errorf("level = %q, want moderate", p.Level)
	}
}

func TestComputePressure_PoolAllBusyQueued(t *testing.T) {
	pool := &poolMetricsDTO{IdleWorkers: 0, QueueDepth: 5}
	p := computePressure(pool, nil)
	if p.Level != "high" {
		t.Errorf("level = %q, want high", p.Level)
	}
}

func TestComputePressure_PoolAllBusyNoQueue(t *testing.T) {
	pool := &poolMetricsDTO{IdleWorkers: 0, QueueDepth: 0}
	p := computePressure(pool, nil)
	if p.Level != "moderate" {
		t.Errorf("level = %q, want moderate", p.Level)
	}
}

func TestComputePressure_PoolIdleWithQueue(t *testing.T) {
	pool := &poolMetricsDTO{IdleWorkers: 2, QueueDepth: 3}
	p := computePressure(pool, nil)
	if p.Level != "moderate" {
		t.Errorf("level = %q, want moderate", p.Level)
	}
}

func TestComputePressure_PoolHealthy(t *testing.T) {
	pool := &poolMetricsDTO{IdleWorkers: 3, QueueDepth: 0}
	p := computePressure(pool, nil)
	if p.Level != "low" {
		t.Errorf("level = %q, want low", p.Level)
	}
}

func TestComputePressure_MemoryLow(t *testing.T) {
	sys := &systemMetricsDTO{
		HeapAllocBytes: 400,
		SysBytesTotal:  1000, // 40%
	}
	p := computePressure(nil, sys)
	if p.Level != "low" {
		t.Errorf("level = %q, want low", p.Level)
	}
}

// TestComputePressure_Combined verifies the highest level wins when multiple
// signals are present.
func TestComputePressure_Combined(t *testing.T) {
	// pool is moderate, memory is high → result must be high
	pool := &poolMetricsDTO{IdleWorkers: 0, QueueDepth: 0} // moderate
	sys := &systemMetricsDTO{
		HeapAllocBytes: 850,
		SysBytesTotal:  1000, // 85% → high
	}
	p := computePressure(pool, sys)
	if p.Level != "high" {
		t.Errorf("level = %q, want high", p.Level)
	}
	if len(p.Reasons) < 2 {
		t.Errorf("expected >= 2 reasons, got %d: %v", len(p.Reasons), p.Reasons)
	}
}

func TestComputePressure_ZeroSysBytes(t *testing.T) {
	// SysBytesTotal = 0 must not cause division by zero.
	sys := &systemMetricsDTO{
		HeapAllocBytes: 100,
		SysBytesTotal:  0,
	}
	p := computePressure(nil, sys)
	if p.Level != "low" {
		t.Errorf("level = %q, want low (no signal when sys bytes = 0)", p.Level)
	}
}
