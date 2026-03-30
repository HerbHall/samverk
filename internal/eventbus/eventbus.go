// Package eventbus provides lightweight in-process pub/sub for dispatch events.
// Subscribers receive events on buffered channels; slow subscribers drop events
// rather than blocking publishers.
package eventbus

import (
	"sync"
	"time"
)

// EventType identifies the kind of event.
type EventType string

const (
	IssueOpened    EventType = "issue.opened"
	IssueClosed    EventType = "issue.closed"
	IssueLabeled   EventType = "issue.labeled"
	IssueAssigned  EventType = "issue.assigned"
	IssueCommented EventType = "issue.commented"
	IssueEdited    EventType = "issue.edited"
)

// Event carries data about a single state change.
type Event struct {
	Type        EventType
	Project     string
	IssueNumber int
	Key         string // label name for labeled events, field name for edited
	Value       string // new value (empty for removals)
	OldValue    string // previous value (empty for additions)
	Source      string // originator: "watch", "dispatcher", "prwatcher"
	Timestamp   time.Time
}

// Bus is a lightweight in-process event bus. Zero value is not usable;
// create with New.
type Bus struct {
	mu      sync.RWMutex
	subs    []subscription
	bufSize int
	closed  bool
}

type subscription struct {
	ch    chan Event
	types map[EventType]bool // nil means all types
}

// New creates an event bus with the given subscriber channel buffer size.
func New(bufSize int) *Bus {
	if bufSize < 1 {
		bufSize = 64
	}
	return &Bus{bufSize: bufSize}
}

// Subscribe returns a channel that receives events of the specified types.
// Pass zero types to receive all events.
func (b *Bus) Subscribe(types ...EventType) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, b.bufSize)
	var typeFilter map[EventType]bool
	if len(types) > 0 {
		typeFilter = make(map[EventType]bool, len(types))
		for _, t := range types {
			typeFilter[t] = true
		}
	}
	b.subs = append(b.subs, subscription{ch: ch, types: typeFilter})
	return ch
}

// Unsubscribe removes and closes the given channel. No-op if not found.
func (b *Bus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.subs {
		if s.ch == ch {
			close(s.ch)
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

// Publish sends an event to all matching subscribers. Non-blocking: if a
// subscriber's channel is full the event is dropped for that subscriber.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, s := range b.subs {
		if s.types != nil && !s.types[ev.Type] {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// Subscriber channel full -- drop event to avoid blocking.
		}
	}
}

// Close closes all subscriber channels. After Close, Publish is a no-op.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for _, s := range b.subs {
		close(s.ch)
	}
	b.subs = nil
}
