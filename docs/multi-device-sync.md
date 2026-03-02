# Multi-Device Context Synchronization

**Issue**: [#73](https://github.com/HerbHall/samverk/issues/73)
**Status**: Design (pre-implementation)
**Scope**: State taxonomy, sync mechanisms, conflict resolution, and cross-device UX for users checking in from multiple devices.

## The Core Question

When a user checks in from their phone Monday, approves two PRs, sets direction -- then checks in from their laptop Wednesday -- what state carries between sessions and devices?

## Design Principle: The Server Knows Everything

Samverk's architecture already answers the hard sync question by design. The Samverk server (MCP endpoint + REST API + SQLite) is the single source of truth for all project state. Devices are stateless views into that truth.

This is not a distributed sync problem. It is a "read from one database, render on different screens" problem. The async check-in model makes this dramatically simpler than real-time collaborative tools:

- No operational transform (OT) or conflict-free replicated data types (CRDTs)
- No offline-first sync queues
- No peer-to-peer state reconciliation
- No merge conflicts between devices

The user is never editing the same document on two devices simultaneously. They are having a conversation (MCP) or clicking buttons (dashboard) that send stateless HTTP requests to a single server.

## State Taxonomy

### Server-Side State (Canonical)

All decision-relevant state lives on the server. Every device sees the same data when it queries.

| State | Storage | Accessed Via | Written By |
|-------|---------|-------------|------------|
| Issue status, labels, comments | Gitea (forge) | MCP `list_issues`, `get_issue` | Agents, user via MCP/dashboard |
| Tier 3 pending actions | Gitea (`status:needs-human` label) | MCP `get_digest` | Agents (create), user (resolve) |
| Tier 2 completed actions | Gitea (issue comments) | MCP `get_digest` | Agents |
| Agent sessions (active, completed) | SQLite `sessions` table | REST API, MCP `get_digest` | Dispatcher, agents |
| Cost records and budgets | SQLite `cost_records` table | MCP `get_cost_summary`, REST API | Agents (record), server (aggregate) |
| User profile | SQLite `profiles` table | MCP, REST API | User via MCP/dashboard |
| Autonomy configuration | YAML (`.samverk/autonomy.yaml`) | MCP (server reads), REST API | User via CLI/dashboard |
| Project configuration | YAML (`.samverk/server.yaml`) | Server process reads at startup | User via CLI |
| Notification routing | YAML (`.samverk/notifications.yaml`) | Server reads on dispatch | User via CLI/dashboard |
| Last check-in timestamp | SQLite (new table, see below) | MCP `get_digest` | Server (on each check-in) |
| Approval/rejection log | Gitea (issue comments) + SQLite | MCP, REST API | User via MCP/dashboard |
| Direction-setting history | Gitea (issue comments) | MCP | User via MCP |

### Device-Local State (Ephemeral)

Device-local state is disposable. Losing it causes zero data loss and at most minor inconvenience.

| State | Storage | Lifetime | Impact of Loss |
|-------|---------|----------|---------------|
| Auth token (API key) | Claude MCP config or browser storage | Until revoked | Re-enter key or re-authenticate |
| UI theme preference | Browser `localStorage` | Per-browser | Reset to default, 2-second fix |
| Dashboard filter selections | Browser `sessionStorage` | Per-tab | Reset on page load, no data loss |
| Conversation context | Claude session memory | Per-conversation | Next check-in starts fresh (by design) |
| Cached digest data | Browser memory / React Query cache | Minutes (stale-while-revalidate) | Re-fetched on next request |
| Scroll position, expanded panels | Browser DOM state | Per-page-load | Cosmetic only |

### State That Looks Device-Local But Is Not

These items might seem device-specific but must be server-side to enable cross-device continuity:

| State | Why Server-Side | Storage |
|-------|----------------|---------|
| "Last check-in" timestamp | Determines digest window for ALL devices | SQLite |
| Notification read/dismiss status | Prevents re-showing dismissed items on another device | SQLite |
| Direction-setting decisions | Must persist beyond the conversation that set them | Gitea (issue comments with structured format) |
| Batch approval preferences | Per-project, not per-device | YAML config |

## Sync Mechanism

### There Is No Sync

The word "sync" implies bidirectional state reconciliation between peers. Samverk has no peers. There is one server and N stateless clients.

```text
Phone (Monday)                    Laptop (Wednesday)
     |                                 |
     |  POST /mcp (approve PR #52)     |
     |  ------>  SAMVERK SERVER  <------|  POST /mcp (get_digest)
     |           SQLite + Gitea        |
     |           (single truth)        |
     |                                 |
     v                                 v
  Server records approval.      Server builds digest from
  Gitea issue updated.          current Gitea + SQLite state.
  Cost recorded.                Includes Monday's approvals
  Last check-in updated.        in "completed since last check-in."
```

Every MCP tool call is a stateless HTTP request. Every dashboard API call is a stateless HTTP request. The server computes fresh results on every call. There is no stale cache to invalidate across devices, no subscription to manage, and no websocket to keep alive.

### Why This Works for Samverk (and Would Not Work for Slack)

Real-time collaborative tools need sync because users expect sub-second updates across devices. Samverk's async check-in model explicitly does not:

| Property | Real-Time Tools | Samverk |
|----------|----------------|---------|
| Update latency expectation | Milliseconds | Minutes to hours |
| Concurrent editors | Common | Never (one user, sequential check-ins) |
| Offline editing | Required | Not supported (intentionally) |
| Conflict frequency | High | Zero (single writer per request) |
| State consistency model | Strong or eventual | Strong (single SQLite, serialized writes) |

The user checks in, does their 5-15 minutes, and leaves. The next check-in (same device or different) queries the server for current state. The server's state reflects everything that happened since the last check-in, regardless of which device performed which action.

## Consistency Model

**Strong consistency via SQLite serialized writes.** The Samverk server runs a single SQLite database with WAL mode. All writes are serialized through a single process. There is no replication, no eventual consistency window, and no split-brain risk.

This is the correct choice for a single-user self-hosted system. If Samverk ever supports multiple simultaneous users (beta scope), the consistency model may need to evolve. That is a future concern, not a current one.

### Read-Your-Writes Guarantee

When the user approves a PR on their phone:

1. Phone sends `approve_action` to MCP server
2. Server writes to Gitea (label change, comment) and SQLite (action log)
3. Server returns success to phone
4. Phone displays confirmation

If the user immediately opens their laptop:

1. Laptop sends `get_digest` to MCP server
2. Server queries Gitea + SQLite for current state
3. The approval from step 2 is already committed
4. Laptop sees the approval in the digest

There is no propagation delay because there is no propagation. Both devices read from the same database rows.

## "Since Your Last Check-in" Logic

### The Check-in Event

A "check-in" is any substantive interaction with the server. The server records it as a timestamped event.

```go
// CheckInEvent records when the user interacted with Samverk.
type CheckInEvent struct {
    ID        string    `json:"id"`
    Timestamp time.Time `json:"timestamp"`
    Device    string    `json:"device"`    // from API key name
    Channel   string    `json:"channel"`   // "mcp" or "dashboard"
    Actions   int       `json:"actions"`   // count of write operations
}
```

### What Triggers a Check-in Record

| Action | Triggers Check-in | Rationale |
|--------|------------------|-----------|
| `get_digest` | Yes | User is actively reviewing state |
| `approve_action` / `reject_action` | Yes | User is making decisions |
| `create_issue` / `update_issue` | Yes | User is directing work |
| `list_issues` / `get_issue` | No | Passive browsing, not engagement |
| `read_file` / `search_code` | No | Research, not check-in |
| Dashboard page load | No | Opening the browser is not a check-in |
| Dashboard Tier 3 approval button | Yes | Active decision |

### Digest Window Calculation

The digest window is the time between the previous check-in and now. This is per-user (there is only one user in alpha), not per-device.

```go
func BuildDigest(ctx context.Context, tracker forge.IssueTracker,
    store Store, checkInStore CheckInStore) (DigestData, error) {

    lastCheckIn, err := checkInStore.GetLastCheckIn(ctx)
    if err != nil {
        // First check-in ever: show last 24 hours
        lastCheckIn = time.Now().Add(-24 * time.Hour)
    }

    // All queries filter by "since lastCheckIn"
    // ...
}
```

### Cross-Device Digest Continuity

Monday phone check-in:

```text
SAMVERK: Welcome back (14h away). 2 decisions needed.

[1] merge_main: PR #52 Gitea IssueTracker -> main
    > 1 approve | 1r reject | 1? details
[2] add_dependency: Add cobra v1.9.0
    > 2 approve | 2r reject | 2? details

Cost since last check-in: $2.10 / $25.00 daily budget
```

User approves both. Server records check-in at Monday 12:05 PM.

Wednesday laptop check-in:

```text
SAMVERK: Welcome back (50h away). 1 decision needed.

--- COMPLETED SINCE LAST CHECK-IN (Monday 12:05 PM) ---
[OK] PR #52 merged to main (approved Mon 12:05)
[OK] cobra v1.9.0 added (approved Mon 12:05)
[OK] Issue #48 closed -- dispatcher heartbeat tests passing
[OK] Issue #51 QC passed -- forge retry logic

--- NEEDS YOUR INPUT ---
[1] merge_main: PR #55 Profile store -> main
    > 1 approve | 1r reject | 1? details

Cost since Monday: $8.40 / $25.00 daily budget
Active: 2 issues in progress, 3 queued
```

The laptop sees everything that happened since the phone check-in. The phone's approvals appear in the "completed" section. The server computed this fresh from Gitea + SQLite -- no state was transferred between devices.

## Conflict Resolution

### Why Conflicts Are Nearly Impossible

A conflict requires two conditions:

1. Two write operations targeting the same resource
2. Arriving before either operation's result is visible to the other

Condition 1 is rare because Samverk has a single user. Condition 2 requires simultaneous requests from two devices within the server's write latency (milliseconds for SQLite).

The only realistic scenario: the user has both their phone and laptop open, approving the same Tier 3 item from both devices within the same second.

### Conflict Rules

For the cases that could theoretically occur:

| Scenario | Resolution | Rationale |
|----------|-----------|-----------|
| Same Tier 3 item approved from two devices | First write wins, second returns "already resolved" | Idempotent approval -- approving twice is the same as approving once |
| Same Tier 3 item approved on phone, rejected on laptop | First write wins | Server processes requests sequentially; the second request finds the item already resolved |
| Direction set on phone, contradicted on laptop | Both recorded as comments; agents follow most recent | Direction is append-only -- later direction supersedes earlier. The digest shows both for audit |
| Autonomy config changed on dashboard while MCP check-in is active | Config change takes effect immediately | Next MCP tool call uses updated config. No stale config cache |

### Implementation: Idempotent State Transitions

```go
func (s *Server) ApproveAction(ctx context.Context, req ApproveRequest) error {
    issue, err := s.tracker.GetIssue(ctx, req.IssueNumber)
    if err != nil {
        return err
    }

    // Guard: already resolved (approved or rejected)
    if !hasLabel(issue, "status:needs-human") {
        return &AlreadyResolvedError{
            IssueNumber: req.IssueNumber,
            CurrentState: issue.Labels,
        }
    }

    // Proceed with approval
    // ...
}
```

The `AlreadyResolvedError` is not an error in the user-facing sense. The front-end agent or dashboard displays "This item was already approved (from your phone on Monday)" rather than a failure message.

## Cross-Device UX Flow

### Scenario: Phone Monday, Laptop Wednesday

**Monday 12:00 PM -- Phone (Claude via MCP)**

User opens Claude on their phone during lunch break. Claude calls `get_digest`.

Server builds digest from Gitea + SQLite. Two Tier 3 items are pending. User says "approve both" (phone mode: quick-glance, Tier 3 only).

Server processes:

1. Records check-in event: `{device: "herb-phone", channel: "mcp", actions: 2}`
2. Executes `approve_action` for PR #52 -- updates Gitea labels, adds comment
3. Executes `approve_action` for cobra dep -- updates Gitea labels, adds comment
4. Records cost for the MCP tool calls

User closes Claude. Total time: 3 minutes.

**Monday 12:05 PM - Wednesday 2:00 PM -- Server Working**

Dispatcher processes the unblocked work:

- PR #52 merges (Monday 12:10 PM)
- Cobra dependency added (Monday 12:11 PM)
- Issue #48 completes, QC passes, closes (Monday 8:00 PM)
- Issue #51 completes, QC passes, closes (Tuesday 3:00 PM)
- Issue #55 created by agent, needs Tier 3 approval for merge (Tuesday 5:00 PM)
- Notification sent (email) about new Tier 3 item

**Wednesday 2:00 PM -- Laptop (Dashboard)**

User opens the web dashboard in their browser. Dashboard SPA loads, calls REST API.

The dashboard shows:

- **Since last check-in (Mon 12:05)**: 4 completed items, 1 new Tier 3 pending
- **Active work**: 2 issues in progress
- **Cost since Monday**: $8.40
- **Tier 3 pending**: PR #55 Profile store merge -- approve/reject buttons

User clicks "Approve" on PR #55. Server processes identically to the phone flow.

User then opens the "Direction" panel and types: "Focus on cost tracking next. Defer the dashboard until cost controls are solid."

Server records this as a structured comment on the project's meta-issue. Agents will read this at their next task selection.

### Scenario: Concurrent Phone and Laptop (Edge Case)

User has the dashboard open on their laptop and also opens Claude on their phone. Both show the same Tier 3 item (PR #55).

1. User taps "1" on phone (approve via MCP)
2. MCP server processes approval, updates Gitea, returns success
3. 30 seconds later, user clicks "Approve" on dashboard
4. REST API calls approve endpoint, finds `status:needs-human` label already removed
5. Dashboard shows: "Already approved (phone, 30 seconds ago)"
6. Dashboard refreshes its state to reflect the approval

No data loss. No conflict. The second device gracefully handles the already-resolved state.

## Session Handoff

### There Is No Handoff

Traditional "session handoff" implies transferring in-progress state from one device to another. Samverk does not need this because:

1. **MCP conversations are stateless.** Each Claude conversation starts fresh. Context comes from the server (digest), not from a previous conversation.
2. **Dashboard state is URL-addressable.** Opening `https://samverk.local/projects/samverk` on any device shows the same project view.
3. **Direction is recorded server-side.** When the user says "focus on cost tracking" on their phone, that direction is stored as a Gitea comment. The next conversation on any device can read it.

The "continue where I left off" experience comes not from transferring state, but from the server always knowing the current state and presenting it coherently regardless of the access device.

### Orientation on Device Switch

When a user switches devices, they need to be oriented quickly. The server provides this via the digest:

```text
SAMVERK: Welcome back (50h away, last check-in: phone).

Quick summary:
- You approved 2 items on Monday (PR #52, cobra dep)
- 2 more issues completed since then
- 1 new decision needed
- Cost: $8.40 since Monday

Continue reviewing, or type "details" for the full breakdown.
```

The "(last check-in: phone)" note tells the user which device they last used, providing continuity cues without requiring any state transfer.

## New Schema Requirements

### check_in_events Table

```sql
CREATE TABLE IF NOT EXISTS check_in_events (
    id         TEXT PRIMARY KEY,
    timestamp  TEXT NOT NULL,
    device     TEXT NOT NULL,
    channel    TEXT NOT NULL,
    actions    INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_checkin_timestamp
    ON check_in_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_checkin_device
    ON check_in_events(device);
```

### notification_status Table

```sql
CREATE TABLE IF NOT EXISTS notification_status (
    id              TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL,
    status          TEXT NOT NULL,
    dismissed_at    TEXT,
    dismissed_by    TEXT,
    created_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notif_status
    ON notification_status(notification_id);
```

### Store Interface Extensions

```go
// CheckInStore tracks user check-in events across devices.
type CheckInStore interface {
    // RecordCheckIn logs a check-in event.
    RecordCheckIn(ctx context.Context, event *CheckInEvent) error
    // GetLastCheckIn returns the most recent check-in timestamp.
    GetLastCheckIn(ctx context.Context) (time.Time, error)
    // GetLastCheckInByDevice returns the most recent check-in per device.
    GetLastCheckInByDevice(ctx context.Context) (map[string]time.Time, error)
    // ListCheckIns returns check-in history within a time window.
    ListCheckIns(ctx context.Context, since time.Time) ([]*CheckInEvent, error)
}
```

## Edge Case Catalog

### E1: Offline Decision (Phone in Airplane Mode)

**Scenario**: User approves a Tier 3 item on their phone, but the phone has no connectivity.

**Behavior**: The MCP request fails (HTTP timeout or DNS failure). Claude on the phone displays an error. No state changes on the server. The Tier 3 item remains pending.

**Resolution**: User retries when connectivity returns. The server processes the approval normally. No special offline queue or retry mechanism is needed -- the user is the retry mechanism.

**Design decision**: Samverk does not support offline operations. This is intentional. Offline sync introduces an entire class of conflict resolution problems that the async model does not need. If the user cannot reach the server, they cannot check in. This is the same as being away from their project -- which is the normal state.

### E2: Stale Dashboard Tab

**Scenario**: User opens the dashboard Wednesday morning. A Tier 3 item appears. User walks away for 4 hours. During that time, the user's phone auto-checks-in via a scheduled notification interaction and the Tier 3 item is auto-approved (future feature). User returns to laptop, clicks "Approve."

**Behavior**: The dashboard's cached state is stale. The approve API call returns `AlreadyResolvedError`. The dashboard refreshes its view.

**Mitigation**: The dashboard SPA uses TanStack Query with a `staleTime` of 60 seconds. Background refetches keep the view reasonably fresh. The approve endpoint is idempotent, so stale-state clicks are harmless.

### E3: API Key Mismatch (Phone Key Has Lower Scope)

**Scenario**: The phone API key has `check-in` scope (read + Tier 3 approval). The user tries to change autonomy configuration from their phone.

**Behavior**: The MCP server rejects the `update_config` call with HTTP 403 and a message: "This API key does not have 'operate' permission. Use your desktop or laptop for configuration changes."

**Resolution**: Claude on the phone relays this to the user conversationally. The user switches to their laptop (which has `full` or `operate` scope) for the config change.

**Design note**: Different permission scopes per device is a feature, not a limitation. The phone is optimized for quick approvals. The desktop is for substantive changes. See [Security Model](security-model.md) for device permission profiles.

### E4: Long Absence (7+ Days)

**Scenario**: User goes on vacation for 10 days. Returns and checks in from their laptop.

**Behavior**: The digest window spans 10 days. The server computes:

- Completed items: potentially dozens (grouped by day)
- Tier 3 items: accumulated, some possibly stale
- Cost: total for the period
- Inactivity: the `check_in_decay` config may have paused work

**Mitigation**: The digest compresses long windows:

```text
SAMVERK: Welcome back (10 days away).

Work was paused after 72h of inactivity (per config).

--- BEFORE PAUSE (first 72h) ---
7 issues completed, 2 decisions auto-deferred
Cost: $12.30

--- DURING PAUSE (7 days) ---
No new work started. 3 Tier 3 items accumulated.

--- NEEDS YOUR INPUT ---
[1] merge_main: PR #55 (requested 8 days ago, context may be stale)
    > 1 approve | 1r reject | 1? refresh context
[2] ...

Type "resume" to restart agent work.
```

The "refresh context" option re-analyzes the Tier 3 item against the current codebase state before the user decides. This addresses the stale context risk identified in the [digest usability validation](digest-usability-validation.md).

### E5: New Device Added

**Scenario**: User gets a new tablet and wants to check in from it.

**Behavior**: User runs `samverk auth create --name "herb-tablet" --scope "check-in"` from their desktop. Configures the API key in Claude's MCP settings on the tablet. First check-in from the tablet works identically to any other device.

**Server impact**: Zero. The tablet is just another HTTP client with an API key. No device registration, no sync setup, no pairing protocol.

### E6: Device Lost or Stolen

**Scenario**: User's phone is stolen. The phone has an API key configured in Claude's MCP settings.

**Behavior**: The attacker has read access to project state and Tier 3 approval capability (if the key has `check-in` scope). They cannot modify files directly.

**Resolution**: User runs `samverk auth revoke --name "herb-phone"` from any other authenticated device. The revocation is immediate. All subsequent requests from the stolen phone receive HTTP 401.

**Cross-device impact**: Zero. Other devices continue working with their own keys. No "log out everywhere" needed because there are no sessions to invalidate.

See [Security Model](security-model.md) for the full compromised device deauthorization flow.

### E7: Concurrent Agent Work During Check-in

**Scenario**: User is in the middle of a check-in (reviewing digest, about to approve). An agent completes a task and creates a new Tier 3 item during the check-in window.

**Behavior**: The new Tier 3 item is not visible in the current digest (which was computed at the start of the check-in). The user does not see it until their next check-in or until they request a digest refresh.

**Mitigation**: Acceptable for the async model. The check-in window is 5-15 minutes. An item that appears during those minutes will be visible at the next check-in (hours later at most). For urgent items, the notification system (email/push) alerts the user independently of the digest.

**Future enhancement**: The dashboard could show a "2 new items since you loaded this page" banner using SSE (Server-Sent Events) or polling.

## What This Design Explicitly Does Not Do

- **No offline support.** If the user cannot reach the server, they cannot interact with Samverk. This is a feature, not a limitation -- it eliminates an entire class of sync complexity.
- **No real-time push to devices.** Devices poll or request state. The server does not push updates. (Future: SSE for dashboard, push notifications for urgent Tier 3 items.)
- **No device-to-device communication.** Devices never talk to each other. All communication goes through the server.
- **No conversation continuity across devices.** Each Claude conversation starts fresh. The server provides orientation, not conversation history.
- **No client-side state persistence beyond auth tokens.** If the user clears their browser, they lose nothing except having to re-open the dashboard.

## ADR-031: Server-Canonical State With Stateless Devices

### Status

Proposed

### Context

Samverk users check in from multiple devices (phone, laptop, tablet, desktop) at different times. The check-in model is async -- users interact for 5-15 minutes and leave. The question is how state synchronizes across devices.

Three approaches were considered:

1. **Client-side sync** -- devices maintain local state and sync bidirectionally (like git, Obsidian, or offline-first apps)
2. **Hybrid sync** -- server-side for decisions, client-side cache with conflict resolution (like Notion, Linear)
3. **Server-canonical** -- all state lives on the server, devices are stateless HTTP clients

### Decision

Server-canonical (option 3). All project state, check-in history, approval logs, cost data, and user direction live on the Samverk server (Gitea + SQLite). Devices store only authentication credentials and ephemeral UI preferences.

### Consequences

**Positive:**

- Zero sync complexity. No CRDTs, no operational transforms, no merge conflicts between devices
- Adding a new device is trivial (just configure an API key)
- Device loss has minimal impact (revoke key, done)
- Strong consistency guaranteed by single SQLite instance
- Aligns with MCP spec: "MCP Servers MUST NOT use sessions for authentication"
- No offline capability to test, debug, or maintain

**Negative:**

- No offline check-ins. User must have connectivity to the server
- No conversation continuity across devices (each starts fresh)
- Dashboard may show stale state if left open for hours (mitigated by TanStack Query refetch)
- Server is a single point of failure (mitigated by Proxmox HA in production, but alpha accepts this risk)

**Neutral:**

- Device permission scoping is orthogonal -- handled by API key scope, not by sync policy
- Future multi-user support would need to evolve from "single SQLite" to "SQLite with row-level access control" -- but the server-canonical model still holds

### Alternatives Rejected

**Client-side sync (option 1)** was rejected because:

- The async model means devices are never used simultaneously for the same task
- Offline editing creates conflicts that are harder to resolve than they are to prevent
- Mobile devices (phone, tablet) are consumption-oriented -- they read and approve, they do not draft or edit
- Implementation complexity is disproportionate to the single-user self-hosted use case

**Hybrid sync (option 2)** was rejected because:

- It inherits most of option 1's complexity without the offline benefit
- The latency between a server write and a client read is already milliseconds (same network, Tailscale or LAN)
- Cache invalidation is a solved problem with TanStack Query's `staleTime` and `refetchOnWindowFocus`

## Related Documents

- [MCP Server](mcp-server.md) -- stateless Streamable HTTP transport, per-device API keys
- [User Interface](user-interface.md) -- device flexibility spec, check-in model
- [Security Model](security-model.md) -- per-device API key authentication, device permission profiles
- [Digest Usability Validation](digest-usability-validation.md) -- two-mode design (quick-glance vs full session)
- [Digest Data Schema](digest-data-schema.md) -- Go types for digest assembly
- [Failure Recovery](failure-recovery.md) -- server-side state reconciliation
- [Multi-Session Safety](multi-session-safety.md) -- manual guardrails for the current development phase
- [Architecture](architecture.md) -- server architecture, single binary deployment
- [ADR-009: Device Flexibility](decisions/ADR-009-device-flexibility.md) -- non-negotiable multi-device support
- [ADR-011: Chat as Interface](decisions/ADR-011-chat-as-interface.md) -- MCP-based conversational interface
- [ADR-020: Web Dashboard](decisions/ADR-020-web-dashboard.md) -- dashboard as peer interface to MCP
