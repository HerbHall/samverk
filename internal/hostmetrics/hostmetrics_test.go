package hostmetrics

import (
	"testing"
	"time"
)

func TestEvaluateAlerts(t *testing.T) {
	tests := []struct {
		name       string
		snap       Snapshot
		wantCount  int
		wantAlerts []Alert
	}{
		{
			name: "all healthy",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 60.0,
				SwapTotalBytes: 100_000, SwapPercent: 10.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 0,
		},
		{
			name: "disk warning",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 75.0,
				RAMTotalBytes: 100_000, RAMPercent: 50.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "disk", Level: AlertWarn, Value: 75.0, Threshold: DiskWarnPct},
			},
		},
		{
			name: "disk critical",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 90.0,
				RAMTotalBytes: 100_000, RAMPercent: 50.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "disk", Level: AlertCritical, Value: 90.0, Threshold: DiskCritPct},
			},
		},
		{
			name: "ram warning",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 85.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "ram", Level: AlertWarn, Value: 85.0, Threshold: RAMWarnPct},
			},
		},
		{
			name: "ram critical",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 95.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "ram", Level: AlertCritical, Value: 95.0, Threshold: RAMCritPct},
			},
		},
		{
			name: "swap warning",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 50.0,
				SwapTotalBytes: 100_000, SwapPercent: 30.0,
				NumCPU: 4, LoadAvg1: 1.0,
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "swap", Level: AlertWarn, Value: 30.0, Threshold: SwapWarnPct},
			},
		},
		{
			name: "cpu load warning",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 50.0,
				NumCPU: 4, LoadAvg1: 3.2, // ratio 0.8 exceeds warn threshold 0.7
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "cpu", Level: AlertWarn},
			},
		},
		{
			name: "cpu load critical",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 50.0,
				RAMTotalBytes: 100_000, RAMPercent: 50.0,
				NumCPU: 4, LoadAvg1: 3.8, // ratio 0.95 exceeds critical threshold 0.9
			},
			wantCount: 1,
			wantAlerts: []Alert{
				{Resource: "cpu", Level: AlertCritical},
			},
		},
		{
			name: "multiple alerts",
			snap: Snapshot{
				DiskTotalBytes: 100_000, DiskPercent: 90.0,
				RAMTotalBytes: 100_000, RAMPercent: 95.0,
				SwapTotalBytes: 100_000, SwapPercent: 60.0,
				NumCPU: 4, LoadAvg1: 4.0,
			},
			wantCount: 4,
		},
		{
			name: "zero totals produce no alerts",
			snap: Snapshot{
				DiskTotalBytes: 0,
				RAMTotalBytes:  0,
				SwapTotalBytes: 0,
				NumCPU:         0,
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateAlerts(tt.snap)
			if len(got) != tt.wantCount {
				t.Fatalf("alert count = %d, want %d; alerts: %+v", len(got), tt.wantCount, got)
			}
			for i, want := range tt.wantAlerts {
				if i >= len(got) {
					break
				}
				if got[i].Resource != want.Resource {
					t.Errorf("alert[%d].Resource = %q, want %q", i, got[i].Resource, want.Resource)
				}
				if got[i].Level != want.Level {
					t.Errorf("alert[%d].Level = %q, want %q", i, got[i].Level, want.Level)
				}
			}
		})
	}
}

