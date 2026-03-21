# Dashboard Information Architecture

## Purpose and Goals

The Samverk dashboard is the operator's window into an async background development
engine. While agents handle issues autonomously, the dashboard provides a single place
to verify that everything is running, see what the agents are currently working on, and
identify any situations that need a human decision before work can continue.

Success for an operator means opening the dashboard, answering "is everything healthy
and making progress?" within seconds, and closing it -- without needing to dig into
logs or debug anything. The dashboard is not a control plane for launching agents or
configuring the system; that work happens through git issues and config files. It is
purely an observation and triage surface.

The secondary goal is cost visibility. Because agents consume AI provider tokens on
the operator's behalf, the dashboard surfaces token spend and per-provider status so
the operator can catch runaway costs or degraded providers before they compound.

## Primary Audience

The sole audience is the solo developer who owns and operates the Samverk instance.
This person is not watching the system in real time. They check in once or a few times
a day -- from a phone, a laptop, or a second monitor -- to see whether the autonomous
pipeline is healthy and whether any issues need a human response.

The operator already knows the system architecture and does not need explanatory text
or onboarding prompts. They need compressed, high-signal information: system health in
one line, items needing attention surfaced immediately, active work visible at a glance.
Everything else is available on demand via the dedicated pages, not pushed onto the home
screen.

## Guiding Principles

- **Status before detail.** Every page answers one question -- is this subsystem healthy
  and what is it doing right now? Details (history, full log lines, cost breakdown) are
  secondary and should not compete with the status answer.

- **Async-first -- batch information, don't demand attention.** The operator is not
  monitoring 24/7. Pages should show complete snapshots, not streams that require
  watching. Auto-refresh is acceptable; push notifications that interrupt are not.

- **Attention only when action is required.** Surface the "Needs Attention" section and
  blocked-issue counts prominently. When nothing needs action, those surfaces should
  stay quiet. A clean dashboard is a signal of success, not absence of data.

- **One screen = one question answered.** Each page has a single primary question it
  answers. A page that tries to answer three questions answers none of them well.

- **No configuration on observation pages.** Pages that change system behaviour belong
  in config files and git issues. Dashboard pages read and display; they do not mutate
  state except for clearly scoped operational actions (such as relabeling an issue from
  My Queue).

## Page Inventory

| Page | Path | Purpose | Primary question | Key data |
|------|------|---------|-----------------|---------|
| Dashboard | `/` | System health overview | Is everything running and making progress? | Health banner, active workers, open issue count, cost today, provider health summary |
| Issues | `/issues` | Full issue backlog | What is the current state of all work? | Issues grouped by status label (claimed, needs-QC, needs-human, queued, blocked), search, open/closed filter |
| My Queue | `/my-queue` | Human-required issues | What do I need to decide or act on today? | Issues labeled `agent:human`, `status:needs-human`, or `status:human-pending`, sorted by priority, with copy-ready agent prompt |
| Agents | `/agents` | Agent runtime detail | How are individual agents performing? | Active workers with provider/model, recent session history per agent, token sparklines |
| Providers | `/providers` | AI provider health | Which providers are available right now? | Per-provider health dot, last-success timestamp, model-loaded status |
| Metrics | `/metrics` | Dispatcher and host telemetry | Is the system running efficiently and are resources holding? | Pool utilization, dispatcher throughput, scaling events, host CPU/RAM/disk gauges |
| Logs | `/logs` | Structured log search | What happened in a specific session or around a specific issue? | Filterable log stream by level, session ID, issue number, text search; expandable JSON fields |
| Quality | `/quality` | Agent output quality KPIs | Are agents succeeding first time and are failures improving? | First-time fix rate (FTFR), recurrence rate, mean time to first failure, root-cause breakdown pie chart |
| MCP | `/mcp` | MCP server registry | Which MCP servers are reachable and what tools do they expose? | Per-server health, endpoint, tool count, hosted vs remote type |
| Projects | `/projects` | Git forge and project config | Which forges and projects is Samverk managing? | Forge health (GitHub/Gitea), project cards with open/closed issue stats |
| Data | `/data` | Persistence layer health | Are databases and vector stores healthy and how large are they? | Per data source: health, type (SQLite, vector-db), file path, size |
| Synapset | `/synapset` | Synapset memory service | Is the memory service healthy and is it being used? | Uptime, pool stats, tool-call frequency charts, top tools by call count |
| DevKit | `/devkit` | DevKit rules and skills catalog | What rules and skills are loaded for agents? | Rules file list with sizes and entry counts, skill inventory |

