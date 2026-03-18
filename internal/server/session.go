package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookieName = "samverk_session"
	sessionIDBytes    = 32 // 32 bytes = 64 hex chars
	defaultMaxAge     = 24 * time.Hour
	cleanupInterval   = 15 * time.Minute
)

// Session represents an authenticated browser session.
type Session struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionManager stores browser sessions in memory.
// Acceptable for single-server deployment.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	maxAge   time.Duration
	done     chan struct{}
}

// NewSessionManager creates a SessionManager and starts background cleanup.
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		maxAge:   defaultMaxAge,
		done:     make(chan struct{}),
	}
	go sm.cleanupLoop()
	return sm
}

// Stop halts the background cleanup goroutine.
func (sm *SessionManager) Stop() {
	close(sm.done)
}

// Create generates a new session and returns its ID.
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

// Delete removes a session (logout).
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
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
		Secure:   false, // Set by reverse proxy (Cloudflare Tunnel)
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
