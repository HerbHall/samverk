# RCA Documentation Standard

Structured root cause analysis fields for every bug fix in Samverk.
Enables recurrence detection, KPI computation, and systemic gap identification.

## Status

Approved -- 2026-03-20. Implements requirements from Gitea issue #115.

## RCA Fields

Every `failure_events` row must capture the following fields when a fix is recorded:

### 1. Root Cause Category (required)

```text
planning_gap        -- Agent misunderstood the spec or skipped analysis
implementation_error -- Correct plan, wrong code (logic error, typo, off-by-one)
environment_issue   -- External system, config, or infra problem
spec_ambiguity      -- Issue description was unclear or contradictory
dependency_failure  -- Upstream library, API, or service failure
regression          -- Change broke previously working behaviour
unknown             -- Cannot determine root cause
```text

### 2. Fix Classification (required)

```text
permanent   -- Root cause eliminated; failure cannot recur in the same way
workaround  -- Symptom suppressed but root cause remains
partial     -- Root cause partially addressed; recurrence risk remains
```text

### 3. Prevention Measure (required)

```text
test_added        -- Regression test added that would have caught this
lint_rule         -- Lint or static analysis rule added
rule_file_update  -- DevKit rules or known-gotchas updated
issue_filed       -- Tracking issue filed for systemic fix
none              -- No prevention measure applicable or taken
```text

### 4. Recurrence Risk (required)

```text
low    -- Isolated incident; unlikely to recur
medium -- Pattern likely to recur without prevention measure
high   -- Systemic issue; will recur without structural fix
```text

### 5. Detection Method (required)

```text
ci_failure         -- Caught by automated CI (build, lint, test)
human_observation  -- Spotted by human during review
monitoring_alert   -- Triggered by health check or alert
agent_self_report  -- Agent identified its own error during a session
```text

### 6. Time to Detect (optional)

Duration from when the defect was introduced (commit timestamp) to when it was
detected. Omit if introduction time is unknown. Stored as integer minutes.

### 7. Linked Issues (optional)

Comma-separated Gitea issue numbers of prior issues sharing the same root cause.
Used for recurrence detection queries.

## Storage

All fields stored in the `failure_events` SQLite table (see issue #117 for schema
migration). Agents populate these fields via the MCP `record_failure` tool call
(to be added) or via structured labels on the closing PR.

## Agent Guidance

When an agent closes an issue, it should fill in these fields as part of the
session wrap-up. If the agent cannot determine a field, it uses `unknown` or
omits optional fields. The dispatcher validates that required fields are present
before marking a session as complete with RCA.

## Recurrence Detection

An issue is a recurrence if:

1. Its `root_cause_category` matches a prior failure within the same component
2. The prior failure's `fix_classification` was `workaround` or `partial`
3. The gap between failures is less than 90 days

Recurrence rate = (recurrent failures in 30d) / (total failures in 30d).
Target: < 10%.
