package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewStore(t *testing.T) {
	s := newTestStore(t)

	// Verify both tables exist by querying sqlite_master.
	rows, err := s.db.QueryContext(context.Background(), "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	tables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables[name] = true
	}

	for _, want := range []string{"sessions", "cost_records"} {
		if !tables[want] {
			t.Errorf("expected table %q to exist, got tables: %v", want, tables)
		}
	}
}

func TestNewStoreClose(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
}

func TestBusyTimeoutEnabled(t *testing.T) {
	s := newTestStore(t)

	var timeout int
	err := s.db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout)
	if err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestConcurrentFileAccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "concurrent.db")

	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store 1: %v", err)
	}
	defer s1.Close()

	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("open store 2: %v", err)
	}
	defer s2.Close()

	// Both stores writing concurrently should not return SQLITE_BUSY
	// because busy_timeout gives the blocked writer time to retry.
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if _, err := s1.db.ExecContext(ctx, "INSERT INTO scaling_events (id, timestamp, action, from_workers, to_workers, reason, confidence) VALUES (?, ?, 'scale_up', 1, 2, 'test', 0.9)",
			generateID(), "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("store1 write %d: %v", i, err)
		}
		if _, err := s2.db.ExecContext(ctx, "INSERT INTO scaling_events (id, timestamp, action, from_workers, to_workers, reason, confidence) VALUES (?, ?, 'scale_up', 2, 3, 'test', 0.9)",
			generateID(), "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("store2 write %d: %v", i, err)
		}
	}
}

func TestMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open: creates tables.
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()

	// Verify the file was created.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}

	// Second open: migrations run again (IF NOT EXISTS), no error.
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	s2.Close()
}
