# Spike #11: Manual Check-in Workflow Prototype

## Verdict: Design Validated, Label Taxonomy Required

The digest data model works. `BuildDigest` successfully queries a real GitHub repo, parses frontmatter, sorts by priority, groups completed work by day, and produces conversational output. The core check-in workflow -- decisions first, awareness second, status third -- is sound.

## What Was Built

- `internal/digest/digest.go` -- `DigestData` types and `BuildDigest` function
- `internal/digest/render.go` -- `FormatDigest` text renderer (conversational format)
- `internal/digest/digest_test.go` -- 7 test cases with mock `IssueTracker`
- `samverk digest` CLI command -- connects to real GitHub, renders live digest

## What Works

### Data Model

The `DigestData` struct from `docs/digest-data-schema.md` maps cleanly to `IssueTracker.ListIssues` queries. The five-query pattern (needs-human, closed, in-progress, queued, blocked) covers all digest sections without redundancy.

### Frontmatter Parsing

`pkg/models.ParseFrontmatter` integrates directly into `BuildDigest`. Fields extracted: `type`, `priority`, `agent_type`, `actual_tokens`, `model_used`, `depends_on`. The parser handles issues with and without frontmatter gracefully (nil Frontmatter, no error).

### Priority Sorting

Critical items surface before high, which surface before normal. Within the same priority, oldest items appear first. This matches the design intent: most urgent and longest-waiting items get user attention first.

### Conversational Format

The renderer produces the exact format from `docs/check-in-digest-design.md`:

```text
SAMVERK: Welcome back. You've been away 14h. Here's where things stand.

--- NEEDS YOUR DECISION (1 item, blocking work) ---

[1] MERGE_MAIN: Merge PR #52
    Why: QC passed, all tests green.
    Blocks: 2 dependent issues
    Waiting: 6h
    > 1 approve | 1r reject | 1? more context

--- COMPLETED AUTONOMOUSLY (1 action since last check-in) ---

Today:
- TASK #47: Research webhooks

--- STATUS ---

Active: 1 issue in progress (#50 Gitea adapter)
Queued: 7 issues waiting
Blocked: 2 issues (dependency, not user)
Cost: ~$1.42 (28k tokens) since last check-in | $48.58 remaining
```

### Edge Cases

- Empty repo shows first-check-in onboarding message
- No pending actions shows "No decisions needed" and skips to Tier 2
- Issues without frontmatter still appear (with empty action type and default priority)
- Completed actions grouped by day (Today, Yesterday, Earlier)

## Gaps Identified

### 1. Label Taxonomy Not Yet Applied

The digest relies on `status:` labels (`needs-human`, `in-progress`, `queued`, `blocked`) that do not exist on the current Samverk or SubNetree repos. Current issues use `priority:` and `type:` labels only.

**Action needed:** Before the digest is useful in production, issues must be labeled with the status taxonomy from `docs/communication-protocol.md`. The dispatcher should apply these labels as issues move through the lifecycle.

### 2. Cost Store Not Connected

`BuildDigest` accepts a `CostSource` interface but the spike passes nil. Cost data shows as "no cost data available" in the output. The `internal/store/` SQLite layer has `CostRecord` and `CostSummary` types but no `ComputeCostSince` query yet.

**Action needed:** Implement `ComputeCostSince` in the store layer when connecting the cost tracking pipeline.

### 3. Dependent Counting Is Expensive

`countDependents` queries ALL open issues for every pending action to find `depends_on` references. For a repo with N open issues and M pending actions, this is O(N*M) API calls.

**Action needed:** When the store layer is connected, build a dependency index. For the spike with <100 issues, the brute-force approach is acceptable.

### 4. Section Extraction Is Fragile

`extractSection` looks for `## SectionName` headings and grabs content until the next heading. This works for well-structured issue bodies but fails when:

- The section heading uses different casing or wording
- Content is in the frontmatter rather than a markdown section
- The issue body has nested headings within a section

**Action needed:** Consider standardizing on frontmatter fields for structured data (context, result summary) and reserving markdown sections for human-readable detail only.

### 5. No Device Adaptation Yet

The renderer always produces the full digest. The compact format for mobile (Tier 3 only + cost one-liner) is not implemented. This is expected for the spike.

**Action needed:** The front-end agent (Claude + MCP) will handle device adaptation based on user profile preferences. The `FormatDigest` function may need a format parameter or the adaptation may happen entirely in the agent's prompt.

## Recommendations for Next Steps

1. **Label taxonomy setup** -- Create the `status:` labels on the Samverk repo and apply them to existing issues. This enables live testing of the digest command.

2. **MCP tool exposure** -- Wrap `BuildDigest` as the `samverk_get_digest` MCP tool. This is the primary interface for the front-end agent.

3. **Store integration** -- Connect `ComputeCostSince` for real cost data. Add a dependency index table for efficient dependent counting.

4. **Front-end agent prompt** -- Write the system prompt for the Claude front-end agent that uses the digest MCP tools. This is where device adaptation, quick-action parsing, and direction-setting logic live.

5. **Live test** -- Run `samverk digest` against a repo with properly labeled issues and evaluate the output with real data.
