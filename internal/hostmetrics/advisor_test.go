package hostmetrics

import (
	"testing"
	"time"
)

func TestAnalyzeHistoryEmpty(t *testing.T) {
	if recs := AnalyzeHistory(nil, InfraContext{}); recs != nil {
		t.Fatalf("expected nil for empty history, got %v", recs)
	}
	if recs := AnalyzeHistory([]Snapshot{{}}, InfraContext{}); recs != nil {
		t.Fatalf("expected nil for single-snapshot history, got %v", recs)
	}
}

func TestCPURecommendations(t *testing.T) {
	base := time.Now()
	makeSnap := func(cpuPct float64) Snapshot {
		return Snapshot{
			CollectedAt: base,
			CPUPercent:  cpuPct,
			CPUSource:   cpuSourceLoadAvg,
			NumCPU:      4,
		}
	}

	tests := []struct {
		name      string
		snaps     []Snapshot
		wantLevel RecommendationLevel
		wantRes   string
	}{
		{
			name:      "healthy cpu",
			snaps:     repeatSnap(makeSnap(30.0), 120),
			wantLevel: RecommendationInfo,
			wantRes:   "cpu",
		},
		{
			name:      "cpu warn (>30% of window above 70%)",
			snaps:     warnPressure(makeSnap, 70.0, 80.0, 120),
			wantLevel: RecommendationWarn,
			wantRes:   "cpu",
		},
		{
			name:      "cpu critical (>25% of window above 90%)",
			snaps:     critPressure(makeSnap, 90.0, 95.0, 120),
			wantLevel: RecommendationCritical,
			wantRes:   "cpu",
		},
		{
			name:      "cpu underutilised",
			snaps:     repeatSnap(makeSnap(10.0), 120),
			wantLevel: RecommendationInfo,
			wantRes:   "cpu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := AnalyzeHistory(tt.snaps, InfraContext{})
			found := findRec(recs, tt.wantRes)
			if found == nil {
				t.Fatalf("no recommendation for resource %q; got %+v", tt.wantRes, recs)
			}
			if found.Level != tt.wantLevel {
				t.Errorf("level = %q, want %q; detail: %s", found.Level, tt.wantLevel, found.Detail)
			}
		})
	}
}

func TestDiskGrowthProjection(t *testing.T) {
	// Simulate disk growing at 1% per day from 60% — will hit 70% warn in ~10 days.
	base := time.Now().Add(-24 * time.Hour)
	snaps := make([]Snapshot, 0, 2880)
	for i := range 2880 {
		elapsed := float64(i) * 30.0 // seconds
		diskPct := 60.0 + (elapsed/86400.0)*1.0
		snaps = append(snaps, Snapshot{
			CollectedAt:    base.Add(time.Duration(i) * 30 * time.Second),
			DiskTotalBytes: 100_000,
			DiskUsedBytes:  uint64(diskPct * 1000),
			DiskPercent:    diskPct,
			CPUPercent:     30.0,
			CPUSource:      cpuSourceLoadAvg,
			NumCPU:         4,
			RAMTotalBytes:  100_000,
			RAMPercent:     50.0,
		})
	}

	recs := AnalyzeHistory(snaps, InfraContext{})
	found := findRec(recs, "disk")
	if found == nil {
		t.Fatal("no disk recommendation produced")
	}
	// Should be info or warn depending on exact projection.
	if found.Level == RecommendationCritical {
		t.Errorf("unexpected critical disk recommendation: %s", found.Detail)
	}
}

func TestLinearRegression(t *testing.T) {
	// y = 2x + 1
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{1, 3, 5, 7, 9}
	slope, intercept := linearRegression(x, y)
	if abs(slope-2.0) > 0.001 {
		t.Errorf("slope = %f, want 2.0", slope)
	}
	if abs(intercept-1.0) > 0.001 {
		t.Errorf("intercept = %f, want 1.0", intercept)
	}
}

// --- helpers ---

func repeatSnap(s Snapshot, n int) []Snapshot {
	out := make([]Snapshot, n)
	for i := range out {
		s.CollectedAt = time.Now().Add(time.Duration(i) * 30 * time.Second)
		out[i] = s
	}
	return out
}

// warnPressure builds a slice where 35% of entries are at pressurePct and rest at normalPct.
func warnPressure(mk func(float64) Snapshot, normalPct, pressurePct float64, n int) []Snapshot {
	out := make([]Snapshot, n)
	pressureCount := int(float64(n) * 0.35)
	for i := range out {
		pct := normalPct
		if i < pressureCount {
			pct = pressurePct
		}
		s := mk(pct)
		s.CollectedAt = time.Now().Add(time.Duration(i) * 30 * time.Second)
		out[i] = s
	}
	return out
}

// critPressure builds a slice where 30% of entries are at critPct.
func critPressure(mk func(float64) Snapshot, normalPct, critPct float64, n int) []Snapshot {
	out := make([]Snapshot, n)
	critCount := int(float64(n) * 0.30)
	for i := range out {
		pct := normalPct
		if i < critCount {
			pct = critPct
		}
		s := mk(pct)
		s.CollectedAt = time.Now().Add(time.Duration(i) * 30 * time.Second)
		out[i] = s
	}
	return out
}

func findRec(recs []Recommendation, resource string) *Recommendation {
	for i := range recs {
		if recs[i].Resource == resource {
			return &recs[i]
		}
	}
	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
