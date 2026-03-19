# Authentication Model

Samverk uses a two-tier auth model: session cookies for browser access and
Bearer tokens for programmatic API and MCP access. The two tiers are
completely independent — each endpoint accepts exactly one.

## Tiers

### Tier 1 — Session Cookies (browser / SPA)

Used by the web dashboard and login flow.

| Property | Value |
|----------|-------|
| Cookie name | `samverk_session` |
| Flags | `HttpOnly`, `SameSite=Strict` |
| TTL | 30 days from creation |
| Storage | In-memory + JSON file (persists across restarts) |
| Session file | Derived from `--db` path: `<db-stem>-sessions.json` |

**Login flow:**

1. `GET /login` — serves the HTML login form
2. `POST /login` — validates password, creates session, sets cookie, redirects to `/`
3. `POST /logout` — deletes session, clears cookie, redirects to `/login`

**What survives a restart:** Sessions are written to disk on every `Create` and
`Delete`. On startup, the session manager loads all non-expired sessions from
the file. Users do not need to re-login after a service restart.

**What expires:** Sessions expire after 30 days. The background cleanup goroutine
removes expired sessions from memory every 15 minutes. Expired sessions are
filtered out on load from disk.

### Tier 2 — Bearer Token (API / MCP / programmatic)

Used by REST API clients and the MCP protocol endpoint. Configured via
environment variable or the key store.

| Property | Value |
|----------|-------|
| Header | `Authorization: Bearer <token>` |
| Config | `SAMVERK_AUTH_TOKEN` env var (single shared token) |
| Key store | `--keys <path>` flag (YAML-backed API key store) |
| No-op mode | When neither is configured, auth is skipped entirely |

**Validation order:**

1. Check `Authorization` header is present and has the `Bearer` prefix
2. If `SAMVERK_AUTH_TOKEN` is set: compare with `crypto/subtle.ConstantTimeCompare` (constant-time)
3. If that fails and a key store is configured: check key store
4. If neither matches: `401 Unauthorized`

**Special case — empty token:** When `SAMVERK_AUTH_TOKEN` is empty and no key
store is configured, the `BearerAuth` middleware is a no-op and all requests
pass through. This is the default for local development.

## Endpoint Auth Map

| Path | Method | Auth Required | Type |
|------|--------|---------------|------|
| `/login` | GET, POST | None | — |
| `/logout` | POST | None | — |
| `/healthz` | GET | None | — |
| `/.well-known/` | GET | None | — (returns 404) |
| `/mcp` | GET, POST, DELETE | Bearer token | Tier 2 |
| `/connect` | GET, POST, DELETE | Bearer token | Tier 2 |
| `/api/` | All | Bearer token OR session | Tier 1 or 2 |
| `/` (SPA) | GET | Session cookie | Tier 1 |

## Configuration

```bash
# Enable auth (both tiers active)
SAMVERK_AUTH_TOKEN=my-secret-token samverk serve

# Key store for multiple API clients (Bearer tier)
samverk serve --keys /path/to/keys.yaml

# Session file path (auto-derived from --db)
samverk serve --db /var/lib/samverk/samverk.db
# → sessions written to /var/lib/samverk/samverk-sessions.json

# No auth (local dev)
samverk serve
```

## Session Manager — Internal Contract

`SessionManager` is safe for concurrent use. Methods:

- `Create() (string, error)` — generates a 32-byte random ID, stores session,
  persists to file if configured
- `Validate(id string) bool` — returns true if session exists and is not expired
- `Delete(id string)` — removes session, persists to file if configured
- `SetCookie(w, id)` — writes the session cookie to the response
- `ClearCookie(w)` — overwrites the cookie with a zero-TTL value
- `Stop()` — cancels the cleanup goroutine; call in tests and on shutdown

`NewSessionManager()` — in-memory only (no file persistence)

`NewSessionManagerWithFile(path)` — persists to disk; loads existing sessions
on creation

## Related

- [ADR-020: Web dashboard](decisions/ADR-020-web-dashboard.md)
- [docs/architecture.md](architecture.md)
- `internal/server/session.go` — session manager implementation
- `internal/server/auth.go` — BearerAuth, BearerOrSessionAuth, requireSession
- `internal/server/login.go` — login/logout handlers
