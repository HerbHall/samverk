# Mobile Experience

## Platform Decision: Responsive Web

No native app. No PWA. The web dashboard (`web/`) is built as a responsive React SPA that works on all screen sizes. This is the simplest path that satisfies the device flexibility requirement (ADR-009).

### Why Responsive Web

| Factor | Responsive Web | PWA | React Native |
|--------|---------------|-----|-------------|
| Codebase | Single (existing React dashboard) | Single + service worker | Separate |
| Build effort | CSS/layout only | Moderate (SW, manifest) | High |
| Push notifications | No (use email) | Unreliable on iOS | Yes |
| Offline | No | Yes | Yes |
| Install friction | Zero (open browser) | Low (add to home) | App store |
| Maintenance | Zero additional | Low | High |

The email notification system eliminates the primary reason for native/PWA (push notifications). Offline is not needed -- the async model means the user checks in when they have connectivity.

### What This Means

- No app store submissions, no native build tooling, no platform-specific code
- Tailwind CSS responsive breakpoints handle layout adaptation
- shadcn/ui components are responsive by default
- Test on Chrome mobile, Safari iOS, Firefox Android

## Notification System

### Architecture: Pluggable Interface

Email is the first implementation. The interface is abstracted for future channels.

```go
// internal/notify/notify.go
type Notifier interface {
    // Send delivers a notification to the user.
    Send(ctx context.Context, notification Notification) error
    // Channel returns the channel type (email, push, slack, etc.)
    Channel() string
}

type Notification struct {
    Type     NotificationType // digest, tier3_approval, alert, health
    Priority Priority         // low, normal, high, critical
    Subject  string
    Body     string
    Actions  []Action         // approve/reject/defer deep links
    Project  string
}

type Action struct {
    Label string
    URL   string // deep link to approval page
    Type  string // approve, reject, defer, review
}
```

### Email Notification Strategy

Notifications are configurable per project and per priority level:

| Notification Type | Default Schedule | Configurable |
|-------------------|-----------------|-------------|
| Daily digest | Once daily (configurable time) | Time of day, frequency |
| Tier 3 approval | Immediate email | Can change to batched |
| Tier 2 completion | Included in digest | Can promote to immediate |
| System health alert | Immediate | Cannot disable |
| Budget warning | Immediate | Threshold configurable |

### Per-Project Configuration

```yaml
# .samverk/notifications.yaml
defaults:
  digest_schedule: "daily"
  digest_time: "08:00"
  tier3_delivery: "immediate"

projects:
  samverk:
    digest_schedule: "twice_daily"
    digest_times: ["08:00", "20:00"]
    tier3_delivery: "immediate"

  side-project:
    digest_schedule: "weekly"
    tier3_delivery: "batched_daily"

overrides:
  # Priority-based overrides within a project
  - project: "samverk"
    priority: "critical"
    delivery: "immediate"
  - project: "side-project"
    priority: "high"
    delivery: "immediate"
```

### Future Notification Channels

The `Notifier` interface supports adding channels without changing core logic:

| Channel | Use Case | Feasibility | Priority |
|---------|----------|-------------|----------|
| Email (SMTP) | Primary channel | High -- standard SMTP | Alpha |
| Slack webhook | Team/personal workspace | High -- simple HTTP POST | Beta |
| Discord webhook | Community/personal | High -- same as Slack | Beta |
| Ntfy.sh | Self-hosted push | High -- simple HTTP POST | Beta |
| Gotify | Self-hosted push | High -- REST API | Beta |
| Matrix | Self-hosted chat | Medium -- SDK needed | Post-beta |
| Browser push (Web Push API) | In-browser alerts | Medium -- requires VAPID keys | Post-beta |
| SMS (Twilio) | Critical alerts only | Medium -- paid service | Post-beta |
| Native push (APNs/FCM) | Would require native app | Low -- breaks responsive-only approach | Not planned |

**Self-hosted push recommendations:** Ntfy.sh and Gotify are the best fits for the target user (self-hoster). Both are simple HTTP POST APIs, open source, and run as Docker containers alongside Samverk.

## Mobile Information Hierarchy

When the user opens Samverk on their phone with 5 minutes to spare, the screen displays:

### Priority Order

1. **Blocked items** -- what needs MY input right now (count badge, sorted by priority)
2. **Quick actions** -- approve/reject/defer buttons inline with each blocked item
3. **Change summary** -- natural language description of what changed, not raw diffs
4. **QC results** -- test pass/fail counts, lint status (enough to judge without reading code)
5. **In progress** -- what agents are currently working on (collapsed by default)
6. **Completed** -- what finished since last check-in (collapsed by default)
7. **Cost** -- spend since last check-in, budget remaining (footer or collapsible)

### Mobile Layout

```text
┌─────────────────────────┐
│ Samverk          💰 $2.40│
│ 3 projects    ← unified │
├─────────────────────────┤
│ ⚠ 2 items need input    │
│                         │
│ [samverk] PR #45        │
│ Add auth middleware      │
│ QC: ✓ 12 tests, 0 lint  │
│ Summary: Adds JWT...    │
│ [Approve] [Defer] [More]│
│                         │
│ [side-project] Direction│
│ Database choice needed   │
│ SQLite vs Postgres?     │
│ [Reply] [Defer]         │
├─────────────────────────┤
│ ▸ 3 in progress         │
│ ▸ 5 completed today     │
└─────────────────────────┘
```

### Code Review on Mobile

