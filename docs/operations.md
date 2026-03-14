# Operations Guide

## Infrastructure

| Service | Host | Port | Container |
| ------- | ---- | ---- | --------- |
| Samverk Server | 192.168.1.162 | 8080 | CT 202 |
| Samverk Dispatcher | 192.168.1.162 | - | CT 202 |
| Gitea | 192.168.1.160 | 3000 | CT 200 |
| Ollama | 192.168.1.207 | 11434 | VM 300 |

All containers run on Proxmox host at 192.168.1.203. SSH key auth is configured for both CT 202 and the Proxmox host.

## Deployment

### Quick Deploy

```bash
make redeploy
```

This cross-compiles for Linux, deploys to CT 202, restarts services, and verifies the health check in one step.

### Manual Deploy

```bash
# 1. Cross-compile on Windows
GOOS=linux GOARCH=amd64 go build -o bin/samverk-linux ./cmd/samverk/

# 2. Copy binary to CT 202
scp bin/samverk-linux root@192.168.1.162:/usr/local/bin/samverk

# 3. Restart services
ssh root@192.168.1.162 "systemctl restart samverk-serve samverk-dispatch"

# 4. Verify health
curl http://192.168.1.162:8080/healthz
```

## Service Management

```bash
# Check service status
systemctl status samverk-serve samverk-dispatch

# View recent logs
journalctl -u samverk-serve --since "1 hour ago"
journalctl -u samverk-dispatch --since "1 hour ago"

# Follow logs in real time
journalctl -u samverk-dispatch -f

# Restart a service
systemctl restart samverk-serve
systemctl restart samverk-dispatch
```

## Authentication

### API Auth

All `/api/` and `/mcp` routes are protected by `BearerAuth` middleware. Unauthenticated requests return 401.

Two auth modes are supported:

- **Simple token**: Single `SAMVERK_AUTH_TOKEN` environment variable
- **Key store**: YAML file with named keys, optional scope and worker identity

### Dashboard Auth

The SPA handler intercepts `index.html` at serve time and injects a `<script>` tag setting `window.__SAMVERK_TOKEN__`. The React app reads this value and adds `Authorization: Bearer` headers to all API requests.

### Key Management

```bash
# Create a general API key
samverk key create --name <name>

# Create a scoped worker key
samverk key create --name <name> --scope worker --worker-id <id>

# List keys
samverk key list --auth-keys .samverk/auth.yaml

# Revoke a key
samverk key revoke --name <name> --auth-keys .samverk/auth.yaml
```

### Per-Worker Identity

Workers register with scoped API keys (`scope: worker`, `worker_id: <name>`). The `KeyStore` validates scope and worker ID on registration and heartbeat endpoints.

## Health Check

```bash
# Basic health (no auth required)
curl http://192.168.1.162:8080/healthz

# Authenticated status endpoint
curl -H "Authorization: Bearer $TOKEN" http://192.168.1.162:8080/api/v1/status
```

## Monitoring

### Metrics API

```bash
curl -H "Authorization: Bearer $TOKEN" http://192.168.1.162:8080/api/v1/metrics
```

### Cross-Process Metrics

The dispatcher and serve processes run as separate systemd units. Metrics flow between them via SQLite:

- Dispatcher writes pool metrics and dispatcher metrics to SQLite every 30 seconds
- Serve process reads from SQLite for API responses
- Dashboard displays live metrics with auto-refresh

This was fixed in PR #423. If metrics show "Not running", check that the dispatcher is running and wait 30 seconds for the first snapshot write.

## Configuration Files (CT 202)

```text
/var/lib/samverk/.samverk/
├── samverk.env       # Environment variables (tokens, API keys)
├── server.yaml       # Project configuration
├── providers.yaml    # AI provider configuration
├── autonomy.yaml     # Trust tier thresholds
├── auth.yaml         # API key store (scoped keys)
└── samverk.db        # SQLite database (WAL mode)
```

## Troubleshooting

### Services will not start

Check status and logs:

```bash
systemctl status samverk-serve
journalctl -u samverk-serve --since "5 min ago"
```

Common causes: missing env file, database locked, port 8080 already in use.

### Metrics show "Not running"

Fixed in PR #423 (cross-process metrics bridge). If still happening:

1. Confirm the dispatcher is running: `systemctl status samverk-dispatch`
2. Wait 30 seconds for the first metrics snapshot to be written
3. Check SQLite for recent writes: look for `metrics_snapshots` table entries

### 401 on API calls

Verify that `SAMVERK_AUTH_TOKEN` in `samverk.env` matches the bearer token in the request. For key-store mode, check that `auth.yaml` contains the key and it has not been revoked.

### Dispatcher stuck in loop

Check for timeout issues in the dispatch log:

```bash
journalctl -u samverk-dispatch --since "1 hour ago" | grep -i "timeout\|stuck\|loop"
```

The dispatcher supports dynamic per-issue timeouts (PR #402) based on complexity. If issues are timing out repeatedly, check the issue's complexity label and consider adjusting thresholds.

### Agent sessions failing

Check for session checkpoint files and streaming progress:

```bash
journalctl -u samverk-dispatch --since "1 hour ago" | grep -i "checkpoint\|progress\|heartbeat"
```

Session checkpoint and resume (PR #406) allows partial work to carry across retries. Streaming progress detection (PR #405) resets heartbeat on active output.
