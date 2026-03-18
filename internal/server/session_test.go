package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herbhall/samverk/internal/server"
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
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "samverk_session", Value: "test-id-123"})

	id := server.GetSessionID(req)
	if id != "test-id-123" {
		t.Errorf("GetSessionID = %q, want %q", id, "test-id-123")
	}
}

func TestGetSessionID_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	id := server.GetSessionID(req)
	if id != "" {
		t.Errorf("GetSessionID = %q, want empty", id)
	}
}