Diffs are NOT shown on mobile by default. Instead:

- **Summary**: Natural language description of the change ("Adds JWT authentication middleware to all API routes. 3 files modified, 150 lines added.")
- **QC results**: Test count, pass/fail, lint status, coverage delta
- **Approval decision**: Based on summary + QC results. If uncertain, tap "Defer to desktop"

The user can optionally expand to see the raw diff, but the UI does not encourage this on small screens.

## Input Methods

### Quick Action Buttons

For queued decisions (approvals, direction choices):

- **Approve** -- accept the work, agent proceeds to next task
- **Reject** -- agent revises (with optional text feedback)
- **Defer** -- skip for now, revisit on desktop or next check-in
- **More** -- expand to see diff, full QC report, agent reasoning

### Text Chat

For nuanced direction:

- Natural language input field at bottom of screen
- "Focus on the auth module this week"
- "Skip the frontend for now, backend is priority"
- "Use PostgreSQL, not SQLite for this project"

### Voice Input

For hands-free direction:

- Microphone button in the input field
- Browser's built-in Web Speech API (no additional dependency)
- Transcribed to text, sent as chat message
- User confirms before sending (prevent misinterpretation)

```typescript
// Voice input using Web Speech API
const recognition = new webkitSpeechRecognition();
recognition.continuous = false;
recognition.interimResults = true;
recognition.lang = 'en-US';

recognition.onresult = (event) => {
  const transcript = event.results[0][0].transcript;
  setDraftMessage(transcript); // user reviews before sending
};
```

**Limitations:**

- Web Speech API requires online connectivity (sends audio to Google/Apple for transcription)
- Not available in all browsers (Firefox support is limited)
- Privacy concern: audio transits to browser vendor's servers
- Fallback: always show text input alongside voice button

## Multi-Project Mobile View

The mobile view is multi-project by default:

### Unified Summary (Default View)

Shows blocked items across all projects, sorted by priority. Each item tagged with project name. The user sees everything that needs attention in one scroll.

### Per-Project Deep Dive

Tap a project name to see its full status:

- Blocked items for this project
- In-progress tasks
- Completed since last check-in
- Cost breakdown
- Agent activity log

### Project Switcher

Horizontal scroll or dropdown at the top of the screen. Shows project name + blocked count badge.

## Competitive Mobile UX Review

### GitHub Mobile

- **Good**: PR review with inline comments, notification triage, issue management
- **Bad**: Code review on phone is painful. Diff view is cramped. No batch actions.
- **Steal**: Notification triage UX (swipe to dismiss, batch mark as read)

### Linear Mobile

- **Good**: Clean issue management, keyboard shortcuts, fast filtering
- **Bad**: No code integration on mobile. Status-update-only tool on phone.
- **Steal**: Information density without clutter. One-tap status changes.

### Jira Mobile

- **Good**: Nothing outstanding
- **Bad**: Slow, cluttered, too many taps to do anything
- **Avoid**: Everything. The opposite of what Samverk should be on mobile.

### Key Takeaway

The best mobile project management UX is **notification triage** -- not full feature parity with desktop. Show what needs attention, let the user act on it quickly, get out of the way.

## Responsive Breakpoints

Using Tailwind CSS breakpoints:

| Breakpoint | Width | Layout |
|-----------|-------|--------|
| `sm` | 640px+ | Single column, larger touch targets |
| `md` | 768px+ | Two-column where beneficial |
| `lg` | 1024px+ | Full dashboard layout |
| `xl` | 1280px+ | Extended sidebar, code diff view |

### Mobile-Specific Adaptations

- Navigation: bottom tab bar (not sidebar)
- Touch targets: minimum 44px (Apple HIG)
- Swipe gestures: left/right on blocked items for approve/reject
- Pull-to-refresh: reload digest data
- No hover states (touch-only interactions)

## ADR-032: Responsive Web for Mobile Experience

### Decision

Use responsive web design for mobile support. No native app, no PWA. Email-based notification system with pluggable interface for future channels.

### Context

ADR-009 declares device flexibility non-negotiable. The check-in workflow must work on phones. The question is platform approach.

### Options Considered

1. **React Native** -- native feel, reliable push, offline. Separate codebase, high maintenance.
2. **PWA** -- installable, offline-capable. Push unreliable on iOS. Service worker complexity.
3. **Responsive web (chosen)** -- single codebase, zero install, zero maintenance overhead.
4. **Flutter** -- cross-platform native. Learning a new framework for mobile only.

### Consequences

**Positive:**

- Zero additional codebase to maintain
- Existing React/shadcn/ui components work with responsive CSS
- No app store submissions or review processes
- Voice input via Web Speech API is sufficient for hands-free
- Email notifications cover the primary notification use case

**Negative:**

- No push notifications (mitigated by email with deep links)
- No offline access (acceptable -- async model assumes connectivity for check-ins)
- No native feel (swipe gestures via JavaScript, not OS-level)
- Voice input depends on browser support and sends audio to vendor servers

**Neutral:**

- Self-hosted push (ntfy.sh, Gotify) can be added later via the pluggable notifier interface without changing the mobile approach
- If native push becomes critical, a thin native wrapper (Capacitor/Tauri Mobile) could wrap the responsive web app later

### Review Trigger

Re-evaluate if:

- iOS Safari restricts Web Speech API
- Email delivery reliability becomes a problem (spam filters, delays)
- User research shows push notifications are essential for Tier 3 approvals
- A second developer joins and can maintain a native codebase
