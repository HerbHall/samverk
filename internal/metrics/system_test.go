package metrics

import (
	"testing"
)

func TestSystemCollector_Collect(t *testing.T) {
	c := NewSystemCollector()
	snap := c.Collect()

	if snap.CollectedAt.IsZero() {
		t.Error("CollectedAt is zero")
	}
	if snap.Goroutines <= 0 {
		t.Errorf("Goroutines = %d, want > 0", snap.Goroutines)
	}
	if snap.HeapAllocBytes == 0 {
		t.Error("HeapAllocBytes = 0, want > 0")
	}
	if snap.SysBytesTotal == 0 {
		t.Error("SysBytesTotal = 0, want > 0")
	}
	// GCCycles can be 0 in short tests; no assertion.
	// NextGCBytes should be set by the runtime.
	if snap.NextGCBytes == 0 {
		t.Error("NextGCBytes = 0, want > 0")
	}
}

func TestSystemCollector_CollectTwice(t *testing.T) {
	c := NewSystemCollector()
	s1 := c.Collect()
	s2 := c.Collect()

	// Allow equal timestamps on fast machines, but second must not precede first.
	if s2.CollectedAt.Before(s1.CollectedAt) {
		t.Errorf("second CollectedAt (%v) is before first (%v)", s2.CollectedAt, s1.CollectedAt)
	}
}

func TestNewSystemCollector_NotNil(t *testing.T) {
	c := NewSystemCollector()
	if c == nil {
		t.Error("NewSystemCollector returned nil")
	}
}
