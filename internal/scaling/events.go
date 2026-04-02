package scaling

import (
	"context"
	"sync"

	"samverk.dev/samverk/pkg/models"
)

// ScalingEvent is an alias for the shared models type.
// All code within this package uses models.ScalingEvent so callers share one type.
type ScalingEvent = models.ScalingEvent

// EventPersister is an optional interface for durable event storage.
// store.SQLiteStore satisfies this interface when scaling_events are enabled.
type EventPersister interface {
	SaveScalingEvent(ctx context.Context, e models.ScalingEvent) error
}

// EventBuffer is a fixed-capacity ring buffer of ScalingEvents.
// Oldest events are dropped when the buffer is full.
// All methods are safe for concurrent use.
type EventBuffer struct {
	mu     sync.RWMutex
	events []models.ScalingEvent
	maxLen int
}

// NewEventBuffer creates an EventBuffer with the given maximum capacity.
// If maxLen <= 0, it defaults to 100.
func NewEventBuffer(maxLen int) *EventBuffer {
	if maxLen <= 0 {
		maxLen = 100
	}
	return &EventBuffer{
		events: make([]models.ScalingEvent, 0, maxLen),
		maxLen: maxLen,
	}
}

// Add appends an event to the buffer. If the buffer is at capacity the
// oldest event is evicted to make room.
func (b *EventBuffer) Add(e models.ScalingEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) >= b.maxLen {
		copy(b.events, b.events[1:])
		b.events[b.maxLen-1] = e
	} else {
		b.events = append(b.events, e)
	}
}

// Events returns a copy of all stored events in newest-first order.
func (b *EventBuffer) Events() []models.ScalingEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := len(b.events)
	if n == 0 {
		return nil
	}
	out := make([]models.ScalingEvent, n)
	for i, e := range b.events {
		out[n-1-i] = e // reverse: newest first
	}
	return out
}
