package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetLastCheckIn_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetLastCheckIn(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSetAndGetLastCheckIn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	want := time.Now().UTC().Truncate(time.Second)
	if err := s.SetLastCheckIn(ctx, want); err != nil {
		t.Fatalf("SetLastCheckIn: %v", err)
	}

	got, err := s.GetLastCheckIn(ctx)
	if err != nil {
		t.Fatalf("GetLastCheckIn: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSetLastCheckIn_Upsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	second := time.Now().UTC().Truncate(time.Second)

	if err := s.SetLastCheckIn(ctx, first); err != nil {
		t.Fatalf("first SetLastCheckIn: %v", err)
	}
	if err := s.SetLastCheckIn(ctx, second); err != nil {
		t.Fatalf("second SetLastCheckIn: %v", err)
	}

	got, err := s.GetLastCheckIn(ctx)
	if err != nil {
		t.Fatalf("GetLastCheckIn: %v", err)
	}
	if !got.Equal(second) {
		t.Errorf("got %v, want %v (second value)", got, second)
	}
}
