# KPI Framework -- Agent Self-Healing System

Defines what to measure, how to measure it, targets, and data sources for
Samverk's autonomous agent pipeline. Implements requirements from Gitea issue #116.

## Status

Approved -- 2026-03-20. Requires RCA fields from `docs/rca-standard.md`.

## KPI Set

### Agent Effectiveness

| KPI | Definition | Target | Window |
|-----|-----------|--------|--------|
| **First-Time Fix Rate (FTFR)** | % fixes with no recurrence within 30d | > 80% | Rolling 30d |
| **Fix Success Rate** | % agent sessions that close their issue | > 70% | Rolling 7d |
| **Recurrence Rate** | % issues re-opened or re-fixed within 30d | < 10% | Rolling 30d |
| **Workaround Rate** | % fixes classified as `workaround` | < 20% | Rolling 30d |
| **Mean Time to Fix (MTTF)** | Avg hours from detection to merged PR | < 4h | Rolling 7d |

### Planning Quality

| KPI | Definition | Target | Window |
|-----|-----------|--------|--------|
| **Planning Gap Rate** | % failures with root cause `planning_gap` | < 15% | Rolling 30d |
| **Unknown Root Cause Rate** | % failures with `root_cause_category = unknown` | < 20% | Rolling 30d |
| **Prevention Coverage** | % fixes with `prevention_measure != none` | > 90% | Rolling 30d |

### Detection Speed

| KPI | Definition | Target | Window |
|-----|-----------|--------|--------|
| **CI Detection Rate** | % failures caught by `ci_failure` detection method | > 85% | Rolling 30d |
| **Mean Time to Detect (MTTD)** | Avg minutes from introduction to detection | < 30m | Rolling 7d |

## Measurement Methods

### FTFR (First-Time Fix Rate)

```sql
SELECT
  CAST(SUM(CASE WHEN NOT EXISTS (
    SELECT 1 FROM failure_events f2
    WHERE f2.component = f1.component
      AND f2.root_cause_category = f1.root_cause_category
      AND f2.created_at BETWEEN f1.resolved_at AND datetime(f1.resolved_at, '+30 days')
  ) THEN 1 ELSE 0 END) AS REAL) / COUNT(*) * 100 AS ftfr
FROM failure_events f1
WHERE f1.resolved_at IS NOT NULL
  AND f1.resolved_at >= datetime('now', '-30 days');
```

### Fix Success Rate

```sql
SELECT
  CAST(SUM(CASE WHEN status = 'resolved' THEN 1 ELSE 0 END) AS REAL)
    / COUNT(*) * 100 AS fix_success_rate
FROM failure_events
WHERE created_at >= datetime('now', '-7 days');
```

### Recurrence Rate

```sql
WITH recurrences AS (
  SELECT DISTINCT f2.id
  FROM failure_events f1
  JOIN failure_events f2
    ON f2.component = f1.component
   AND f2.root_cause_category = f1.root_cause_category
   AND f2.id != f1.id
   AND f2.created_at > f1.resolved_at
   AND f2.created_at <= datetime(f1.resolved_at, '+30 days')
  WHERE f1.resolved_at >= datetime('now', '-30 days')
)
SELECT
  CAST(COUNT(DISTINCT r.id) AS REAL)
    / NULLIF((SELECT COUNT(*) FROM failure_events WHERE created_at >= datetime('now', '-30 days')), 0)
    * 100 AS recurrence_rate
FROM recurrences r;
```

### Root Cause Breakdown

```sql
SELECT root_cause_category, COUNT(*) AS count,
  CAST(COUNT(*) AS REAL) / SUM(COUNT(*)) OVER () * 100 AS pct
FROM failure_events
WHERE created_at >= datetime('now', '-30 days')
GROUP BY root_cause_category
ORDER BY count DESC;
```

## Data Sources

| Field | Source |
|-------|--------|
| `failure_events.created_at` | When failure detected |
| `failure_events.resolved_at` | When fix merged |
| `failure_events.root_cause_category` | RCA field (see rca-standard.md) |
| `failure_events.fix_classification` | RCA field |
| `failure_events.prevention_measure` | RCA field |
| `failure_events.detection_method` | RCA field |
| `failure_events.component` | Package/module where failure occurred |
| `failure_events.issue_number` | Linked Gitea issue |

## Quality Page Layout

Surface these KPIs on the `/quality` dashboard page:

### Row 1 -- Headline KPI Cards (4)

- FTFR: `XX%` with green (>80) / amber (60-80) / red (<60) indicator
- Fix Success Rate: `XX%` with trend arrow (7d change)
- MTTF: `Xh Xm` with 30d average
- Recurrence Rate: `X.X%` with target `<10%` indicator

### Row 2 -- Root Cause Breakdown (pie/donut, 30d)

Segments by `root_cause_category`. Hovering shows count + %.

### Row 3 -- Fix Classification Trend (stacked bar, last 12 weeks)

Weekly breakdown of `permanent` / `workaround` / `partial` fixes.

### Row 4 -- Advisory Panel (future: issue #120)

Pattern detection recommendations from the advisory engine.

## Alert Thresholds

| Condition | Action |
|-----------|--------|
| FTFR < 60% for 7d | File issue `status:needs-human` with analysis |
| Recurrence Rate > 20% for 3d | File issue with recurring component name |
| Planning Gap Rate > 25% | Trigger prompt quality review |
| Unknown Rate > 30% | Flag: RCA discipline is degrading |
