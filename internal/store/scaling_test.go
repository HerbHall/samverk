package store

import (
	"context"
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

func TestSaveAndListScalingEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e1 := models.ScalingEvent{
		Timestamp:   time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Second),
		Action:      "scale-up",
		FromWorkers: 2,
		ToWorkers:   3,
		Reason:      "queue full",
		Confidence:  0.9,
	}
	e2 := models.ScalingEvent{
		Timestamp:   time.Now().UTC().Truncate(time.Second),
		Action:      "scale-down",
		FromWorkers: 3,
		ToWorkers:   2,
		Reason:      "idle workers",
		Confidence:  0.75,
	}

	if err := s.SaveScalingEvent(ctx, e1); err != nil {
		t.Fatalf("save e1: %v", err)
	}
	if err := s.SaveScalingEvent(ctx, e2); err != nil {
		t.Fatalf("save e2: %v", err)
	}

	events, err := s.ListScalingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Newest-first: e2 first.
	if events[0].Action != "scale-down" {
		t.Errorf("expected scale-down first, got %q", events[0].Action)
	}
	if events[1].Action != "scale-up" {
		t.Errorf("expected scale-up second, got %q", events[1].Action)
	}
	if events[0].FromWorkers != 3 || events[0].ToWorkers != 2 {
		t.Errorf("unexpected worker counts: %d→%d", events[0].FromWorkers, events[0].ToWorkers)
	}
	if events[0].Confidence != 0.75 {
		t.Errorf("expected confidence 0.75, got %f", events[0].Confidence)
	}
}

func TestListScalingEvents_Empty(t *testing.T) {
	s := newTestStore(t)
	events, err := s.ListScalingEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("list on empty store: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestListScalingEvents_DefaultLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for range 5 {
		_ = s.SaveScalingEvent(ctx, models.ScalingEvent{
			Timestamp:  time.Now().UTC(),
			Action:     "scale-up",
			FromWorkers: 1,
			ToWorkers:  2,
			Reason:     "test",
			Confidence: 0.8,
		})
	}
	events, err := s.ListScalingEvents(ctx, 0) // 0 → defaults to 100
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events with default limit, got %d", len(events))
	}
}

func TestListScalingEvents_LimitEnforced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for range 10 {
		_ = s.SaveScalingEvent(ctx, models.ScalingEvent{
			Timestamp:  time.Now().UTC(),
			Action:     "scale-up",
			FromWorkers: 1,
			ToWorkers:  2,
			Reason:     "test",
			Confidence: 0.8,
		})
	}
	events, err := s.ListScalingEvents(ctx, 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit=3, got %d", len(events))
	}
}
