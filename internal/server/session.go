package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	sessionCookieName = "samverk_session"
	sessionIDBytes    = 32 // 32 bytes = 64 hex chars
	defaultMaxAge     = 30 * 24 * time.Hour // 30 days; persisted across restarts
	cleanupInterval   = 15 * time.Minute
)

// Session represents an authenticated browser session.
type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// sessionSnapshot is the on-disk format for session persistence.
type sessionSnapshot struct {
	Sessions []*Session `json:"sessions"`
}

// SessionManager stores browser sessions in memory with optional disk persistence.
// Acceptable for single-server deployment.
type SessionManager struct {
	mu             sync.RWMutex
	sessions        map[string]*Session
	maxAge          time.Duration
	file            string // optional path for persistence; empty = in-memory only
	done            chan struct{}
	secureCookies   bool   // whether to set the Secure flag on session cookies
}

// NewSessionManager creates an in-memory SessionManager and starts background cleanup.
func NewSessionManager() *SessionManager {
	return newSessionManager("")
}

// NewSessionManagerWithFile creates a SessionManager that persists sessions to disk.
// On creation, any previously saved valid sessions are loaded from the file.
func NewSessionManagerWithFile(path string) *SessionManager {
	return newSessionManager(path)
}

func newSessionManager(file string) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		maxAge:   defaultMaxAge,
		file:     file,
		done:     make(chan struct{}),
	}
	if file != "" {
		sm.load()
	}
	go sm.cleanupLoop()
	return sm
}

// Stop halts the background cleanup goroutine.
func (sm *SessionManager) Stop() {
	close(sm.done)
}

// Create generates a new session, persists it, and returns its ID.
func (sm *SessionManager) Create() (string, error) {
	raw := make([]byte, sessionIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)

	now := time.Now()
	sm.mu.Lock()
	sm.sessions[id] = &Session{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(sm.maxAge),
	}
	sm.mu.Unlock()

	sm.persist()
	return id, nil
}

// Validate checks whether a session ID is valid and not expired.
func (sm *SessionManager) Validate(id string) bool {
	sm.mu.RLock()
	sess, ok := sm.sessions[id]
	sm.mu.RUnlock()

	if !ok {
		return false
	}
	return time.Now().Before(sess.ExpiresAt)
}

// Delete removes a session (logout) and persists the change.
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
	sm.persist()
}

// persist writes all non-expired sessions to disk. No-op when no file is configured.
func (sm *SessionManager) persist() {
	if sm.file == "" {
		return
	}
	now := time.Now()
	sm.mu.RLock()
	active := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		if now.Before(s.ExpiresAt) {
			active = append(active, s)
		}
	}
	sm.mu.RUnlock()

	data, err := json.Marshal(sessionSnapshot{Sessions: active})
	if err != nil {
		return
	}
	_ = os.WriteFile(sm.file, data, 0o600)
}

// load reads sessions from disk, discarding any that have already expired.
func (sm *SessionManager) load() {
	data, err := os.ReadFile(sm.file)
	if err != nil {
		return // file doesn't exist yet
	}
	var snap sessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return
	}
	now := time.Now()
	sm.mu.Lock()
	for _, s := range snap.Sessions {
		if now.Before(s.ExpiresAt) {
			sm.sessions[s.ID] = s
		}
	}
	sm.mu.Unlock()
}

// SetCookie writes the session cookie to the response.
func (sm *SessionManager) SetCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   sm.secureCookies, // configurable; set false when behind Cloudflare Tunnel
		MaxAge:   int(sm.maxAge.Seconds()),
	})
}

// ClearCookie removes the session cookie from the browser.
func (sm *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// GetSessionID reads the session ID from the request cookie.
func GetSessionID(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// cleanupLoop periodically removes expired sessions.
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.done:
			return
		case <-ticker.C:
			sm.purgeExpired()
		}
	}
}

// purgeExpired removes all sessions that have passed their expiry.
func (sm *SessionManager) purgeExpired() {
	now := time.Now()
	sm.mu.Lock()
	for id, sess := range sm.sessions {
		if now.After(sess.ExpiresAt) {
			delete(sm.sessions, id)
		}
	}
	sm.mu.Unlock()
}
