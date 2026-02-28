package store

import (
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
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
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
