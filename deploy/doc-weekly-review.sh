#!/usr/bin/env bash
# doc-weekly-review.sh -- Weekly documentation freshness and consistency review.
# Runs via systemd timer on CT 202. Uses claude --print with Samverk + Synapset MCP.
set -euo pipefail

LOG_DIR="/var/lib/samverk/.samverk/logs"
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/doc-review-$(date +%Y%m%d).log"

PROMPT='You are the weekly documentation review agent for the Toolkit projects (DevKit, Samverk, Synapset, OpsKit).

## Step 1: Check Stale Documents

Query the Synapset docs pool for entries older than 14 days:
- Use search_memory(pool="docs", created_before="'"$(date -d '14 days ago' +%Y-%m-%d)"'", limit=50, format="json")
- For each stale entry, check if the source file was modified since indexing using git log

## Step 2: Cross-Project Consistency Check

For key documents (CLAUDE.md, architecture.md) in each project, check for contradictions:
- Use find_contradictions(pool="docs", query="<key claims from each doc>", threshold=0.85)
- Focus on: build command consistency, architecture claims vs actual structure, cross-references

## Step 3: Create Issues for Findings

Before creating any issue, search for existing open issues to avoid duplicates:
- Use search_issues(query="<finding>", labels=["doc:stale"])
- Only create new issues via Samverk MCP create_issue if no match exists
- Labels: doc:stale, doc:inconsistent, or doc:structure as appropriate
- Title format: "docs: <specific finding>"

## Step 4: OpsKit Runbook Validation

Check that script paths and SSH targets in opskit runbooks still exist and match topology.md.

## Step 5: Summary

Store a summary in Synapset for audit trail:
- store_memory(pool="docs", content="Weekly review '"$(date +%Y-%m-%d)"': <N> stale, <M> inconsistent, <K> issues created", category="general", tags="weekly-review", source="doc-weekly-review")

Report total findings.'

echo "=== Doc Weekly Review: $(date) ===" >> "$LOG_FILE"
claude --print --model claude-sonnet-4-6 "$PROMPT" >> "$LOG_FILE" 2>&1
echo "=== Review Complete: $(date) ===" >> "$LOG_FILE"
