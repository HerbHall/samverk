package server

import (
	"crypto/subtle"
	"html"
	"net/http"
	"strings"
)

// loginPage is a minimal HTML login form served before the SPA loads.
// It is intentionally self-contained (no external CSS/JS) so it works
// without loading any SPA assets.
const loginPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Samverk - Login</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #0a0a0a; color: #e5e5e5;
    display: flex; align-items: center; justify-content: center;
    min-height: 100vh;
  }
  .card {
    background: #171717; border: 1px solid #262626; border-radius: 12px;
    padding: 2rem; width: 100%; max-width: 380px;
  }
  h1 { font-size: 1.5rem; margin-bottom: 0.5rem; color: #fafafa; }
  p { font-size: 0.875rem; color: #a3a3a3; margin-bottom: 1.5rem; }
  label { display: block; font-size: 0.875rem; margin-bottom: 0.5rem; color: #d4d4d4; }
  input[type="password"] {
    width: 100%; padding: 0.625rem 0.75rem; font-size: 0.875rem;
    background: #0a0a0a; border: 1px solid #404040; border-radius: 6px;
    color: #fafafa; outline: none; margin-bottom: 1rem;
  }
  input[type="password"]:focus { border-color: #525252; }
  button {
    width: 100%; padding: 0.625rem; font-size: 0.875rem; font-weight: 500;
    background: #fafafa; color: #0a0a0a; border: none; border-radius: 6px;
    cursor: pointer;
  }
  button:hover { background: #e5e5e5; }
  .error {
    background: #450a0a; border: 1px solid #7f1d1d; border-radius: 6px;
    padding: 0.75rem; margin-bottom: 1rem; font-size: 0.875rem; color: #fca5a5;
  }
</style>
</head>
<body>
<div class="card">
  <h1>Samverk</h1>
  <p>Enter your auth token to access the dashboard.</p>
  {{ERROR}}
  <form method="POST" action="/login">
    <label for="password">Auth Token</label>
    <input type="password" id="password" name="password" autocomplete="current-password" autofocus required>
    <button type="submit">Sign In</button>
  </form>
</div>
</body>
</html>`

// handleLoginPage renders the login form.
func (s *Server) handleLoginPage(w http.ResponseWriter, _ *http.Request) {
	s.renderLogin(w, "")
}

// handleLoginSubmit validates the password and creates a session.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096) //nolint:gomnd // 4096: login form is tiny; limit prevents memory exhaustion
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, "Invalid form submission.")
		return
	}

	password := r.FormValue("password")

	if !s.validatePassword(password) {
		w.WriteHeader(http.StatusUnauthorized)
		s.renderLogin(w, "Invalid auth token.")
		return
	}

	sessionID, err := s.sessions.Create()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.renderLogin(w, "Session creation failed. Please try again.")
		return
	}

	s.sessions.SetCookie(w, sessionID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout destroys the session and redirects to login.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if id := GetSessionID(r); id != "" {
		s.sessions.Delete(id)
	}
	s.sessions.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// validatePassword checks the submitted password against the configured
// auth token and the key store.
func (s *Server) validatePassword(password string) bool {
	if s.cfg.AuthToken != "" {
		if subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.AuthToken)) == 1 {
			return true
		}
	}
	if s.cfg.KeyStore != nil {
		if _, ok := s.cfg.KeyStore.Validate(password); ok {
			return true
		}
	}
	return false
}

// renderLogin writes the login page HTML with an optional error message.
func (s *Server) renderLogin(w http.ResponseWriter, errMsg string) {
	errorHTML := ""
	if errMsg != "" {
		errorHTML = `<div class="error">` + html.EscapeString(errMsg) + `</div>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Replace {{ERROR}} placeholder with the error div (or empty string).
	page := strings.Replace(loginPage, "{{ERROR}}", errorHTML, 1)
	_, _ = w.Write([]byte(page))
}
