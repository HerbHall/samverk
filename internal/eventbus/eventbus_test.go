package eventbus

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	bus := New(16)
	defer bus.Close()

	ch := bus.Subscribe()
	ev := Event{Type: IssueOpened, Project: "test/repo", IssueNumber: 42, Timestamp: time.Now()}
	bus.Publish(ev)

	select {
	case got := <-ch:
		if got.Type != IssueOpened {
			t.Errorf("type = %q, want %q", got.Type, IssueOpened)
		}
		if got.IssueNumber != 42 {
			t.Errorf("issue = %d, want 42", got.IssueNumber)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeFiltered(t *testing.T) {
	bus := New(16)
	defer bus.Close()

	ch := bus.Subscribe(IssueClosed)

	bus.Publish(Event{Type: IssueOpened, IssueNumber: 1})
	bus.Publish(Event{Type: IssueClosed, IssueNumber: 2})

	select {
	case got := <-ch:
		if got.IssueNumber != 2 {
			t.Errorf("issue = %d, want 2 (only closed events)", got.IssueNumber)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}

	// Channel should be empty (opened event was filtered out).
	select {
	case extra := <-ch:
		t.Errorf("unexpected extra event: %+v", extra)
	default:
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := New(16)
	defer bus.Close()

	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe(IssueLabeled)

	bus.Publish(Event{Type: IssueLabeled, IssueNumber: 5, Key: "qc:pass"})

	// Both should receive the labeled event.
	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.IssueNumber != 5 {
				t.Errorf("issue = %d, want 5", got.IssueNumber)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event on subscriber")
		}
	}
}

func TestFullBufferDropsEvent(t *testing.T) {
	bus := New(1) // buffer size 1
	defer bus.Close()

	ch := bus.Subscribe()

	// Fill the buffer.
	bus.Publish(Event{Type: IssueOpened, IssueNumber: 1})
	// This should be dropped (buffer full), not block.
	bus.Publish(Event{Type: IssueOpened, IssueNumber: 2})

	got := <-ch
	if got.IssueNumber != 1 {
		t.Errorf("issue = %d, want 1", got.IssueNumber)
	}

	// Channel should be empty now (second event was dropped).
	select {
	case extra := <-ch:
		t.Errorf("expected dropped event, got: %+v", extra)
	default:
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := New(16)
	defer bus.Close()

	ch := bus.Subscribe()
	bus.Unsubscribe(ch)

	// Channel should be closed after unsubscribe.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Unsubscribe")
	}
}

func TestCloseStopsPublish(t *testing.T) {
	bus := New(16)
	ch := bus.Subscribe()
	bus.Close()

	// Publish after close should not panic.
	bus.Publish(Event{Type: IssueOpened, IssueNumber: 99})

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after bus.Close()")
	}
}
