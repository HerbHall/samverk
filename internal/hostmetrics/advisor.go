package hostmetrics

import (
	"fmt"
	"math"
	"sort"
)

// RecommendationLevel classifies the urgency of a capacity recommendation.
type RecommendationLevel string

const (
	RecommendationInfo     RecommendationLevel = "info"
	RecommendationWarn     RecommendationLevel = "warn"
	RecommendationCritical RecommendationLevel = "critical"
)

// Recommendation is an actionable suggestion produced by analyzing metric history.
type Recommendation struct {
	Resource string              `json:"resource"`
	Level    RecommendationLevel `json:"level"`
	Title    string              `json:"title"`
	Detail   string              `json:"detail"`
}

// ResourceStats summarizes a resource over a window of snapshots.
type ResourceStats struct {
	Avg           float64
	P95           float64
	Peak          float64
	TimeAboveWarn float64 // fraction 0–1
	TimeAboveCrit float64 // fraction 0–1
	DaysToWarn    float64 // –1 if not applicable or not trending up
	DaysToCrit    float64 // –1 if not applicable or not trending up
	SampleCount   int
}

// AnalyzeHistory inspects snapshot history and returns capacity recommendations.
// At least 2 snapshots are required to produce results.
func AnalyzeHistory(snaps []Snapshot) []Recommendation {
	if len(snaps) < 2 {
		return nil
	}

	recs := make([]Recommendation, 0, 4)
	recs = append(recs, cpuRecommendations(cpuStats(snaps), snaps)...)
	recs = append(recs, ramRecommendations(ramStats(snaps))...)
	recs = append(recs, diskRecommendations(diskStats(snaps))...)
	recs = append(recs, swapRecommendations(swapStats(snaps))...)
	return recs
}

// cpuStats computes resource statistics for CPU utilisation.
func cpuStats(snaps []Snapshot) ResourceStats {
	var vals []float64
	for i := range snaps {
		if snaps[i].CPUSource != "" {
			vals = append(vals, snaps[i].CPUPercent)
		}
	}
	return computeStats(vals, CPULoadWarnRatio*100.0, CPULoadCritRatio*100.0, nil)
}

// ramStats computes resource statistics for RAM utilisation.
func ramStats(snaps []Snapshot) ResourceStats {
	var vals []float64
	for i := range snaps {
		if snaps[i].RAMTotalBytes > 0 {
			vals = append(vals, snaps[i].RAMPercent)
		}
	}
	return computeStats(vals, RAMWarnPct, RAMCritPct, nil)
}

// diskStats computes resource statistics for disk utilisation, including growth trend.
func diskStats(snaps []Snapshot) ResourceStats {
	var xs, ys []float64
	for i := range snaps {
		if snaps[i].DiskTotalBytes > 0 {
			xs = append(xs, float64(snaps[i].CollectedAt.Unix()))
			ys = append(ys, snaps[i].DiskPercent)
		}
	}
	return computeStats(ys, DiskWarnPct, DiskCritPct, xs)
}

// swapStats computes resource statistics for swap utilisation.
func swapStats(snaps []Snapshot) ResourceStats {
	var vals []float64
	for i := range snaps {
		if snaps[i].SwapTotalBytes > 0 {
			vals = append(vals, snaps[i].SwapPercent)
		}
	}
	return computeStats(vals, SwapWarnPct, SwapCritPct, nil)
}

