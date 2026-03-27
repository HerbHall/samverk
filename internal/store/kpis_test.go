package store

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

// TestReadinessGapRate verifies that ReadinessGapRate correctly calculates the
// percentage of failures with readiness_gap root cause.
func TestReadinessGapRate(t *testing.T) {
	tests := []struct {
		name      string
		events    [][2]string // [root_cause_category, created_at_offset]
		days      int
		wantRate  float64
		wantError bool
	}{
		{
			name:     "no events",
			events:   [][2]string{},
			days:     7,
			wantRate: 0,
		},
		{
			name: "all readiness_gap",
			events: [][2]string{
				{"readiness_gap", "-1d"},
				{"readiness_gap", "-2d"},
				{"readiness_gap", "-3d"},
			},
			days:     7,
			wantRate: 100,
		},
		{
			name: "mixed categories",
			events: [][2]string{
				{"readiness_gap", "-1d"},
				{"planning_gap", "-2d"},
				{"readiness_gap", "-3d"},
				{"impact_conflict", "-4d"},
			},
			days:     7,
			wantRate: 50, // 2 out of 4
		},
		{
			name: "no readiness_gap in window",
			events: [][2]string{
				{"planning_gap", "-1d"},
				{"impact_conflict", "-2d"},
			},
			days:     7,
			wantRate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			base := time.Now().UTC().Truncate(time.Second)

			for _, ev := range tt.events {
				category, offsetStr := ev[0], ev[1]
				var offset time.Duration
				switch offsetStr {
				case "-1d":
					offset = -24 * time.Hour
				case "-2d":
					offset = -48 * time.Hour
				case "-3d":
					offset = -72 * time.Hour
				case "-4d":
					offset = -96 * time.Hour
				}

				e := makeFailureEvent(1, models.FailureClassUnknown, base.Add(offset))
				e.RootCauseCategory = category
				if err := s.SaveFailureEvent(ctx, e); err != nil {
					t.Fatalf("save failure event: %v", err)
				}
			}

			rate, err := s.ReadinessGapRate(ctx, tt.days)
			if (err != nil) != tt.wantError {
				t.Fatalf("ReadinessGapRate error = %v, want error: %v", err, tt.wantError)
			}
			if rate != tt.wantRate {
				t.Errorf("ReadinessGapRate = %f, want %f", rate, tt.wantRate)
			}
		})
	}
}

// TestPlanningEscalationRate verifies that PlanningEscalationRate correctly calculates
// the percentage of failures with planning_gap root cause.
func TestPlanningEscalationRate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	// Create 5 failure events: 2 planning_gap, 3 others
	categories := []string{"planning_gap", "planning_gap", "readiness_gap", "impact_conflict", "infrastructure"}
	for i, category := range categories {
		e := makeFailureEvent(i+1, models.FailureClassUnknown, base.Add(time.Duration(-i)*24*time.Hour))
		e.RootCauseCategory = category
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save failure event: %v", err)
		}
	}

	rate, err := s.PlanningEscalationRate(ctx, 7)
	if err != nil {
		t.Fatalf("PlanningEscalationRate: %v", err)
	}

	wantRate := 40.0 // 2 out of 5
	if rate != wantRate {
		t.Errorf("PlanningEscalationRate = %f, want %f", rate, wantRate)
	}
}

// TestImpactConflictRate verifies that ImpactConflictRate correctly calculates
// the percentage of failures with impact_conflict root cause.
func TestImpactConflictRate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	// Create 4 failure events: 1 impact_conflict, 3 others
	categories := []string{"impact_conflict", "readiness_gap", "planning_gap", "code_quality"}
	for i, category := range categories {
		e := makeFailureEvent(i+1, models.FailureClassUnknown, base.Add(time.Duration(-i)*24*time.Hour))
		e.RootCauseCategory = category
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save failure event: %v", err)
		}
	}

	rate, err := s.ImpactConflictRate(ctx, 7)
	if err != nil {
		t.Fatalf("ImpactConflictRate: %v", err)
	}

	wantRate := 25.0 // 1 out of 4
	if rate != wantRate {
		t.Errorf("ImpactConflictRate = %f, want %f", rate, wantRate)
	}
}

// TestKPIRates_WindowFilter verifies that rates only consider events within the
// specified day window.
func TestKPIRates_WindowFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	// Create events: 2 within 7 days, 1 outside
	categories := []string{"readiness_gap", "readiness_gap", "readiness_gap"}
	offsets := []time.Duration{-1 * 24 * time.Hour, -3 * 24 * time.Hour, -10 * 24 * time.Hour}

	for i, category := range categories {
		e := makeFailureEvent(i+1, models.FailureClassUnknown, base.Add(offsets[i]))
		e.RootCauseCategory = category
		if err := s.SaveFailureEvent(ctx, e); err != nil {
			t.Fatalf("save failure event: %v", err)
		}
	}

	// Query within 7 days should only see 2 readiness_gap events
	rate, err := s.ReadinessGapRate(ctx, 7)
	if err != nil {
		t.Fatalf("ReadinessGapRate(7): %v", err)
	}

	wantRate := 100.0 // 2 out of 2 within 7 days
	if rate != wantRate {
		t.Errorf("ReadinessGapRate(7) = %f, want %f", rate, wantRate)
	}

	// Query within 5 days should only see 1 readiness_gap event
	rate, err = s.ReadinessGapRate(ctx, 5)
	if err != nil {
		t.Fatalf("ReadinessGapRate(5): %v", err)
	}

	wantRate = 100.0 // 1 out of 1 within 5 days
	if rate != wantRate {
		t.Errorf("ReadinessGapRate(5) = %f, want %f", rate, wantRate)
	}
}
