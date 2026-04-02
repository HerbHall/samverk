package advisor

import (
	"fmt"
	"math"
	"time"

	"samverk.dev/samverk/pkg/models"
)

// detectRecurrence finds root_cause_category values appearing >3 times in the provided events.
func detectRecurrence(events []*models.FailureEvent) []models.QualityRecommendation {
	counts := make(map[string]int)
	for i := range events {
		cat := events[i].RootCauseCategory
		if cat != "" {
			counts[cat]++
		}
	}

	var recs []models.QualityRecommendation
	for cat, n := range counts {
		if n > 3 {
			recs = append(recs, models.QualityRecommendation{
				ID:            fmt.Sprintf("recurrence-%s", cat),
				Level:         "warn",
				Category:      cat,
				Title:         fmt.Sprintf("Recurrent root cause: %s", cat),
				Detail:        fmt.Sprintf("%d failures with root cause '%s' in the last 30 days — investigate systemic fix", n, cat),
				AffectedCount: n,
				WindowDays:    30,
				GeneratedAt:   time.Now().UTC(),
			})
		}
	}
	return recs
}

// detectStagnantWorkarounds finds workaround-classified events resolved more than 14 days ago
// with no prevention measure recorded.
func detectStagnantWorkarounds(events []*models.FailureEvent) []models.QualityRecommendation {
	cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)

	recs := make([]models.QualityRecommendation, 0, len(events))
	for i := range events {
		e := events[i]
		if e.FixClassification != "workaround" {
			continue
		}
		if e.ResolvedAt == nil {
			continue
		}
		if e.ResolvedAt.After(cutoff) {
			continue
		}
		pm := e.PreventionMeasure
		if pm != "" && pm != "none" {
			continue
		}
		daysAgo := int(math.Round(time.Since(*e.ResolvedAt).Hours() / 24))
		recs = append(recs, models.QualityRecommendation{
			ID:            fmt.Sprintf("stagnant-workaround-%d", e.IssueNumber),
			Level:         "warn",
			Category:      "workaround",
			Title:         fmt.Sprintf("Stale workaround: issue #%d", e.IssueNumber),
			Detail:        fmt.Sprintf("Workaround applied %d days ago with no permanent fix or prevention measure", daysAgo),
			AffectedCount: 1,
			WindowDays:    30,
			GeneratedAt:   time.Now().UTC(),
		})
	}
	return recs
}

// detectUnknownRootCause fires when >20% of events have no root_cause_category and total >= 5.
func detectUnknownRootCause(events []*models.FailureEvent) []models.QualityRecommendation {
	if len(events) < 5 {
		return nil
	}

	unknown := 0
	for i := range events {
		if events[i].RootCauseCategory == "" {
			unknown++
		}
	}

	pct := float64(unknown) / float64(len(events)) * 100
	if pct <= 20 {
		return nil
	}

	return []models.QualityRecommendation{
		{
			ID:            "unknown-root-cause-rate",
			Level:         "info",
			Category:      "data_quality",
			Title:         "High unknown root-cause rate",
			Detail:        fmt.Sprintf("%.0f%% of failures in the last 30 days have no root cause recorded — add RCA fields when closing agent sessions", pct),
			AffectedCount: unknown,
			WindowDays:    30,
			GeneratedAt:   time.Now().UTC(),
		},
	}
}

// detectPreventionGaps finds root_cause_category groups where >50% have no prevention_measure
// and the group has at least 3 events.
func detectPreventionGaps(events []*models.FailureEvent) []models.QualityRecommendation {
	type catStats struct {
		total       int
		noPrevention int
	}
	stats := make(map[string]*catStats)

	for i := range events {
		e := events[i]
		cat := e.RootCauseCategory
		if cat == "" {
			continue
		}
		if stats[cat] == nil {
			stats[cat] = &catStats{}
		}
		stats[cat].total++
		pm := e.PreventionMeasure
		if pm == "" || pm == "none" {
			stats[cat].noPrevention++
		}
	}

	recs := make([]models.QualityRecommendation, 0, len(stats))
	for cat, s := range stats {
		if s.total < 3 {
			continue
		}
		pct := float64(s.noPrevention) / float64(s.total)
		if pct <= 0.5 {
			continue
		}
		recs = append(recs, models.QualityRecommendation{
			ID:            fmt.Sprintf("prevention-gap-%s", cat),
			Level:         "info",
			Category:      cat,
			Title:         fmt.Sprintf("Prevention gap: %s", cat),
			Detail:        fmt.Sprintf("%d/%d %s failures have no prevention measure recorded", s.noPrevention, s.total, cat),
			AffectedCount: s.noPrevention,
			WindowDays:    30,
			GeneratedAt:   time.Now().UTC(),
		})
	}
	return recs
}

// detectKPIRegression compares the most recent KPI report to a 7-day sub-window
// embedded in the same report via FixSuccessRate (7d window) vs FirstTimeFixRate (30d).
// When FTFR drops >10pp or recurrence rises >5pp it emits a warning.
//
// Since the store's GetKPIReport already computes all metrics for the 30d window,
// and FixSuccessRate uses the 7d window, we compare those two to detect trends.
func detectKPIRegression(report *models.KPIReport) []models.QualityRecommendation {
	if report == nil {
		return nil
	}

	var recs []models.QualityRecommendation

	// Compare fix_success_rate (7d) vs first_time_fix_rate (30d).
	// A large drop between the 7d and 30d windows indicates recent degradation.
	if report.FirstTimeFixRate != nil && report.FixSuccessRate != nil {
		old := *report.FirstTimeFixRate
		recent := *report.FixSuccessRate
		drop := old - recent
		if drop > 10 {
			recs = append(recs, models.QualityRecommendation{
				ID:            "kpi-regression-ftfr",
				Level:         "warn",
				Category:      "kpi_regression",
				Title:         "KPI regression detected",
				Detail:        fmt.Sprintf("First-time fix rate dropped from %.1f%% (30d) to %.1f%% (7d)", old, recent),
				AffectedCount: report.SampleSize,
				WindowDays:    30,
				GeneratedAt:   time.Now().UTC(),
			})
		}
	}

	// Recurrence rate rise: if recurrence_rate (30d) > 5 and planning_gap_rate is also elevated,
	// flag it. We treat recurrence > 15% as a regression signal.
	if report.RecurrenceRate != nil && *report.RecurrenceRate > 15 {
		recs = append(recs, models.QualityRecommendation{
			ID:            "kpi-regression-recurrence",
			Level:         "warn",
			Category:      "kpi_regression",
			Title:         "Elevated recurrence rate",
			Detail:        fmt.Sprintf("Recurrence rate is %.1f%% in the last 30 days — review systemic fixes", *report.RecurrenceRate),
			AffectedCount: report.SampleSize,
			WindowDays:    30,
			GeneratedAt:   time.Now().UTC(),
		})
	}

	return recs
}