// computeStats derives statistical summary for a value series.
// xs (optional) are Unix-second timestamps for disk growth projection.
func computeStats(vals []float64, warnPct, critPct float64, xs []float64) ResourceStats {
	n := len(vals)
	if n == 0 {
		return ResourceStats{DaysToWarn: -1, DaysToCrit: -1}
	}

	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)

	var sum, peak float64
	var aboveWarn, aboveCrit int
	for _, v := range vals {
		sum += v
		if v >= warnPct {
			aboveWarn++
		}
		if v >= critPct {
			aboveCrit++
		}
		if v > peak {
			peak = v
		}
	}

	p95Idx := int(math.Ceil(float64(n)*0.95)) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p95Idx >= n {
		p95Idx = n - 1
	}

	rs := ResourceStats{
		Avg:           sum / float64(n),
		P95:           sorted[p95Idx],
		Peak:          peak,
		TimeAboveWarn: float64(aboveWarn) / float64(n),
		TimeAboveCrit: float64(aboveCrit) / float64(n),
		SampleCount:   n,
		DaysToWarn:    -1,
		DaysToCrit:    -1,
	}

	// Linear regression for growth projection (disk only).
	if len(xs) >= 2 && len(xs) == n {
		slope, intercept := linearRegression(xs, vals)
		if slope > 0 {
			lastX := xs[n-1]
			tWarn := (warnPct - intercept) / slope
			tCrit := (critPct - intercept) / slope
			if d := (tWarn - lastX) / 86400.0; d > 0 {
				rs.DaysToWarn = d
			}
			if d := (tCrit - lastX) / 86400.0; d > 0 {
				rs.DaysToCrit = d
			}
		}
	}

	return rs
}

func cpuRecommendations(s ResourceStats, snaps []Snapshot) []Recommendation {
	if s.SampleCount == 0 {
		return nil
	}

	inLXC := len(snaps) > 0 && snaps[0].InLXC
	source := "load average"
	if inLXC {
		source = "cgroup v2"
	}

	switch {
	case s.TimeAboveCrit >= 0.25:
		return []Recommendation{{
			Resource: "cpu",
			Level:    RecommendationCritical,
			Title:    "CPU critically overloaded — add cores",
			Detail: fmt.Sprintf(
				"CPU (via %s) exceeded %.0f%% for %.0f%% of the window "+
					"(avg %.1f%%, p95 %.1f%%, peak %.1f%%). Increase CPU core allocation.",
				source, CPULoadCritRatio*100, s.TimeAboveCrit*100, s.Avg, s.P95, s.Peak,
			),
		}}
	case s.TimeAboveWarn >= 0.30:
		return []Recommendation{{
			Resource: "cpu",
			Level:    RecommendationWarn,
			Title:    "CPU frequently elevated — consider more cores",
			Detail: fmt.Sprintf(
				"CPU above %.0f%% for %.0f%% of the window (p95 %.1f%%, peak %.1f%%). "+
					"If sustained, increase CPU allocation.",
				CPULoadWarnRatio*100, s.TimeAboveWarn*100, s.P95, s.Peak,
			),
		}}
	case s.Avg < 20.0 && s.P95 < 40.0 && s.SampleCount >= 120:
		return []Recommendation{{
			Resource: "cpu",
			Level:    RecommendationInfo,
			Title:    "CPU under-utilised",
			Detail: fmt.Sprintf(
				"avg %.1f%%, p95 %.1f%% — current core allocation may exceed actual demand.",
				s.Avg, s.P95,
			),
		}}
	default:
		return []Recommendation{{
			Resource: "cpu",
			Level:    RecommendationInfo,
			Title:    "CPU healthy",
			Detail:   fmt.Sprintf("avg %.1f%%, p95 %.1f%%, peak %.1f%%", s.Avg, s.P95, s.Peak),
		}}
	}
}

func ramRecommendations(s ResourceStats) []Recommendation {
	if s.SampleCount == 0 {
		return nil
	}
	switch {
	case s.TimeAboveCrit >= 0.20:
		return []Recommendation{{
			Resource: "ram",
			Level:    RecommendationCritical,
			Title:    "RAM critically overloaded — add memory",
			Detail: fmt.Sprintf(
				"RAM above %.0f%% for %.0f%% of the window (avg %.1f%%, peak %.1f%%). "+
					"Increase RAM allocation immediately.",
				RAMCritPct, s.TimeAboveCrit*100, s.Avg, s.Peak,
			),
		}}
	case s.TimeAboveWarn >= 0.25:
		return []Recommendation{{
			Resource: "ram",
			Level:    RecommendationWarn,
			Title:    "RAM frequently elevated — consider more memory",
			Detail: fmt.Sprintf(
				"RAM exceeded %.0f%% for %.0f%% of the window (p95 %.1f%%). "+
					"Consider increasing RAM allocation.",
				RAMWarnPct, s.TimeAboveWarn*100, s.P95,
			),
		}}
	case s.Avg < 30.0 && s.P95 < 50.0 && s.SampleCount >= 120:
		return []Recommendation{{
			Resource: "ram",
			Level:    RecommendationInfo,
			Title:    "RAM under-utilised",
			Detail: fmt.Sprintf(
				"avg %.1f%%, p95 %.1f%% — current allocation may exceed actual needs.",
				s.Avg, s.P95,
			),
		}}
	default:
		return []Recommendation{{
			Resource: "ram",
			Level:    RecommendationInfo,
			Title:    "RAM healthy",
			Detail:   fmt.Sprintf("avg %.1f%%, p95 %.1f%%, peak %.1f%%", s.Avg, s.P95, s.Peak),
		}}
	}
}

