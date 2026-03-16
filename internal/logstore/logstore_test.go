package logstore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *LogStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-logs.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestInsertAndQuery(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < 5; i++ {
		entry := &LogEntry{
			Timestamp:   now.Add(time.Duration(i) * time.Second),
			Level:       "info",
			Message:     "test message",
			Component:   "test",
			SessionID:   "sess_1",
			IssueNumber: 42,
			Fields:      `{"key":"value"}`,
		}
		if err := s.Insert(ctx, entry); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	entries, err := s.Query(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}

	// Verify DESC order: newest first.
	if entries[0].Timestamp.Before(entries[4].Timestamp) {
		t.Error("expected newest entry first")
	}
}

func TestQueryFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	entries := []LogEntry{
		{Timestamp: now, Level: "info", Message: "hello world", Component: "api", SessionID: "s1", IssueNumber: 1},
		{Timestamp: now.Add(time.Second), Level: "error", Message: "bad thing", Component: "agent", SessionID: "s2", IssueNumber: 2},
		{Timestamp: now.Add(2 * time.Second), Level: "info", Message: "another info", Component: "api", SessionID: "s1", IssueNumber: 1},
		{Timestamp: now.Add(3 * time.Second), Level: "warn", Message: "warning msg", Component: "dispatch", SessionID: "s3", IssueNumber: 3},
		{Timestamp: now.Add(4 * time.Second), Level: "info", Message: "final entry", Component: "agent", SessionID: "s2", IssueNumber: 2},
	}
	for i := range entries {
		if err := s.Insert(ctx, &entries[i]); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	tests := []struct {
		name  string
		f     QueryFilter
		count int
	}{
		{"by level", QueryFilter{Level: "info"}, 3},
		{"by component", QueryFilter{Component: "api"}, 2},
		{"by session", QueryFilter{SessionID: "s2"}, 2},
		{"by issue", QueryFilter{Issue: 1}, 2},
		{"by search", QueryFilter{Search: "bad"}, 1},
		{"by since", QueryFilter{Since: now.Add(2 * time.Second)}, 3},
		{"by until", QueryFilter{Until: now.Add(time.Second)}, 2},
		{"by limit", QueryFilter{Limit: 2}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.Query(ctx, tt.f)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(result) != tt.count {
				t.Errorf("got %d entries, want %d", len(result), tt.count)
			}
		})
	}
}

func TestPrune(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC()

	for i := 0; i < 3; i++ {
		if err := s.Insert(ctx, &LogEntry{Timestamp: old, Level: "info", Message: "old"}); err != nil {
			t.Fatalf("Insert old: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.Insert(ctx, &LogEntry{Timestamp: recent, Level: "info", Message: "new"}); err != nil {
			t.Fatalf("Insert new: %v", err)
		}
	}

	deleted, err := s.Prune(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted %d, want 3", deleted)
	}

	remaining, err := s.Query(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("Query after prune: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("remaining %d, want 2", len(remaining))
	}
}

func TestSyncerParseJSON(t *testing.T) {
	s := newTestStore(t)
	var buf bytes.Buffer

	syncer := NewSyncer(&buf, s)

	logLine := `{"level":"info","ts":"2026-03-15T10:00:00.000Z","msg":"task completed","logger":"agent","session_id":"sess_123","issue_number":507,"extra_key":"extra_val"}` + "\n"

	n, err := syncer.Write([]byte(logLine))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(logLine) {
		t.Errorf("wrote %d bytes, want %d", n, len(logLine))
	}

	// Verify stdout received the data.
	if buf.String() != logLine {
		t.Errorf("stdout got %q, want %q", buf.String(), logLine)
	}

	// Verify SQLite received the parsed entry.
	entries, err := s.Query(context.Background(), QueryFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	e := entries[0]
	if e.Level != "info" {
		t.Errorf("level = %q, want %q", e.Level, "info")
	}
	if e.Message != "task completed" {
		t.Errorf("msg = %q, want %q", e.Message, "task completed")
	}
	if e.Component != "agent" {
		t.Errorf("component = %q, want %q", e.Component, "agent")
	}
	if e.SessionID != "sess_123" {
		t.Errorf("session_id = %q, want %q", e.SessionID, "sess_123")
	}
	if e.IssueNumber != 507 {
		t.Errorf("issue_number = %d, want %d", e.IssueNumber, 507)
	}

	// Verify extra fields are captured.
	var extra map[string]any
	if err := json.Unmarshal([]byte(e.Fields), &extra); err != nil {
		t.Fatalf("unmarshal fields: %v", err)
	}
	if extra["extra_key"] != "extra_val" {
		t.Errorf("extra_key = %v, want %q", extra["extra_key"], "extra_val")
	}
}

func TestSyncerNonJSON(t *testing.T) {
	s := newTestStore(t)
	var buf bytes.Buffer

	syncer := NewSyncer(&buf, s)

	plainText := "2026-03-15 10:00:00 INFO some colorized dev log\n"
	n, err := syncer.Write([]byte(plainText))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(plainText) {
		t.Errorf("wrote %d bytes, want %d", n, len(plainText))
	}

	// Verify stdout got the text.
	if buf.String() != plainText {
		t.Errorf("stdout got %q, want %q", buf.String(), plainText)
	}

	// Verify SQLite has no entries (non-JSON is skipped).
	entries, err := s.Query(context.Background(), QueryFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (non-JSON should be skipped)", len(entries))
	}
}

func TestQueryLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert 10 entries.
	for i := 0; i < 10; i++ {
		if err := s.Insert(ctx, &LogEntry{
			Timestamp: time.Now().UTC(),
			Level:     "info",
			Message:   "msg",
		}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	// Request limit > 1000: should be clamped.
	entries, err := s.Query(ctx, QueryFilter{Limit: 2000})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// All 10 returned since there are only 10 (1000 clamp doesn't reduce below actual count).
	if len(entries) != 10 {
		t.Errorf("got %d entries, want 10", len(entries))
	}

	// Request limit of 3.
	entries, err = s.Query(ctx, QueryFilter{Limit: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

func TestSyncerSync(t *testing.T) {
	s := newTestStore(t)

	// Sync with a regular writer (no-op).
	syncer := NewSyncer(&bytes.Buffer{}, s)
	if err := syncer.Sync(); err != nil {
		t.Errorf("Sync with buffer: %v", err)
	}

	// Sync with os.Stdout.
	syncer2 := NewSyncer(os.Stdout, s)
	if err := syncer2.Sync(); err != nil {
		t.Errorf("Sync with stdout: %v", err)
	}
}