func TestRingBuffer(t *testing.T) {
	c := NewCollector("/")

	// Verify empty history.
	got := c.History(time.Time{})
	if len(got) != 0 {
		t.Fatalf("empty collector History() returned %d entries", len(got))
	}

	// Add entries via direct manipulation (bypassing collect() to avoid procfs).
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 10 {
		snap := Snapshot{CollectedAt: base.Add(time.Duration(i) * time.Minute)}
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

	// All entries.
	got = c.History(time.Time{})
	if len(got) != 10 {
		t.Fatalf("History(zero) = %d entries, want 10", len(got))
	}

	// Filtered: since minute 5.
	since := base.Add(5 * time.Minute)
	got = c.History(since)
	if len(got) != 5 {
		t.Fatalf("History(since 5m) = %d entries, want 5", len(got))
	}
	if !got[0].CollectedAt.Equal(since) {
		t.Errorf("first entry at %v, want %v", got[0].CollectedAt, since)
	}

	// Verify chronological order.
	for i := 1; i < len(got); i++ {
		if got[i].CollectedAt.Before(got[i-1].CollectedAt) {
			t.Errorf("entry %d (%v) before entry %d (%v)", i, got[i].CollectedAt, i-1, got[i-1].CollectedAt)
		}
	}
}

func TestRingBufferWrapping(t *testing.T) {
	c := NewCollector("/")

	// Fill past capacity to trigger wrapping.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	total := historyCapacity + 100
	for i := range total {
		snap := Snapshot{CollectedAt: base.Add(time.Duration(i) * 30 * time.Second)}
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

	got := c.History(time.Time{})
	if len(got) != historyCapacity {
		t.Fatalf("after wrapping, History() = %d entries, want %d", len(got), historyCapacity)
	}

	// Oldest should be entry #100 (first 100 were overwritten).
	wantOldest := base.Add(100 * 30 * time.Second)
	if !got[0].CollectedAt.Equal(wantOldest) {
		t.Errorf("oldest entry at %v, want %v", got[0].CollectedAt, wantOldest)
	}

	// Verify chronological order.
	for i := 1; i < len(got); i++ {
		if got[i].CollectedAt.Before(got[i-1].CollectedAt) {
			t.Errorf("entry %d (%v) before entry %d (%v)", i, got[i].CollectedAt, i-1, got[i-1].CollectedAt)
		}
	}
}

func TestParseMemInfo(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantTotal uint64
		wantAvail uint64
		wantSwapT uint64
		wantSwapF uint64
		wantErr   bool
	}{
		{
			name: "typical",
			data: `MemTotal:        8167848 kB
MemFree:          240972 kB
MemAvailable:    4506296 kB
Buffers:          263488 kB
Cached:          4196756 kB
SwapTotal:       2097148 kB
SwapFree:        1048576 kB
`,
			wantTotal: 8167848 * 1024,
			wantAvail: 4506296 * 1024,
			wantSwapT: 2097148 * 1024,
			wantSwapF: 1048576 * 1024,
		},
		{
			name: "no swap",
			data: `MemTotal:        8167848 kB
MemAvailable:    4506296 kB
SwapTotal:             0 kB
SwapFree:              0 kB
`,
			wantTotal: 8167848 * 1024,
			wantAvail: 4506296 * 1024,
			wantSwapT: 0,
			wantSwapF: 0,
		},
		{
			name:    "empty",
			data:    "",
			wantErr: true,
		},
		{
			name:    "garbage",
			data:    "not a meminfo file at all\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, avail, swapT, swapF, err := ParseMemInfo([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if avail != tt.wantAvail {
				t.Errorf("available = %d, want %d", avail, tt.wantAvail)
			}
			if swapT != tt.wantSwapT {
				t.Errorf("swapTotal = %d, want %d", swapT, tt.wantSwapT)
			}
			if swapF != tt.wantSwapF {
				t.Errorf("swapFree = %d, want %d", swapF, tt.wantSwapF)
			}
		})
	}
}

func TestParseLoadAvg(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want1   float64
		want5   float64
		want15  float64
		wantErr bool
	}{
		{
			name:  "typical",
			data:  "0.12 0.15 0.10 1/234 5678\n",
			want1: 0.12, want5: 0.15, want15: 0.10,
		},
		{
			name:  "high load",
			data:  "24.50 18.32 12.01 5/1024 99999\n",
			want1: 24.50, want5: 18.32, want15: 12.01,
		},
		{
			name:  "zeros",
			data:  "0.00 0.00 0.00 1/1 1\n",
			want1: 0.0, want5: 0.0, want15: 0.0,
		},
		{
			name:    "too few fields",
			data:    "0.12 0.15\n",
			wantErr: true,
		},
		{
			name:    "empty",
			data:    "",
			wantErr: true,
		},
		{
			name:    "non-numeric",
			data:    "abc def ghi\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l1, l5, l15, err := ParseLoadAvg([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if l1 != tt.want1 {
				t.Errorf("load1 = %f, want %f", l1, tt.want1)
			}
			if l5 != tt.want5 {
				t.Errorf("load5 = %f, want %f", l5, tt.want5)
			}
			if l15 != tt.want15 {
				t.Errorf("load15 = %f, want %f", l15, tt.want15)
			}
		})
	}
}

func TestSnapshotReturnsLatest(t *testing.T) {
	c := NewCollector("/")

	// Initially zero.
	snap := c.Snapshot()
	if !snap.CollectedAt.IsZero() {
		t.Fatalf("empty collector Snapshot().CollectedAt = %v, want zero", snap.CollectedAt)
	}

	// Manually set current.
	now := time.Now()
	c.mu.Lock()
	c.current = Snapshot{CollectedAt: now, NumCPU: 8}
	c.mu.Unlock()

	snap = c.Snapshot()
	if !snap.CollectedAt.Equal(now) {
		t.Errorf("Snapshot().CollectedAt = %v, want %v", snap.CollectedAt, now)
	}
	if snap.NumCPU != 8 {
		t.Errorf("Snapshot().NumCPU = %d, want 8", snap.NumCPU)
	}
}