func diskRecommendations(s ResourceStats) []Recommendation {
	if s.SampleCount == 0 {
		return nil
	}
	switch {
	case s.Peak >= DiskCritPct:
		return []Recommendation{{
			Resource: "disk",
			Level:    RecommendationCritical,
			Title:    "Disk space critical — expand now",
			Detail: fmt.Sprintf(
				"Disk reached %.1f%% (critical threshold %.0f%%). "+
					"Expand disk or clean up data immediately.",
				s.Peak, DiskCritPct,
			),
		}}
	case s.DaysToCrit > 0 && s.DaysToCrit <= 14:
		return []Recommendation{{
			Resource: "disk",
			Level:    RecommendationCritical,
			Title:    fmt.Sprintf("Disk critical in ~%.0f days — plan expansion", s.DaysToCrit),
			Detail: fmt.Sprintf(
				"At current growth rate, disk will reach %.0f%% in %.0f days. "+
					"Expand disk storage now.",
				DiskCritPct, s.DaysToCrit,
			),
		}}
	case s.DaysToWarn > 0 && s.DaysToWarn <= 30:
		return []Recommendation{{
			Resource: "disk",
			Level:    RecommendationWarn,
			Title:    fmt.Sprintf("Disk warning in ~%.0f days", s.DaysToWarn),
			Detail: fmt.Sprintf(
				"Current usage %.1f%%, trending toward %.0f%% warning in %.0f days. "+
					"Schedule disk expansion.",
				s.Avg, DiskWarnPct, s.DaysToWarn,
			),
		}}
	case s.DaysToCrit > 0 && s.DaysToCrit <= 90:
		return []Recommendation{{
			Resource: "disk",
			Level:    RecommendationInfo,
			Title:    fmt.Sprintf("Disk on track — critical in ~%.0f days", s.DaysToCrit),
			Detail: fmt.Sprintf(
				"Disk growing steadily at %.1f%% avg. Plan expansion within 3 months.",
				s.Avg,
			),
		}}
	default:
		return []Recommendation{{
			Resource: "disk",
			Level:    RecommendationInfo,
			Title:    "Disk healthy",
			Detail:   fmt.Sprintf("current %.1f%%, no concerning growth trend", s.Avg),
		}}
	}
}

func swapRecommendations(s ResourceStats) []Recommendation {
	if s.SampleCount == 0 || s.Peak < SwapWarnPct {
		return nil
	}
	switch {
	case s.TimeAboveCrit >= 0.10:
		return []Recommendation{{
			Resource: "swap",
			Level:    RecommendationCritical,
			Title:    "Heavy swap usage — system is memory-starved",
			Detail: fmt.Sprintf(
				"Swap above %.0f%% for %.0f%% of the window. Add RAM to eliminate swap pressure.",
				SwapCritPct, s.TimeAboveCrit*100,
			),
		}}
	case s.TimeAboveWarn >= 0.15:
		return []Recommendation{{
			Resource: "swap",
			Level:    RecommendationWarn,
			Title:    "Elevated swap usage",
			Detail: fmt.Sprintf(
				"Swap exceeded %.0f%% for %.0f%% of the window (peak %.1f%%). "+
					"Consider increasing RAM allocation.",
				SwapWarnPct, s.TimeAboveWarn*100, s.Peak,
			),
		}}
	}
	return nil
}

// linearRegression returns slope and intercept for y = slope·x + intercept.
func linearRegression(x, y []float64) (slope, intercept float64) {
	n := float64(len(x))
	if n < 2 {
		return 0, 0
	}
	var sumX, sumY, sumXY, sumX2 float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
	}
	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}