## Information Hierarchy

Pages are organized into four functional groups. These groups reflect the operator's
mental model when checking in: first confirm the system is running, then see what work
is active, then dig into quality and observability if needed, then inspect configuration
as a last resort.

**Operational health** -- the first question answered on every check-in

- Dashboard: unified health banner + active work snapshot
- Agents: who is running and how they are performing
- Providers: which AI backends are available
- Metrics: resource utilization and dispatcher performance

**Work management** -- the pipeline of issues and projects

- Issues: full backlog, grouped by workflow status
- My Queue: filtered view of issues that require a human decision
- Projects: the forges and repositories being managed

**Observability** -- diagnosis and trend analysis

- Quality: KPI trends (FTFR, recurrence, root cause)
- Logs: raw structured log search and inspection
- Synapset: memory-layer usage and health

**External integrations** -- Samverk's runtime dependencies and configuration artifacts

- MCP: registered MCP servers and their tool inventories
- Data: persistence layer health and sizing
- DevKit: loaded rules files and skill catalog (read-only reference)

The sidebar navigation follows this order, with External services separated by a visual
divider because they reflect third-party services (Synapset, DevKit) rather than
Samverk internals.

## Evaluation Criteria

Before adding a new page, section, or widget, answer these questions:

1. **Who acts on this information?** If the answer is "not the operator" or "only
   during an incident that hasn't happened yet," it should not be on the dashboard.
2. **What decision or action does it enable?** If the data cannot be connected to a
   concrete action the operator could take, it is observational noise.
3. **Is this status or history?** Status belongs on the dashboard. Deep history belongs
   in a dedicated tool (Grafana, log aggregator, GitHub Insights) that is linked from
   the dashboard but not embedded in it.
4. **Does it duplicate another page?** If the same data is already visible on an
   existing page, add a link rather than a second display.
5. **Is it actionable on the timescale the operator checks in?** Data that only changes
   over weeks (license counts, total lifetime cost) belongs in a report, not a live page.
6. **Does it require configuration to be useful?** Widgets that need the operator to
   select a date range, enter a query, or pick a filter before showing anything useful
   belong on a dedicated page, not the Dashboard home screen.

## Page Design Standard

Every dashboard page should have:

- **Page heading** -- a clear `<h2>` that matches the nav label, with a
  WebSocket/polling indicator on pages that auto-refresh
- **Loading state** -- a brief loading indicator while data fetches; not a skeleton
  that mimics full page structure
- **Empty state** -- a short message explaining that there is no data yet (for example,
  "No active workers" rather than a blank grid)
- **Error state** -- a clearly visible error banner with the error message; never a
  silent blank page
- **Last-updated indicator** -- on pages showing point-in-time snapshots, a relative
  timestamp or polling cadence so the operator knows how stale the data might be

Dashboard pages should not have:

- **Configuration forms** -- changes to Samverk behaviour go in config files or git
  issues, not the dashboard
- **Raw SQL output or JSON dumps** -- data should be formatted and labeled before
  display; the Logs page is the single exception for expandable raw JSON payloads
- **Debug information** -- stack traces, internal IDs, and verbose error details belong
  in logs, not on overview pages
- **Alerts that cannot be dismissed** -- the operator should be able to acknowledge or
  navigate past any attention state; sticky banners that cannot be resolved are noise
