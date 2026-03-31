# Credentials and Secrets Management

How Samverk handles API keys, tokens, and secrets across development and production environments.

## Architecture Overview

Samverk uses **environment variables** as the universal secret transport. Go code calls `os.Getenv()` to read credentials at runtime -- no secrets are ever committed to the repository.

```text
Development Machine (Windows)                Production Server (CT 202, Linux)
┌───────────────────────────┐                ┌────────────────────────────────┐
│  PowerShell SecretStore   │                │  /var/lib/samverk/.samverk/    │
│  (HomeLabVault)           │                │    samverk.env                 │
│         │                 │                │         │                      │
│         ▼                 │                │         ▼                      │
│  User-level env vars      │                │  systemd EnvironmentFile       │
│  (HKCU\Environment)       │                │         │                      │
│         │                 │                │         ▼                      │
│         ▼                 │                │  os.Getenv() in Go binary      │
│  All shells inherit       │                └────────────────────────────────┘
│  (PS, Git Bash, CMD, WSL) │
│         │                 │
│         ▼                 │
│  os.Getenv() in Go binary │
└───────────────────────────┘
```

## Environment Variables

### Required by Samverk

| Env Var | Purpose | Used By | Required On |
|---------|---------|---------|-------------|
| `GITHUB_TOKEN` | GitHub API access (issues, labels, PRs) | serve, dispatch, digest | Dev + Server |
| `SAMVERK_GITHUB_OWNER` | Default GitHub repo owner | serve, dispatch, digest | Dev + Server |
| `SAMVERK_GITHUB_REPO` | Default GitHub repo name | serve, dispatch, digest | Dev + Server |
| `SAMVERK_AUTH_TOKEN` | MCP Bearer token (simple single-token auth) | serve, scale CLI | Server |
| `GITEA_TOKEN` | Gitea API access (fallback if not in server.yaml) | serve (multi-project) | Server (if Gitea) |

### AI Provider Authentication

**The fleet uses OAuth (Max plan) for all Claude Code instances. API keys are NOT used.**

All Claude CLI instances authenticate via `claude login` (browser-based OAuth flow). The `claude-cli` provider type strips `ANTHROPIC_API_KEY` from subprocess environments as a safety net. Ollama providers use `base_url` from `providers.yaml` and require no API key.

API-based providers (`type: claude`, `type: openai`) are commented out in `providers.yaml.example`. If API keys are ever needed in the future, this will be an explicit decision documented in an ADR.

### Not Used by Samverk Directly

These exist on the development machine for other tools:

| Env Var | Purpose |
|---------|---------|
| `GITHUB_MCP_TOKEN` | GitHub MCP server (Claude Code) |
| `GITEA_DISPATCHER_TOKEN` | Gitea Actions dispatcher PAT |
| `CLOUDFLARE_API_TOKEN` | Caddy dns-proxy (infrastructure) |
| `HOME_ASSISTANT_TOKEN` | Home Assistant API |

## Where os.Getenv() Is Called

