# Security Model

**Issue**: [#65](https://github.com/HerbHall/samverk/issues/65)
**Status**: Design (pre-implementation)
**Scope**: Authentication, authorization, secret management, and threat model for Samverk's MCP server, agent infrastructure, and multi-device access.

## Threat Model

### Attack Surface Inventory

Samverk exposes six attack surfaces. Each is analyzed for access vector, attacker profile, and blast radius.

| Surface | Protocol | Exposed To | Network |
|---------|----------|------------|---------|
| MCP endpoint (`/mcp`) | Streamable HTTP (JSON-RPC) | Claude on any device | LAN + Tailscale |
| Dashboard API (`/api/v1/`) | REST over HTTP | Browser SPA | LAN + Tailscale |
| Dashboard SPA (`/`) | Static files (embedded) | Browser | LAN + Tailscale |
| Gitea webhook/API | HTTPS | Samverk server process | LAN only |
| Agent containers | Docker API / Ollama REST | Dispatcher process | Localhost only |
| Config files (`.samverk/`) | Filesystem | Local processes, SSH | Host filesystem |

### Threat Scenarios and Blast Radius

#### T1: Stolen MCP API Key

**Vector**: API key extracted from a device (Claude MCP config file on phone, laptop clipboard, shoulder surfing).

**Blast radius**: The attacker can perform any action the key's scope allows. With an unrestricted key, this means full read access to all registered projects, issue creation and modification, and Tier 3 approval (approving merges to main, dependency additions, CI changes). The attacker cannot directly modify files in the repo (the MCP server has no write tools), but can approve agent actions that do.

**Impact**: HIGH. Tier 3 approval capability makes this equivalent to full project control.

**Mitigation**: Per-device keys with granular scopes. Mobile keys should be read-only + Tier 3 approval only. See [Device Permission Profiles](#device-permission-profiles).

#### T2: Compromised Agent Container

**Vector**: Malicious model output causes an agent to execute unintended commands. Container escape via Docker vulnerability. Prompt injection via issue body or comment.

**Blast radius**: The agent has access to: (a) its assigned Ollama model endpoint, (b) the Gitea API token scoped to the current project, (c) a working directory with the repo clone. If the container escapes, it reaches the host network and other containers.

**Impact**: MEDIUM (contained by Docker isolation). HIGH if container escape succeeds.

**Mitigation**: Read-only filesystem mounts for source code. Scoped Gitea tokens (not admin tokens). Network isolation -- agents cannot reach the MCP server or other agents directly. No host volume mounts for sensitive paths. See [Container Isolation](#container-isolation).

#### T3: Compromised Gitea Instance

**Vector**: Gitea LXC vulnerability. Weak Gitea admin credentials. Gitea API token leakage.

**Blast radius**: Full access to all repository contents, issue state, webhooks, and CI secrets. An attacker can modify issue bodies (injecting malicious frontmatter), create fake issues that trick the dispatcher into launching agent work, and exfiltrate source code.

**Impact**: HIGH. Gitea is the source of truth for task state and code.

**Mitigation**: Gitea behind Tailscale (not exposed to public internet). Strong admin password + 2FA. Gitea API tokens scoped per-project with minimal required permissions. Regular Gitea updates. Proxmox LXC isolation from other services.

#### T4: Dashboard Session Hijacking

**Vector**: XSS in the React SPA. Session token theft via network sniffing (if TLS is not enforced). CSRF if session cookies are used without proper SameSite attributes.

**Blast radius**: Full dashboard access -- configuration changes, autonomy tier modifications, API key viewing (if not properly masked), agent management. Dashboard has broader config access than the MCP endpoint.

**Impact**: MEDIUM-HIGH depending on dashboard capabilities exposed.

**Mitigation**: SPA served via embedded `embed.FS` (no CDN, no third-party scripts). CSP headers restricting script sources. Session tokens as HttpOnly cookies with SameSite=Strict. TLS via Tailscale cert or reverse proxy.

#### T5: Config File Exfiltration

**Vector**: Unauthorized filesystem access on the Proxmox host. SSH compromise. Backup containing `.samverk/` directory.

**Blast radius**: `.samverk/auth.yaml` contains API key hashes (not plaintext, but brute-forceable if weak). `.samverk/server.yaml` contains Gitea token environment variable names (not the tokens themselves, but reveals the naming convention). `autonomy.yaml` reveals the full permission model (useful for crafting an attack that stays within Tier 1/2).

**Impact**: MEDIUM. Hashed keys limit immediate exploitation. Autonomy config exposure enables social engineering of the permission model.

**Mitigation**: File permissions `0600` on all `.samverk/` files. Encrypt sensitive fields at rest (see [Secret Storage](#secret-storage-design)). Exclude `.samverk/` from backups or encrypt backups.

#### T6: Tailscale Node Compromise

**Vector**: A device on the Tailscale network is compromised. The attacker has network access to all Tailscale-exposed services.

**Blast radius**: Access to MCP endpoint, dashboard, and potentially Gitea if also on Tailscale. Equivalent to T1 + T4 combined if the attacker also obtains API keys from the compromised device.

**Impact**: HIGH. Tailscale is the network perimeter -- once inside, all services are reachable.

**Mitigation**: Per-device API keys so compromised device can be revoked without affecting others. Tailscale ACLs restricting which nodes can reach which services. Rate limiting on MCP and dashboard endpoints.

### OWASP MCP Top 10 Applicability

Cross-referencing with the [OWASP MCP Top 10](https://owasp.org/www-project-mcp-top-10/) threat categories:

| OWASP Category | Samverk Relevance | Mitigation |
|----------------|-------------------|------------|
| Tool poisoning | LOW -- Samverk defines its own tools, not third-party | Code review of tool implementations |
| Prompt injection | MEDIUM -- issue bodies are user/agent-authored | Frontmatter parser validates schema strictly; tool inputs are parameterized |
| Excessive permissions | HIGH -- agents hold Gitea tokens and API keys | Scoped tokens, autonomy tiers, container isolation |
| Credential leakage | HIGH -- multiple API keys in play | Encrypted storage, scoped injection, key rotation |
| Insufficient isolation | MEDIUM -- agents share the same host | Docker network isolation, read-only mounts |
| Token passthrough | LOW -- Samverk issues its own tokens | MCP server validates tokens server-side |

## Authentication Design

### Mechanism: Per-Device API Keys (Alpha) with OAuth 2.1 Migration Path (Beta)

**Alpha (personal, single user, local network + Tailscale)**: Static API keys per device with Bearer token authentication. Simple, sufficient, and appropriate for a single-user self-hosted deployment.

**Beta (shared use, multiple users)**: OAuth 2.1 with PKCE per the [MCP authorization specification](https://modelcontextprotocol.io/specification/draft/basic/authorization). The MCP server acts as an OAuth Resource Server. An authorization server (self-hosted, e.g., Authelia or Authentik) issues tokens.

### Alpha Authentication Flow

```text
1. User runs: samverk auth create --name "herb-desktop" --scope "full"
2. Server generates a cryptographically random 256-bit key
3. Server displays the key ONCE (never stored in plaintext)
4. Server stores SHA-256 hash in .samverk/auth.yaml
5. User configures key in Claude MCP settings on that device
6. Every MCP/API request includes: Authorization: Bearer <key>
7. Server hashes the received key and compares against stored hashes
```

### Token Lifecycle

```text
ISSUANCE          USAGE              ROTATION            REVOCATION
    |                |                   |                    |
samverk auth    Bearer header       samverk auth         samverk auth
  create        on every request      rotate               revoke
    |                |               --name "x"           --name "x"
    v                v                   |                    |
Generate key    Hash + compare      Generate new key     Delete hash
Store hash      against auth.yaml   Replace old hash     from auth.yaml
Show key once   Reject if no match  Show new key once    Reject future
    |                |               Old key invalid         requests
    v                v               immediately              |
Key in Claude   Access granted/                              v
MCP config      denied                                   Device locked
                                                         out immediately
```

### Key Properties

| Property | Value | Rationale |
|----------|-------|-----------|
| Key length | 256 bits (32 bytes, base64url encoded) | Resistant to brute force even against fast hashing |
| Hash algorithm | SHA-256 | Fast verification, sufficient for high-entropy keys |
| Storage | `.samverk/auth.yaml`, file mode `0600` | Server-side only, never in git |
| Transport | Bearer token in Authorization header | Standard HTTP auth, works with Tailscale TLS |
| Scope | Per-key, configurable | Different devices get different permissions |
| Rotation | Manual via CLI, zero-downtime | New key active immediately, old key invalidated |
| Revocation | Immediate on CLI command | Hash removed, all future requests rejected |

### auth.yaml Schema

```yaml
# .samverk/auth.yaml -- NEVER committed to git
# File permissions: 0600 (owner read/write only)

api_keys:
  - name: herb-desktop
    key_hash: "sha256:a1b2c3d4e5f6..."
    created: "2026-03-01T10:00:00Z"
    last_used: "2026-03-01T14:30:00Z"
    scope:
      projects: ["*"]         # all projects
      permissions: "full"     # full control including Tier 3 approvals
    device_metadata:
      type: desktop
      os: windows

  - name: herb-phone
    key_hash: "sha256:f6e5d4c3b2a1..."
    created: "2026-03-01T10:05:00Z"
    last_used: "2026-03-01T08:15:00Z"
    scope:
      projects: ["samverk", "subnetree"]
      permissions: "check-in"  # read + Tier 3 approval only
    device_metadata:
      type: mobile
      os: ios

  - name: herb-laptop
    key_hash: "sha256:1a2b3c4d5e6f..."
    created: "2026-03-01T10:10:00Z"
    last_used: "2026-03-01T20:00:00Z"
    scope:
      projects: ["*"]
      permissions: "operate"   # read + write + Tier 2, no Tier 3
    device_metadata:
      type: laptop
      os: macos
```

### Multi-Device Session Model

Samverk uses **stateless authentication** -- there are no sessions to manage. Each request is independently authenticated via the API key. This is deliberately chosen over session-based auth for several reasons:

- **No session fixation or hijacking** -- there are no session IDs to steal
- **No session state to synchronize** across MCP server restarts or scale-out
- **Multi-device access is trivial** -- each device has its own key, no session conflicts
- **MCP spec alignment** -- the MCP security spec states "MCP Servers MUST NOT use sessions for authentication"

The `last_used` field in `auth.yaml` tracks the most recent request per key for audit purposes. It does not affect authentication decisions.

### Compromised Device Deauthorization

```bash
# Immediate revocation -- phone lost at a bar
samverk auth revoke --name "herb-phone"

# Verify revocation
samverk auth list
# NAME           SCOPE       LAST USED            STATUS
# herb-desktop   full        2026-03-01 14:30     active
# herb-phone     check-in    2026-03-01 08:15     REVOKED
# herb-laptop    operate     2026-03-01 20:00     active

# Other devices are unaffected -- desktop and laptop continue working
```

Revocation is immediate. The hash is removed from `auth.yaml` (or marked revoked and retained for audit). All subsequent requests with the revoked key receive HTTP 401.

## Authorization Design

### Permission Model

Authorization in Samverk operates on two independent axes:

1. **API key scope** -- what the device is allowed to do (coarse-grained, per-key)
2. **Autonomy tier** -- what level of action requires confirmation (fine-grained, per-project)

Both checks must pass for an action to proceed. A mobile device with `check-in` scope cannot bypass autonomy tiers, and an autonomy Tier 1 action still requires a valid API key with appropriate scope.

### Device Permission Profiles

Three predefined profiles cover the primary use cases. Custom profiles can be defined for specific needs.

| Profile | Read | Create Issues | Modify Issues | Tier 3 Approve | Config Changes | Use Case |
|---------|------|---------------|---------------|----------------|----------------|----------|
| `check-in` | Yes | Yes | No | Yes | No | Phone -- quick check-ins, approve/reject pending actions |
| `operate` | Yes | Yes | Yes | No | No | Laptop -- full issue management, no irreversible approvals |
| `full` | Yes | Yes | Yes | Yes | Yes | Desktop -- complete control including config and Tier 3 |

The `check-in` profile is specifically designed for mobile use: the user can read the digest, approve or reject Tier 3 pending actions, and create new issues (to capture ideas on the go), but cannot modify existing issues or configuration. This minimizes the blast radius of a compromised phone.

The `operate` profile is the daily driver for a work laptop: full issue management but no Tier 3 approval authority. Tier 3 actions accumulate and wait for the `full`-scope desktop session.

### Authorization Flow

```text
Request arrives with Bearer token
    |
    v
Hash token, look up in auth.yaml
    |
    +-- No match --> 401 Unauthorized
    |
    v
Check key scope: is this project allowed?
    |
    +-- Project not in scope --> 403 Forbidden
    |
    v
Check key scope: is this action type allowed?
    |
    +-- Action not in profile --> 403 Forbidden
    |
    v
Look up action's autonomy tier (autonomy.yaml)
    |
    +-- Tier 1/2: Execute immediately, log
    |
    +-- Tier 3: Return confirmation_required response
    |
    v
Response to client
```

### Relationship Between Scopes and Autonomy Tiers

The API key scope acts as a **ceiling** on what a device can do. Autonomy tiers act as a **gate** on how actions are executed. They are complementary, not redundant.

| Scenario | API Key Scope | Autonomy Tier | Result |
|----------|---------------|---------------|--------|
| Phone approves merge to main | `check-in` (Tier 3 allowed) | Tier 3 | Confirmation prompt, then execute |
| Phone edits issue labels | `check-in` (modify not allowed) | Tier 2 | 403 Forbidden (scope blocks it) |
| Desktop edits issue labels | `full` (modify allowed) | Tier 2 | Execute, log in digest |
| Desktop merges to main | `full` (Tier 3 allowed) | Tier 3 | Confirmation prompt, then execute |
| Laptop merges to main | `operate` (Tier 3 not allowed) | Tier 3 | 403 Forbidden (scope blocks it) |

### Multi-User Scenario (Beta)

Alpha is single-user. For beta (shared Samverk instance):

- Each user has their own set of API keys
- Each user has their own permission profiles
- Project-level ACLs determine which users can access which projects
- Autonomy config (`autonomy.yaml`) applies per-project, not per-user -- all users of a project share the same tier definitions
- User identity is derived from the API key (alpha) or OAuth token claims (beta)

```yaml
# Beta: .samverk/auth.yaml with multi-user support
users:
  - name: herb
    api_keys:
      - name: herb-desktop
        key_hash: "sha256:..."
        scope:
          projects: ["*"]
          permissions: "full"
  - name: alice
    api_keys:
      - name: alice-laptop
        key_hash: "sha256:..."
        scope:
          projects: ["shared-project"]
          permissions: "operate"
```

## Secret Management Design

### Secret Inventory

Samverk manages four categories of secrets:

| Secret | Where Used | Risk If Exposed |
|--------|-----------|-----------------|
| MCP API keys | Claude MCP config on devices | Full MCP access per key scope |
| Gitea API tokens | Dispatcher + agent containers | Full repo and issue access |
| AI provider API keys | Agent containers via provider clients | API billing, model access |
| Dashboard session tokens | Browser | Dashboard access |

### Secret Storage Design

#### Tier 1: Hashed Secrets (API keys)

MCP API keys are stored as SHA-256 hashes in `auth.yaml`. The plaintext key exists only on the device where it was configured. This is the same pattern used by Gitea for personal access tokens and Home Assistant for long-lived access tokens.

**Why not bcrypt/argon2?** These are designed for low-entropy passwords where brute force is a concern. Samverk API keys are 256-bit random values -- brute-forcing SHA-256 of a 256-bit key is computationally infeasible. SHA-256 is faster to verify (lower latency per MCP request) with no security tradeoff for high-entropy keys.

#### Tier 2: Encrypted Secrets (provider API keys, Gitea tokens)

AI provider API keys and Gitea tokens must be stored as usable values (not just hashes) because the server needs to pass them to external APIs. These are encrypted at rest.

```yaml
# .samverk/secrets.yaml -- encrypted at rest
# File permissions: 0600

encryption:
  method: "aes-256-gcm"
  key_derivation: "argon2id"
  # Encryption key derived from a master passphrase entered on server start
  # OR from a hardware-backed key (Trusted Platform Module) on supported systems

secrets:
  gitea_tokens:
    samverk:
      encrypted: "base64:iv:ciphertext:tag"
    subnetree:
      encrypted: "base64:iv:ciphertext:tag"

  provider_keys:
    anthropic:
      encrypted: "base64:iv:ciphertext:tag"
    openai:
      encrypted: "base64:iv:ciphertext:tag"
    # Ollama: no API key needed (local, unauthenticated)
```

**Alpha simplification**: For alpha (single user, trusted host), environment variables are acceptable:

```bash
# In the systemd unit file or shell profile (not committed to git)
export GITEA_TOKEN_SAMVERK="gta_abc123..."
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENAI_API_KEY="sk-..."
```

The `token_env` fields in `server.yaml` reference these environment variable names. The actual keys never appear in any config file.

**Beta requirement**: Move to `secrets.yaml` with encryption. The master passphrase is entered at server start or provided via a hardware token.

#### Tier 3: Ephemeral Secrets (dashboard sessions)

Dashboard session tokens are generated per-login, stored in memory only (not persisted), and expire after 24 hours of inactivity. They are HttpOnly cookies, not accessible to JavaScript.

### Secret Rotation

| Secret | Rotation Trigger | Procedure | Downtime |
|--------|-----------------|-----------|----------|
| MCP API key | Manual, or on suspected compromise | `samverk auth rotate --name "x"` | Zero -- new key active immediately |
| Gitea token | Manual, recommended quarterly | Regenerate in Gitea UI, update env var, restart server | Brief -- server restart required |
| AI provider key | Manual, or on billing alert | Regenerate in provider dashboard, update env var, restart | Brief -- server restart required |
| Dashboard session | Automatic on expiry | User re-authenticates via dashboard login | None -- transparent |

### Agent Secret Scoping

Agents must receive the minimum secrets required for their task. No agent receives all secrets.

```text
DISPATCHER PROCESS (host)
├── Has: All Gitea tokens (for all projects)
├── Has: All AI provider keys (for routing to providers)
├── Does NOT have: MCP API keys (those are for inbound auth)
│
├── AGENT: code-gen (Docker container)
│   ├── Receives: Gitea token for CURRENT PROJECT ONLY
│   ├── Receives: Ollama endpoint (no key needed)
│   ├── Does NOT receive: Other project tokens
│   ├── Does NOT receive: Cloud provider API keys
│   └── Network: Can reach Ollama + Gitea only
│
├── AGENT: qc (Docker container)
│   ├── Receives: Gitea token for CURRENT PROJECT ONLY
│   ├── Receives: Claude API key (for complex reasoning)
│   ├── Does NOT receive: Other project tokens
│   ├── Does NOT receive: OpenAI key (not needed for this agent type)
│   └── Network: Can reach Claude API + Gitea only
│
└── AGENT: docs (Docker container)
    ├── Receives: Gitea token for CURRENT PROJECT ONLY
    ├── Receives: Ollama endpoint (no key needed)
    └── Network: Can reach Ollama + Gitea only
```

Secrets are injected via Docker secrets or environment variables at container creation time. They are never baked into the container image, never written to the container filesystem, and never logged.

### Container Isolation

Each agent container operates with these restrictions:

```yaml
# Docker container security configuration (conceptual)
agent_container:
  user: "65534:65534"           # nobody:nogroup -- not root
  read_only: true               # Read-only root filesystem
  no_new_privileges: true       # Prevent privilege escalation
  cap_drop: ["ALL"]             # Drop all Linux capabilities
  security_opt:
    - "no-new-privileges:true"
  tmpfs:
    - /tmp:size=100m            # Writable temp only in tmpfs

  # Network isolation
  networks:
    - agent-net                 # Isolated network, not the host network
  # Only specific endpoints are reachable via agent-net

  # Resource limits
  mem_limit: "2g"
  cpus: "2.0"
  pids_limit: 256

  # No host mounts for sensitive paths
  # Source code mounted read-only
  volumes:
    - type: bind
      source: /data/repos/samverk
      target: /workspace
      read_only: true
  # Agent writes output to Gitea API (commits, comments), not local filesystem
```

**Why read-only source mount?** Agents should modify code via git operations (commit to branch, push via Gitea API), not by writing directly to the host filesystem. This ensures all changes go through the forge's audit trail and are subject to autonomy tier evaluation.

### What Happens If an Agent Container Is Compromised

| Compromised Component | Attacker Gets | Attacker Does NOT Get |
|----------------------|---------------|----------------------|
| Agent container memory | Current task's Gitea token (one project) | Other project tokens |
| Agent container memory | Ollama endpoint (or one cloud API key) | All provider keys |
| Agent container filesystem | Read-only source code | Write access to source |
| Agent container network | Access to Ollama + Gitea (scoped) | Access to MCP server, dashboard, other agents |
| Container escape to host | Host filesystem access | MCP API keys (hashed in auth.yaml) |

The blast radius of a single agent compromise is limited to one project's Gitea access and one AI provider's API key. Cross-project access requires compromising the dispatcher process itself.

## Minimum Viable Security

### Alpha Release (Personal Use)

Alpha is a single user on a trusted local network with Tailscale for remote access. The threat model is: accidental exposure, device loss, and defense against opportunistic attacks. Not: targeted attacks by sophisticated adversaries.

| Requirement | Implementation | Priority |
|-------------|---------------|----------|
| MCP authentication | Per-device API keys, SHA-256 hashed | P0 (ship blocker) |
| API key CLI management | `samverk auth create/revoke/list/rotate` | P0 |
| Device permission profiles | `check-in`, `operate`, `full` scopes | P0 |
| Autonomy tier enforcement | Server-side tier check before every action | P0 (already designed) |
| Secret storage (provider keys) | Environment variables, documented setup | P0 |
| Config file permissions | `0600` on all `.samverk/` files, enforced at startup | P0 |
| TLS transport | Tailscale auto-TLS for remote; localhost exempt | P1 |
| Dashboard auth | Simple password or API key reuse | P1 |
| Rate limiting | Basic rate limit on auth failures (10/min) | P1 |
| Audit logging | All Tier 2/3 actions logged to SQLite | P1 |
| Container isolation | Docker containers with `--read-only`, no-root, cap-drop | P2 |
| Agent secret scoping | Per-project Gitea tokens injected at runtime | P2 |

**Alpha explicitly defers:**

- OAuth 2.1 / OIDC (not needed for single user)
- Multi-user access control
- Encrypted secret storage (env vars are sufficient on a trusted host)
- Network-level container isolation (Docker default bridge is acceptable)
- Automated key rotation
- SIEM integration or external audit log shipping

### Beta Release (Shared Use)

Beta adds remote access for collaborators and multi-user support. The threat model expands to include: malicious or compromised collaborators, shared network access, and defense against targeted attacks.

| Requirement | Implementation | Priority |
|-------------|---------------|----------|
| OAuth 2.1 authentication | MCP server as OAuth Resource Server, self-hosted AS (Authelia/Authentik) | P0 |
| Multi-user authorization | Per-user API keys, project-level ACLs | P0 |
| Encrypted secret storage | `secrets.yaml` with AES-256-GCM, Argon2id KDF | P0 |
| TLS everywhere | Mandatory TLS on all endpoints, no plaintext | P0 |
| Dashboard auth | OAuth 2.1 via same AS, session cookies with CSRF protection | P0 |
| Container network isolation | Per-agent Docker networks, egress firewall rules | P1 |
| Automated key rotation | `samverk auth rotate --all --interval 90d` | P1 |
| Audit log integrity | Append-only SQLite with hash chain | P1 |
| Rate limiting (advanced) | Per-key rate limits, exponential backoff on failures | P1 |
| Tailscale ACLs | Restrict which Tailscale nodes reach which services | P1 |
| Vulnerability scanning | Automated `govulncheck` + `trivy` in CI | P2 |
| Secrets rotation automation | Auto-rotate Gitea tokens via Gitea API | P2 |

## Security Configuration Reference

### HTTP Headers (All Responses)

```go
// Mandatory security headers for all HTTP responses
w.Header().Set("X-Content-Type-Options", "nosniff")
w.Header().Set("X-Frame-Options", "DENY")
w.Header().Set("X-XSS-Protection", "0") // Disabled per OWASP (use CSP instead)
w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

// CSP for dashboard pages (not MCP JSON-RPC endpoint)
w.Header().Set("Content-Security-Policy",
    "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
    "img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
```

### Rate Limiting Configuration

```yaml
# .samverk/server.yaml (rate limiting section)
rate_limiting:
  # Auth failure rate limit -- prevents brute force on API keys
  auth_failures:
    window: 60s
    max_attempts: 10
    lockout_duration: 300s  # 5 minute lockout after 10 failures

  # Per-key request rate limit -- prevents abuse
  per_key:
    window: 60s
    max_requests: 120  # 2 requests/second sustained

  # Global rate limit -- prevents DoS
  global:
    window: 60s
    max_requests: 600
```

### Audit Log Schema

```sql
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,          -- RFC3339
    api_key_name TEXT NOT NULL,       -- which device
    action TEXT NOT NULL,             -- MCP tool name or API endpoint
    project TEXT,                     -- project context
    autonomy_tier INTEGER,           -- 1, 2, or 3
    result TEXT NOT NULL,             -- "allowed", "denied", "confirmation_required"
    details TEXT,                     -- JSON blob with action-specific data
    ip_address TEXT,                  -- client IP (for anomaly detection)
    user_agent TEXT                   -- client identifier
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_key ON audit_log(api_key_name);
CREATE INDEX idx_audit_action ON audit_log(action);
```

## ADR Draft: ADR-024 -- Per-Device API Key Authentication

### Status

Proposed

### Context

Samverk's MCP server accepts connections from Claude running on multiple devices (phone, laptop, desktop). The MCP specification recommends OAuth 2.1 for production deployments but acknowledges simpler mechanisms for local/personal use. Our alpha is a single-user, self-hosted deployment on a local network with Tailscale for remote access.

The MCP security best practices specify that "MCP Servers MUST NOT use sessions for authentication" -- each request must be independently authenticated. This rules out session-based auth and points toward token-based auth.

We need an auth mechanism that:

- Works with Claude MCP configuration (Bearer token in Authorization header)
- Supports multiple devices with different permission levels
- Allows immediate revocation of a single device without affecting others
- Is simple enough for a single Go binary with no external dependencies
- Has a clear migration path to OAuth 2.1 for multi-user beta

### Decision

Use per-device API keys with SHA-256 hashing for alpha. Each device gets its own key with a named permission profile (`check-in`, `operate`, `full`). Keys are managed via the `samverk auth` CLI subcommand.

The migration path to OAuth 2.1 for beta is straightforward: the authorization check that currently reads `auth.yaml` will be replaced by an OAuth token validation middleware. The permission profiles map directly to OAuth scopes. The autonomy tier enforcement layer is unchanged.

### Consequences

**Positive:**

- Zero external dependencies (no OAuth server for alpha)
- Per-device revocation is instant and surgical
- Permission profiles enforce least-privilege per device
- Stateless auth aligns with MCP spec requirements
- Simple to implement and reason about

**Negative:**

- Key distribution is manual (user copies key to Claude config)
- No token expiry (keys are valid until explicitly revoked)
- No refresh mechanism (rotation requires re-configuring the device)
- Single-user only -- multi-user requires the OAuth 2.1 migration

**Mitigations for negatives:**

- Manual key distribution is acceptable for a single user with 2-3 devices
- No expiry is mitigated by the `last_used` audit field and manual rotation CLI
- The `samverk auth rotate` command makes re-keying a single command

### Alternatives Considered

| Alternative | Why Rejected |
|-------------|-------------|
| OAuth 2.1 from day one | Requires running an OAuth AS (Authelia, Authentik). Over-engineered for single-user alpha. Add in beta. |
| mTLS (mutual TLS) | Excellent security but certificate management is complex. Tailscale already provides mutual authentication at the network layer. |
| Session cookies | MCP spec says "MUST NOT use sessions for authentication." Stateful, requires storage synchronization. |
| Single shared API key | No per-device revocation. Compromised phone requires re-keying all devices. |
| IP-based allowlisting | Tailscale IPs can change. CGNAT and mobile networks make IP-based auth unreliable. |

## Related Documents

- [MCP Server Requirements](mcp-server.md) -- transport, tools, and current auth design
- [Autonomy Model](autonomy-model.md) -- tier definitions and enforcement
- [Architecture](architecture.md) -- system components and data flow
- [Tech Stack](tech-stack.md) -- implementation choices
- [Intent Verification Protocol](intent-verification.md) -- understanding verification (complements permission model)
- [Multi-Session Safety](multi-session-safety.md) -- manual guardrails for concurrent access

## External References

- [MCP Security Best Practices](https://modelcontextprotocol.io/specification/draft/basic/security_best_practices) -- official MCP specification security guidance
- [MCP Authorization Specification](https://modelcontextprotocol.io/specification/draft/basic/authorization) -- OAuth 2.1 for MCP servers
- [OWASP MCP Top 10](https://owasp.org/www-project-mcp-top-10/) -- MCP-specific threat categories
- [OWASP Secure MCP Server Development Guide](https://genai.owasp.org/resource/a-practical-guide-for-secure-mcp-server-development/) -- practical implementation guidance
- [Pentagi Agent Security Patterns](https://www.sitepoint.com/security-patterns-for-autonomous-agents-lessons-from-pentagi/) -- container isolation and least-privilege patterns for autonomous agents
- [Docker Sandboxes for Coding Agents](https://www.docker.com/blog/docker-sandboxes-a-new-approach-for-coding-agent-safety/) -- microVM and container isolation strategies
- [Gitea API Scoped Tokens](https://docs.gitea.com/development/api-usage) -- per-scope token model for forge access
- [Home Assistant Authentication](https://www.home-assistant.io/docs/authentication/) -- long-lived access token pattern for self-hosted tools
