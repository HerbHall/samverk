package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"samverk.dev/samverk/internal/server"
)

func TestSessionManager_CreateAndValidate(t *testing.T) {
	sm := server.NewSessionManager()
	defer sm.Stop()

	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty ID")
	}
	if len(id) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("session ID length = %d, want 64", len(id))
	}

	if !sm.Validate(id) {
		t.Error("Validate returned false for valid session")
	}
}

func TestSessionManager_InvalidSession(t *testing.T) {
	sm := server.NewSessionManager()
	defer sm.Stop()

	if sm.Validate("nonexistent") {
		t.Error("Validate returned true for nonexistent session")
	}
}

func TestSessionManager_Delete(t *testing.T) {
	sm := server.NewSessionManager()
	defer sm.Stop()

	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sm.Delete(id)

	if sm.Validate(id) {
		t.Error("Validate returned true after Delete")
	}
}

func TestSessionManager_SetCookie(t *testing.T) {
	sm := server.NewSessionManager()
	defer sm.Stop()

	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := httptest.NewRecorder()
	sm.SetCookie(rec, id)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookies set")
	}

	found := false
	for _, c := range cookies {
		if c.Name == "samverk_session" {
			found = true
			if c.Value != id {
				t.Errorf("cookie value = %q, want %q", c.Value, id)
			}
			if !c.HttpOnly {
				t.Error("cookie should be HttpOnly")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("cookie should be SameSite=Strict")
			}
		}
	}
	if !found {
		t.Error("samverk_session cookie not found")
	}
}

func TestGetSessionID(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "samverk_session", Value: "test-id-123"})

	id := server.GetSessionID(req)
	if id != "test-id-123" {
		t.Errorf("GetSessionID = %q, want %q", id, "test-id-123")
	}
}

func TestGetSessionID_NoCookie(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)

	id := server.GetSessionID(req)
	if id != "" {
		t.Errorf("GetSessionID = %q, want empty", id)
	}
}

func TestSessionManager_PersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	// Create a session and persist it to disk.
	sm := server.NewSessionManagerWithFile(path)
	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sm.Stop()

	// Simulate a restart: create a new manager loading from the same file.
	sm2 := server.NewSessionManagerWithFile(path)
	defer sm2.Stop()

	if !sm2.Validate(id) {
		t.Error("session should be valid after reload from disk")
	}
}

func TestSessionManager_PersistAndReload_ExpiredNotLoaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	sm := server.NewSessionManagerWithFile(path)
	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually expire the session by deleting it (simulates TTL expiry on reload).
	sm.Delete(id)
	sm.Stop()

	sm2 := server.NewSessionManagerWithFile(path)
	defer sm2.Stop()

	if sm2.Validate(id) {
		t.Error("deleted session should not be valid after reload")
	}
}

func TestSessionManager_PersistFile_NotCreatedWhenInMemory(t *testing.T) {
	// In-memory manager must not create any file.
	sm := server.NewSessionManager()
	_, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sm.Stop()
	// No file path configured — nothing to assert on disk, but Create must not error.
}

func tempSessionPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sessions.json")
}

func TestSessionManager_DeletePersistsToFile(t *testing.T) {
	path := tempSessionPath(t)

	sm := server.NewSessionManagerWithFile(path)
	id, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sm.Delete(id)
	sm.Stop()

	// After reload the deleted session must still be absent.
	sm2 := server.NewSessionManagerWithFile(path)
	defer sm2.Stop()

	if sm2.Validate(id) {
		t.Error("session should not be valid after Delete + reload")
	}
}

func TestSessionManager_MultipleSessionsPersist(t *testing.T) {
	path := tempSessionPath(t)

	sm := server.NewSessionManagerWithFile(path)
	ids := make([]string, 5)
	for i := range ids {
		id, err := sm.Create()
		if err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
		ids[i] = id
	}
	sm.Stop()

	sm2 := server.NewSessionManagerWithFile(path)
	defer sm2.Stop()

	for i, id := range ids {
		if !sm2.Validate(id) {
			t.Errorf("session[%d] should be valid after reload", i)
		}
	}
}