| File | Env Var | Purpose |
|------|---------|---------|
| [main.go:74](../cmd/samverk/main.go#L74) | `SAMVERK_AUTH_TOKEN` | MCP Bearer auth for serve |
| [main.go:110](../cmd/samverk/main.go#L110) | `GITHUB_TOKEN` | GitHub forge for serve |
| [main.go:112](../cmd/samverk/main.go#L112) | `SAMVERK_GITHUB_OWNER` | Repo owner for serve |
| [main.go:115](../cmd/samverk/main.go#L115) | `SAMVERK_GITHUB_REPO` | Repo name for serve |
| [main.go:166](../cmd/samverk/main.go#L166) | `GITEA_TOKEN` | Gitea forge fallback |
| [main.go:267](../cmd/samverk/main.go#L267) | `GITHUB_TOKEN` | GitHub forge for dispatch |
| [main.go:269](../cmd/samverk/main.go#L269) | `SAMVERK_GITHUB_OWNER` | Repo owner for dispatch |
| [main.go:272](../cmd/samverk/main.go#L272) | `SAMVERK_GITHUB_REPO` | Repo name for dispatch |
| [main.go:408](../cmd/samverk/main.go#L408) | `SAMVERK_AUTH_TOKEN` | Bearer auth for scale CLI |
| [main.go:536](../cmd/samverk/main.go#L536) | `GITHUB_TOKEN` | GitHub forge for digest |
| [main.go:747](../cmd/samverk/main.go#L747) | Dynamic (`cfg.APIKeyEnv`) | Provider API key (from providers.yaml) |
| [claudecli.go:89](../internal/provider/claudecli/claudecli.go#L89) | `ANTHROPIC_API_KEY` | Stripped to force Claude CLI OAuth |

## The Two auth.yaml Files

There are two files both named `auth.yaml` with **completely different purposes**:

### Developer auth.yaml (`~/.samverk/auth.yaml`)

- **Location:** `C:\Users\herbh\.samverk\auth.yaml` (dev machine)
- **Purpose:** Reference file with plaintext tokens for local development
- **Created by:** Credential management setup (PowerShell vault sync)
- **Format:** YAML with forge tokens, AI API keys, SSH hosts
- **Read by Samverk:** No -- Samverk reads `os.Getenv()`, not this file
- **Security:** File-level permissions only; gitignored

### Server auth.yaml (`/var/lib/samverk/.samverk/auth.yaml`)

- **Location:** `/var/lib/samverk/.samverk/auth.yaml` (CT 202)
- **Purpose:** MCP client API key store (hashed keys for device authentication)
- **Created by:** `samverk key create <name>` CLI command
- **Format:** YAML with SHA-256 hashed keys, names, project scopes, timestamps
- **Read by Samverk:** Yes -- the `KeyStore` in [apikey.go](../internal/server/apikey.go) loads it
- **Security:** 0600 permissions, hashes only (plaintext shown once at creation)

Do not confuse these files. The developer file is a convenience reference; the server file is the production authentication store.

## Provider Configuration

Providers are configured in `.samverk/providers.yaml` (see [providers.yaml.example](../.samverk/providers.yaml.example)). API keys are referenced by environment variable name, never stored directly:

```yaml
providers:
  claude-sonnet:
    type: claude-cli                   # Uses OAuth, not API keys
    default_model: claude-sonnet-4-6
```

The `claude-cli` provider type shells out to the `claude` binary which handles its own authentication via OAuth. API key-based providers are not used in the fleet.

## Server-Side Setup (CT 202)

### Environment File

The systemd services load secrets from `/var/lib/samverk/.samverk/samverk.env`:

```bash
# Copy from deploy/samverk.env.example and fill in real values
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
SAMVERK_GITHUB_OWNER=herbhall
SAMVERK_GITHUB_REPO=samverk
SAMVERK_AUTH_TOKEN=<generated-with-openssl-rand-hex-32>
# Claude Code uses OAuth (Max plan), not API keys.
# Run 'claude login' on each machine to authenticate.
```

File permissions must be `0600` owned by the `samverk` user.

### Systemd Units

Both services reference the environment file:

- **samverk-serve.service:** `EnvironmentFile=/var/lib/samverk/.samverk/samverk.env`
- **samverk-dispatch.service:** `EnvironmentFile=/var/lib/samverk/.samverk/samverk.env`

To update secrets on the server:

```bash
ssh samverk
sudo -u samverk vi /var/lib/samverk/.samverk/samverk.env
sudo systemctl restart samverk-serve samverk-dispatch
```

### MCP Client Keys

MCP clients (Claude Desktop, other tools) authenticate with per-device API keys managed by the `KeyStore`:

```bash
# On the server (or via SSH)
samverk key create "claude-desktop"    # Prints plaintext key once
samverk key list                       # Shows hashed keys
samverk key revoke "claude-desktop"    # Removes key
```

Keys are stored as SHA-256 hashes in `/var/lib/samverk/.samverk/auth.yaml`.

## Development Machine Setup

### PowerShell Vault (Source of Truth)

All secrets are stored in the `HomeLabVault` SecretStore vault (PowerShell 7 only):

```powershell
# Read a secret
gvs 'github/pat-personal'

# Store a new secret
nvs 'category/name' 'value'

# Sync vault to persistent user-level env vars
sync-secrets
```

After `sync-secrets`, all shells (PS5, PS7, CMD, Git Bash, WSL) inherit the updated values via `HKCU\Environment` registry key.

### Verifying Local Setup

```bash
# Check all required env vars are set
echo $GITHUB_TOKEN | head -c 10
echo $SAMVERK_AUTH_TOKEN | head -c 10
# Claude OAuth: claude --print --max-turns 1 "hello" (should return text, not error)
```

## Adding a New Secret

1. **Store in vault:** `nvs 'category/name' 'value'` (dev machine, PS7)
2. **Sync to env vars:** `sync-secrets` (dev machine, PS7)
3. **Add to server:** Edit `/var/lib/samverk/.samverk/samverk.env` on CT 202
4. **Update example:** Add placeholder to `deploy/samverk.env.example`
5. **Update Go code:** Add `os.Getenv("NEW_VAR")` where needed
6. **Update this doc:** Add row to the environment variables table

## Rotating a Secret

1. **Generate new value** in the source system (GitHub, Anthropic console, etc.)
2. **Update vault:** `nvs 'category/name' 'new-value'` (dev machine)
3. **Sync locally:** `sync-secrets` (dev machine)
4. **Update server:** SSH to CT 202, edit `samverk.env`, restart services
5. **Verify:** `curl -sf http://192.168.1.162:8080/healthz` (or `make redeploy`)

## Phase 3 Authentication (Future)

The current alpha uses per-device API keys via `KeyStore`. The planned migration path:

1. **Alpha (current):** Bearer token (`SAMVERK_AUTH_TOKEN`) + per-device API keys (`sk_*` hashes)
2. **Beta:** OAuth 2.1 with PKCE (device code flow for CLI, authorization code for web)
3. **v1.0:** Full OAuth 2.1 with token refresh, scoped permissions, audit logging

The `KeyStore` pattern in [apikey.go](../internal/server/apikey.go) is the correct foundation -- it stores hashes, never plaintext, and supports per-key project scoping. Future OAuth integration should build on this by adding:

- Token expiration and refresh
- Scope-based authorization (read vs write vs admin)
- Audit log entries for all auth events

Secrets must always come from environment variables or `EnvironmentFile` directives, never from config files committed to the repository.

## Related Documentation

- [Security Model](security-model.md) -- threat model, mitigation strategies
- [Data Governance](data-governance.md) -- what data flows to which providers
- [Architecture](architecture.md) -- system components and data flow
