# Webhook vs Polling Strategy for the Dispatcher

## Problem

The dispatcher agent needs real-time awareness of issue changes (new issues, label updates, comments) to route work to specialist agents. Two mechanisms exist:

- **Webhooks**: The forge pushes events to an HTTP endpoint -- low latency, but unreliable
- **Polling**: The dispatcher periodically queries the forge API -- reliable, but adds latency

Neither alone is sufficient.

## Decision: Hybrid Approach

**Webhooks primary, polling fallback.** Webhooks provide speed. Polling provides reliability. Together they cover each other's failure modes.

```text
                    ┌──────────────────┐
                    │   Git Forge       │
                    │  (GitHub/Gitea)   │
                    └───────┬──────────┘
                            │
              ┌─────────────┼─────────────┐
              │             │             │
              ▼             │             ▼
     ┌────────────┐        │      ┌──────────────┐
     │  Webhook    │        │      │  Poll Timer   │
     │  Endpoint   │        │      │  (60s cycle)  │
     └─────┬──────┘        │      └──────┬───────┘
           │               │             │
           ▼               │             ▼
     ┌──────────────────────────────────────┐
     │         Event Deduplicator            │
     │  (seen map: event-key → timestamp)    │
     └───────────────┬──────────────────────┘
                     │
                     ▼
     ┌──────────────────────────────────┐
     │         Dispatcher Core           │
     │  (evaluate, route, claim, track)  │
     └──────────────────────────────────┘
```

## Webhook Characteristics

### GitHub

| Property | Value |
|----------|-------|
| Delivery timeout | 10 seconds |
| Automatic retries | **No** -- failed deliveries stay failed |
| Manual redelivery | Yes, via REST API (past 30 days) |
| Deduplication header | `X-GitHub-Delivery` (unique GUID per delivery) |
| Ordering guarantee | **None** -- events may arrive out of sequence |
| Payload size limit | 25 MB (silently dropped if exceeded) |
| Signature validation | HMAC-SHA256 (`X-Hub-Signature-256` header) |

### Gitea (Self-Hosted)

| Property | Value |
|----------|-------|
| Delivery timeout | 5 seconds (configurable via `DELIVER_TIMEOUT` in `app.ini`) |
| Automatic retries | **No** |
| Manual redelivery | UI only -- no programmatic API |
| Deduplication header | Not documented -- implement via payload hash |
| Ordering guarantee | **None** |
| Payload size limit | Not documented |
| Signature validation | HMAC-SHA256 (configurable) |
| Queue length | 1000 events (configurable via `QUEUE_LENGTH`) |

### Key Takeaway

Neither platform retries failed webhook deliveries automatically. Any webhook-only strategy **will miss events** during server restarts, network partitions, or TLS failures.

## Polling Characteristics

### GitHub (Authenticated)

| Property | Value |
|----------|-------|
| Rate limit | 5,000 requests/hour (authenticated) |
| Max results per page | 100 issues |
| Recommended interval | 60 seconds (uses ~60 req/hr for issue polling) |
| Conditional requests | `If-None-Match` / `304 Not Modified` (saves rate limit) |

At 60-second intervals, polling a single repo's issues consumes ~60 of 5,000 hourly requests (1.2%). Additional calls for comments on changed issues might double this, still well within limits.

### Gitea (Self-Hosted)

| Property | Value |
|----------|-------|
| Rate limit | **None** by default (you own the server) |
| Max results per page | Up to 100 (configurable) |
| Recommended interval | 30 seconds (no rate limit concern) |
| Conditional requests | Not documented |

Self-hosted Gitea can poll more aggressively since there are no rate limits.

## Hybrid Architecture

### Event Flow

1. **Webhook handler** receives a forge event, normalizes it to `forge.Event`, and passes it to the deduplicator
2. **Poll timer** fires every N seconds, queries for issues updated since last poll, and generates synthetic events for any changes found
3. **Deduplicator** maintains a time-windowed set of recently processed events, drops duplicates
4. **Dispatcher core** receives deduplicated events and executes routing logic

### State Diffing for Polling

The poller tracks a snapshot of known issue states. On each poll:

```text
1. Fetch all open issues with status:queued, status:claimed, status:in-progress
2. Compare against previous snapshot
3. For each difference:
   - New issue not in snapshot → emit EventIssueOpened
   - Label changed → emit EventIssueLabeled
   - New comment → emit EventIssueCommented
   - Issue closed → emit EventIssueClosed
4. Update snapshot
```

This converts polling results into the same event stream that webhooks produce, so the dispatcher core handles both identically.

### Deduplication

Events are deduplicated by a composite key:

```text
key = "{issue_number}:{event_type}:{trigger_timestamp}"
```

The deduplicator maintains a map of seen keys with a configurable TTL (default: 5 minutes). If a webhook and poll both produce the same event, the second one is silently dropped.

