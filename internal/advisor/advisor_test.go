package advisor

import (
	"testing"
	"time"

	"samverk.dev/samverk/pkg/models"
)

func ptr[T any](v T) *T { return &v }

// ── detectRecurrence ─────────────────────────────────────────────────────────

func TestDetectRecurrence(t *testing.T) {
	tests := []struct {
		name      string
		events    []*models.FailureEvent
		wantCount int
		wantLevel string
	}{
		{
			name:      "no events",
			events:    nil,
			wantCount: 0,
		},
		{
			name: "category under threshold",
			events: []*models.FailureEvent{
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
			},
			wantCount: 0,
		},
		{
			name: "category exceeds threshold",
			events: []*models.FailureEvent{
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
			},
			wantCount: 1,
			wantLevel: "warn",
		},
		{
			name: "multiple categories, only one exceeds",
			events: []*models.FailureEvent{
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "planning_gap"},
				{RootCauseCategory: "tool_error"},
				{RootCauseCategory: "tool_error"},
			},
			wantCount: 1,
			wantLevel: "warn",
		},
		{
			name: "empty category ignored",
			events: []*models.FailureEvent{
				{RootCauseCategory: ""},
				{RootCauseCategory: ""},
				{RootCauseCategory: ""},
				{RootCauseCategory: ""},
				{RootCauseCategory: ""},
			},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs := detectRecurrence(tc.events)
			if len(recs) != tc.wantCount {
				t.Errorf("got %d recommendations, want %d", len(recs), tc.wantCount)
			}
			for i := range recs {
				if recs[i].Level != tc.wantLevel {
					t.Errorf("rec[%d].Level = %q, want %q", i, recs[i].Level, tc.wantLevel)
				}
			}
		})
	}
}

// ── detectStagnantWorkarounds ────────────────────────────────────────────────

func TestDetectStagnantWorkarounds(t *testing.T) {
	stale := time.Now().UTC().Add(-20 * 24 * time.Hour) // 20 days ago
	recent := time.Now().UTC().Add(-5 * 24 * time.Hour) // 5 days ago

	tests := []struct {
		name      string
		events    []*models.FailureEvent
		wantCount int
	}{
		{
			name:      "empty",
			events:    nil,
			wantCount: 0,
		},
		{
			name: "workaround not yet resolved",
			events: []*models.FailureEvent{
				{IssueNumber: 1, FixClassification: "workaround", ResolvedAt: nil},
			},
			wantCount: 0,
		},
		{
			name: "workaround resolved recently (within 14d)",
			events: []*models.FailureEvent{
				{IssueNumber: 2, FixClassification: "workaround", ResolvedAt: ptr(recent)},
			},
			wantCount: 0,
		},
		{
			name: "workaround resolved stale, no prevention",
			events: []*models.FailureEvent{
				{IssueNumber: 3, FixClassification: "workaround", ResolvedAt: ptr(stale), PreventionMeasure: ""},
			},
			wantCount: 1,
		},
		{
			name: "workaround resolved stale, has prevention",
			events: []*models.FailureEvent{
				{IssueNumber: 4, FixClassification: "workaround", ResolvedAt: ptr(stale), PreventionMeasure: "added_test"},
			},
			wantCount: 0,
		},
		{
			name: "workaround resolved stale, prevention=none",
			events: []*models.FailureEvent{
				{IssueNumber: 5, FixClassification: "workaround", ResolvedAt: ptr(stale), PreventionMeasure: "none"},
			},
			wantCount: 1,
		},
		{
			name: "non-workaround classification ignored",
			events: []*models.FailureEvent{
				{IssueNumber: 6, FixClassification: "permanent_fix", ResolvedAt: ptr(stale)},
			},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs := detectStagnantWorkarounds(tc.events)
			if len(recs) != tc.wantCount {
				t.Errorf("got %d recommendations, want %d", len(recs), tc.wantCount)
			}
			for i := range recs {
				if recs[i].Level != "warn" {
					t.Errorf("rec[%d].Level = %q, want warn", i, recs[i].Level)
				}
			}
		})
	}
}

// ── detectUnknownRootCause ───────────────────────────────────────────────────

