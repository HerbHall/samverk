# Logging Design

## Principle

**In production, emit structured JSON without color. For local development, use a colorized console formatter for easier reading.**

Samverk must support dual-mode logging controlled by environment, not build flags. The same binary behaves differently based on context.

## Library: `uber-go/zap`

zap provides both encoders natively — `zapcore.NewJSONEncoder` for production and `zapcore.NewConsoleEncoder` with color for development. No wrapper library needed.

## Environment Toggle

```go
package logging

import (
    "os"

    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

// New creates a logger based on SAMVERK_ENV.
//   - "development" or "dev": colorized console output, debug level
//   - anything else (including unset): structured JSON, info level
func New() (*zap.Logger, error) {
    env := os.Getenv("SAMVERK_ENV")

    switch env {
    case "development", "dev":
        cfg := zap.NewDevelopmentConfig()
        cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
        cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
        return cfg.Build()
    default:
        cfg := zap.NewProductionConfig()
        cfg.EncoderConfig.TimeKey = "ts"
        cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
        return cfg.Build()
    }
}
```

### Output Examples

**Production** (`SAMVERK_ENV` unset or `production`):

```json
{"level":"info","ts":"2026-03-09T12:34:56.789Z","msg":"task claimed","agent":"code-gen-1","issue":42,"project":"subnetree"}
{"level":"warn","ts":"2026-03-09T12:34:57.123Z","msg":"provider unhealthy","provider":"claude","latency_ms":5200}
{"level":"error","ts":"2026-03-09T12:34:58.456Z","msg":"dispatch failed","issue":43,"error":"dependency not met: #41"}
```

**Development** (`SAMVERK_ENV=development`):

```text
12:34:56.789  INFO   task claimed           {"agent": "code-gen-1", "issue": 42, "project": "subnetree"}
12:34:57.123  WARN   provider unhealthy     {"provider": "claude", "latency_ms": 5200}
12:34:58.456  ERROR  dispatch failed        {"issue": 43, "error": "dependency not met: #41"}
```

(WARN in yellow, ERROR in red, INFO in default — via `CapitalColorLevelEncoder`)

## Structured Fields Standard

Every log entry from agent or dispatcher context MUST include these fields where applicable:

| Field | Type | When |
|-------|------|------|
| `agent` | string | Any agent-initiated log |
| `issue` | int | When operating on an issue |
| `project` | string | When scoped to a project |
| `provider` | string | AI provider calls |
| `task_id` | string | Dispatcher task tracking |
| `duration_ms` | int64 | Any timed operation |
| `cost_usd` | float64 | API calls with billing |

Use `zap.String()`, `zap.Int()`, etc. for type-safe structured fields — never `fmt.Sprintf` into the message string.

## Migration Path (slog to zap)

Current codebase uses `log/slog` across 14 files. Migration steps:

1. Add `go.uber.org/zap` to `go.mod`
2. Create `internal/logging/logging.go` with the `New()` function above
3. Initialize the logger in `cmd/samverk/main.go` and pass via dependency injection (not globals)
4. Replace each `slog.Info/Warn/Error` call with the equivalent `logger.Info/Warn/Error`
5. Add structured fields (agent, issue, project) to each call site
6. Remove `log/slog` imports

Estimated scope: ~50 log call sites across 14 files.

## Log Persistence (Phase 2 Dashboard)

For the dashboard structured log viewer:

1. **Primary output:** stdout (JSON in production) — captured by systemd journal on CT 202
2. **Dashboard persistence:** A `zapcore.WriteSyncer` tee that also writes to SQLite
3. **Retention:** SQLite log table with TTL-based cleanup (configurable, default 7 days)
4. **Query:** `/api/v1/logs` REST endpoint with filtering by agent, severity, task, time range
5. **Real-time:** WebSocket endpoint for live tail in the dashboard

## External Tool Compatibility

Production JSON output is compatible with standard log processing tools:

- `jq` for ad-hoc filtering: `samverk serve 2>&1 | jq 'select(.level=="error")'`
- `humanlog` for colorized terminal reading of JSON: `samverk serve 2>&1 | humanlog`
- Grafana Loki / Promtail for centralized log aggregation (future)
- systemd journal captures stdout natively on CT 202

## CT 202 (Production Server)

The systemd unit should NOT set `SAMVERK_ENV` — defaulting to production JSON output. For debugging on the server, SSH in and override temporarily:

```bash
# Normal operation (JSON to journal)
systemctl status samverk-serve

# Debug session (colorized console)
SAMVERK_ENV=development /opt/samverk/samverk serve
```

## Development Machine (HDH-NZXT)

Set in your shell profile or `.samverk/` config:

```powershell
# PowerShell
$env:SAMVERK_ENV = "development"
```

```bash
# Git Bash / WSL
export SAMVERK_ENV=development
```

The `make run` target should default to development mode:

```bash
make run  # SAMVERK_ENV=development go run ./cmd/samverk/ serve
```

## Related Documents

- [Tech Stack](tech-stack.md) — library choice rationale
- [Adaptive Scaling Plan](adaptive-scaling-plan.md) — W01-W03 metrics that feed into log context
- [Architecture](architecture.md) — dashboard scope includes structured log viewer