```go
type Deduplicator struct {
    mu   sync.Mutex
    seen map[string]time.Time
    ttl  time.Duration
}

func (d *Deduplicator) IsDuplicate(key string) bool {
    d.mu.Lock()
    defer d.mu.Unlock()

    if t, ok := d.seen[key]; ok && time.Since(t) < d.ttl {
        return true
    }
    d.seen[key] = time.Now()
    return false
}
```

### Graceful Degradation

| Scenario | Webhook | Poll | Result |
|----------|---------|------|--------|
| Normal operation | Active | Active (backup) | Webhook delivers within seconds; poller confirms nothing was missed |
| Webhook endpoint down | Dead | Active | Events arrive on next poll cycle (30-60s latency) |
| Forge API degraded | May still deliver | Fails/slow | Webhooks still work; poller backs off with exponential delay |
| Network partition | Dead | Dead | Both fail; dispatcher detects and logs alert; recovers on reconnection |
| Server restart | Misses events during downtime | First poll after restart catches up | Polling recovers missed events via state diff |

## Webhook Endpoint Design

### Validation

Both GitHub and Gitea sign payloads with HMAC-SHA256. The webhook handler must:

1. Read the raw request body
2. Compute HMAC-SHA256 using the configured webhook secret
3. Compare against the signature header (`X-Hub-Signature-256`)
4. Reject requests that don't match (return 401)

### Fast Response

The 10-second (GitHub) / 5-second (Gitea) timeout means the handler must respond quickly. Pattern:

```go
func webhookHandler(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    if !validateSignature(body, r.Header.Get("X-Hub-Signature-256")) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    // Respond immediately -- process asynchronously.
    w.WriteHeader(http.StatusAccepted)

    // Parse and enqueue in a separate goroutine.
    go func() {
        event, err := parseWebhookPayload(body, r.Header)
        if err != nil {
            log.Error("webhook parse", zap.Error(err))
            return
        }
        eventCh <- event
    }()
}
```

### Endpoint Exposure

For self-hosted Gitea, the webhook endpoint is on the same network. For GitHub, the Samverk server must be reachable from the internet. Options:

1. **Direct exposure**: Open a port, configure TLS, point GitHub webhook at it
2. **Reverse tunnel**: Use ngrok, Cloudflare Tunnel, or similar to expose local endpoint
3. **Polling-only mode**: Skip webhooks entirely, rely on polling

The configuration should support all three modes. Polling-only mode is the safe default that works everywhere.

## Configuration

```yaml
# .samverk/dispatcher.yaml
watch:
  # Mode: "hybrid" (default), "webhook-only", "poll-only"
  mode: hybrid

  polling:
    interval_seconds: 60       # GitHub: 60s (rate limit aware); Gitea: 30s
    backoff_max_seconds: 300   # Max backoff on consecutive API failures

  webhook:
    enabled: true
    listen_addr: ":8081"       # Webhook listener port
    secret: ""                 # HMAC-SHA256 secret (from forge config)
    tls_cert: ""               # TLS cert path (optional)
    tls_key: ""                # TLS key path (optional)

  dedup:
    ttl_seconds: 300           # How long to remember seen events
```

### Mode Selection Guide

| Situation | Recommended Mode |
|-----------|-----------------|
| GitHub.com + server has public IP/tunnel | `hybrid` |
| GitHub.com + no public IP | `poll-only` |
| Self-hosted Gitea on same LAN | `hybrid` (no TLS/tunnel needed) |
| Development/testing | `poll-only` |

## Polling Interval Analysis

### GitHub

With 5,000 requests/hour:

| Interval | Requests/hour | % of limit | Latency |
|----------|--------------|-----------|---------|
| 30s | 120 | 2.4% | 30s avg |
| 60s | 60 | 1.2% | 60s avg |
| 120s | 30 | 0.6% | 120s avg |

The 60-second default uses 1.2% of the rate limit. Even with additional comment-fetching on changed issues, total usage stays under 5%.

### Gitea

No rate limits. A 30-second interval is reasonable. Could go as low as 10 seconds without issue.

## Implementation Phases

1. **Phase 1 (current)**: Polling-only with state diffing. Already referenced in the existing `Watch()` method signature. No webhook infrastructure needed.
2. **Phase 2**: Add webhook endpoint to the Samverk HTTP server. Parse GitHub/Gitea payloads into `forge.Event`. Add deduplicator.
3. **Phase 3**: Add webhook registration via `IssueTracker` interface (create/delete hooks programmatically). Add TLS configuration.

Phase 1 is sufficient for development and single-user self-hosted use. Phase 2 adds responsiveness. Phase 3 adds zero-configuration setup.

## Related Decisions

- [ADR-012: Git Issues as Agent Communication](decisions/ADR-012-git-issues-protocol.md)
- [ADR-013: Forge Abstraction](decisions/ADR-013-forge-abstraction.md)
- [ADR-014: Dedicated Dispatcher Agent](decisions/ADR-014-dispatcher-agent.md)
- [Communication Protocol](communication-protocol.md)
- [Forge Compatibility Matrix](forge-compatibility.md)
- [Optimistic Locking](optimistic-locking.md)
