package scaling

import (
	"testing"
	"time"

	"github.com/herbhall/samverk/pkg/models"
)

func makeEvent(action string, from, to int) models.ScalingEvent {
	return models.ScalingEvent{
		ID:          action,
		Timestamp:   time.Now(),
		Action:      action,
		FromWorkers: from,
		ToWorkers:   to,
		Reason:      "test",
		Confidence:  0.9,
	}
}

func TestEventBuffer_DefaultCapacity(t *testing.T) {
	b := NewEventBuffer(0)
	if b.maxLen != 100 {
		t.Fatalf("expected default maxLen 100, got %d", b.maxLen)
	}
}

func TestEventBuffer_AddAndEvents(t *testing.T) {
	b := NewEventBuffer(5)

	e1 := makeEvent("scale-up", 2, 3)
	e2 := makeEvent("scale-down", 3, 2)
	b.Add(e1)
	b.Add(e2)

	events := b.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Events() returns newest-first: e2 should be first.
	if events[0].Action != "scale-down" {
		t.Errorf("expected newest-first order, got %q first", events[0].Action)
	}
}

func TestEventBuffer_Empty(t *testing.T) {
	b := NewEventBuffer(10)
	if got := b.Events(); got != nil {
		t.Errorf("expected nil for empty buffer, got %v", got)
	}
}

func TestEventBuffer_Eviction(t *testing.T) {
	b := NewEventBuffer(3)
	for i := range 5 {
		b.Add(models.ScalingEvent{ID: string(rune('A' + i)), Action: "scale-up"})
	}
	events := b.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events after eviction, got %d", len(events))
	}
	// Oldest (A, B) should be evicted; newest (C, D, E) remain.
	// Events() is newest-first, so events[0].ID == "E".
	if events[0].ID != "E" {
		t.Errorf("expected newest event ID=E, got %q", events[0].ID)
	}
	if events[2].ID != "C" {
		t.Errorf("expected oldest remaining ID=C, got %q", events[2].ID)
	}
}

func TestEventBuffer_ConcurrentSafe(t *testing.T) {
	b := NewEventBuffer(100)
	done := make(chan struct{})
	go func() {
		for i := range 50 {
			b.Add(models.ScalingEvent{ID: string(rune('a' + i%26))})
		}
		close(done)
	}()
	for range 50 {
		_ = b.Events()
	}
	<-done
}