func TestDetectUnknownRootCause(t *testing.T) {
	tests := []struct {
		name      string
		events    []*models.FailureEvent
		wantCount int
	}{
		{
			name:      "fewer than 5 events",
			events:    []*models.FailureEvent{{}, {}, {}, {}},
			wantCount: 0,
		},
		{
			name: "5 events all with root cause",
			events: []*models.FailureEvent{
				{RootCauseCategory: "a"}, {RootCauseCategory: "b"},
				{RootCauseCategory: "c"}, {RootCauseCategory: "d"},
				{RootCauseCategory: "e"},
			},
			wantCount: 0,
		},
		{
			name: "exactly 20% unknown (at threshold, no rec)",
			events: []*models.FailureEvent{
				{RootCauseCategory: ""}, {RootCauseCategory: "a"},
				{RootCauseCategory: "b"}, {RootCauseCategory: "c"},
				{RootCauseCategory: "d"},
			},
			wantCount: 0,
		},
		{
			name: "over 20% unknown triggers rec",
			events: []*models.FailureEvent{
				{RootCauseCategory: ""}, {RootCauseCategory: ""},
				{RootCauseCategory: "a"}, {RootCauseCategory: "b"},
				{RootCauseCategory: "c"},
			},
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs := detectUnknownRootCause(tc.events)
			if len(recs) != tc.wantCount {
				t.Errorf("got %d recommendations, want %d", len(recs), tc.wantCount)
			}
			for i := range recs {
				if recs[i].Level != "info" {
					t.Errorf("rec[%d].Level = %q, want info", i, recs[i].Level)
				}
			}
		})
	}
}

// ── detectPreventionGaps ─────────────────────────────────────────────────────

func TestDetectPreventionGaps(t *testing.T) {
	tests := []struct {
		name      string
		events    []*models.FailureEvent
		wantCount int
	}{
		{
			name:      "empty",
			events:    nil,
			wantCount: 0,
		},
		{
			name: "category with <3 events ignored",
			events: []*models.FailureEvent{
				{RootCauseCategory: "auth", PreventionMeasure: ""},
				{RootCauseCategory: "auth", PreventionMeasure: ""},
			},
			wantCount: 0,
		},
		{
			name: "3 events, all no prevention (>50%)",
			events: []*models.FailureEvent{
				{RootCauseCategory: "auth", PreventionMeasure: ""},
				{RootCauseCategory: "auth", PreventionMeasure: ""},
				{RootCauseCategory: "auth", PreventionMeasure: ""},
			},
			wantCount: 1,
		},
		{
			name: "4 events, exactly 50% no prevention (not >50%, no rec)",
			events: []*models.FailureEvent{
				{RootCauseCategory: "auth", PreventionMeasure: ""},
				{RootCauseCategory: "auth", PreventionMeasure: ""},
				{RootCauseCategory: "auth", PreventionMeasure: "added_test"},
				{RootCauseCategory: "auth", PreventionMeasure: "added_test"},
			},
			wantCount: 0,
		},
		{
			name: "empty category skipped",
			events: []*models.FailureEvent{
				{RootCauseCategory: "", PreventionMeasure: ""},
				{RootCauseCategory: "", PreventionMeasure: ""},
				{RootCauseCategory: "", PreventionMeasure: ""},
			},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs := detectPreventionGaps(tc.events)
			if len(recs) != tc.wantCount {
				t.Errorf("got %d recommendations, want %d", len(recs), tc.wantCount)
			}
			for i := range recs {
				if recs[i].Level != "info" {
					t.Errorf("rec[%d].Level = %q, want info", i, recs[i].Level)
				}
			}
		})
	}
}

// ── detectKPIRegression ──────────────────────────────────────────────────────

func TestDetectKPIRegression(t *testing.T) {
	tests := []struct {
		name      string
		report    *models.KPIReport
		wantCount int
	}{
		{
			name:      "nil report",
			report:    nil,
			wantCount: 0,
		},
		{
			name:      "no rate data",
			report:    &models.KPIReport{SampleSize: 10},
			wantCount: 0,
		},
		{
			name: "no regression (7d close to 30d)",
			report: &models.KPIReport{
				SampleSize:       10,
				FirstTimeFixRate: ptr(85.0),
				FixSuccessRate:   ptr(80.0),
				RecurrenceRate:   ptr(5.0),
			},
			wantCount: 0,
		},
		{
			name: "FTFR drops >10pp triggers regression",
			report: &models.KPIReport{
				SampleSize:       10,
				FirstTimeFixRate: ptr(85.0),
				FixSuccessRate:   ptr(70.0),
				RecurrenceRate:   ptr(5.0),
			},
			wantCount: 1,
		},
		{
			name: "recurrence >15% triggers warning",
			report: &models.KPIReport{
				SampleSize:       10,
				FirstTimeFixRate: ptr(80.0),
				FixSuccessRate:   ptr(75.0),
				RecurrenceRate:   ptr(20.0),
			},
			wantCount: 1,
		},
		{
			name: "both regressions fire",
			report: &models.KPIReport{
				SampleSize:       10,
				FirstTimeFixRate: ptr(85.0),
				FixSuccessRate:   ptr(70.0),
				RecurrenceRate:   ptr(20.0),
			},
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs := detectKPIRegression(tc.report)
			if len(recs) != tc.wantCount {
				t.Errorf("got %d recommendations, want %d", len(recs), tc.wantCount)
			}
			for i := range recs {
				if recs[i].Level != "warn" {
					t.Errorf("rec[%d].Level = %q, want warn", i, recs[i].Level)
				}
			}
		})
	}
}
