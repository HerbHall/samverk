---
name: session-wrap
description: End-of-session Synapset memory sync. Scans the conversation for infrastructure state changes, diffs against current Synapset state, and writes updates. Use when wrapping up a working session to keep machine specs, Ollama models, and project state current.
allowed-tools: mcp__synapset__query_memory, mcp__synapset__search_memory, mcp__synapset__store_memory, mcp__synapset__update_memory, mcp__synapset__bulk_update
---

# Session Wrap — Synapset Memory Sync

> Keeps Synapset current at session end. Detects infrastructure changes from the conversation, diffs against stored state, and writes targeted updates.

---

## Trigger Phrases

Activate this skill when the user says any of:

- "wrap up session"
- "end of session"
- "sync memory"
- "update synapset"
- "session complete"
- "sync synapset from this session"

---

## Step 1 — Scan the Conversation for Changes

Review the full conversation for signals that infrastructure state has changed. Do NOT infer or assume — only capture things that were explicitly confirmed in the conversation.

### Change Detection Signals

| Signal Pattern | Pool | Source Key |
|---|---|---|
| Ollama model pulled / confirmed running on a host | `machines` | hostname |
| Service version confirmed / upgraded | `machines` | hostname |
| LAN IP confirmed for a host | `machines` | hostname |
| Disk resized / storage expanded | `machines` | hostname |
| Native Ollama install confirmed working | `machines` | hostname |
| New Samverk ADR / architecture decision | `samverk` | `session-YYYY-MM-DD` |
| New pattern or gotcha discovered | `devkit` | `session-YYYY-MM-DD` |
| Issue created, closed, or resolved with a notable outcome | `samverk` | `session-YYYY-MM-DD` |

### What to IGNORE

- Routine forge work (code reviews, PR merges, issue triage) — in forge history already
- Speculative state ("I think the model might be...")
- Unconfirmed IPs or versions
- Transient state (CPU load, running processes, current memory usage)
- Changes you created (issues, PRs) — these are in the forge, not infrastructure state

---

## Step 2 — Diff Against Current Synapset State

For each detected change, find the existing memory entry:

```text
query_memory(pool="machines", source="<hostname-lowercase>")
```

Read the returned entries and identify which one covers the changed field.

**Source naming conventions:**

- `hdh-nzxt` — primary desktop
- `vm-300` — Proxmox VM 300 (Ollama)
- `proxmox` — Proxmox host
- `cm-asus` — secondary workstation
- `hdh-dell` — Dell machine

If no entry covers the changed field, it needs a new `store_memory` call.

---

## Step 3 — Classify Each Change

For each detected signal, classify it into one of three categories:

### CORRECTION

Content in memory is **wrong or stale**. Requires a full content rewrite + re-embedding.

Example: Memory says "target model qwen2.5-coder:32b" but this session confirmed qwen3-coder:30b is running.

Action: `update_memory(id=X, content="<full rewrite>", tags="...,updated:YYYY-MM-DD")`

### CONFIRMATION

Content in memory is **correct** — this session verified it. No content change needed, but tags updated to reset the staleness clock.

Example: VM 300 memory says qwen2.5-coder:32b. This session confirmed that's correct.

Action: `update_memory(id=X, tags="...,validated:YYYY-MM-DD")` — content unchanged, only tags updated.

### NEW ENTRY

No existing memory covers this topic. Requires a new `store_memory` call.

---

## Step 4 — Propose Changes

Before writing anything, present a summary to the user:

```text
Session Memory Sync — proposed changes:

CORRECTIONS (content rewrite):
  [#484] machines/hdh-nzxt — Ollama: "broken/WSL2, target qwen2.5-coder:32b" → "native Windows working, qwen3-coder:30b"
  [#505] machines/hdh-nzxt — GPU note: stale model reference → qwen3-coder:30b

CONFIRMATIONS (tags only, no content change):
  [#585] machines/vm-300 — qwen2.5-coder:32b confirmed current → add validated:2026-03-18

NEW ENTRIES:
  (none this session)

NO ACTION:
  machines/proxmox — no state changes this session

Proceed? (yes / skip / edit)
```

Wait for confirmation before writing.

---

## Step 5 — Execute Writes

On confirmation:

### For confirmations (tags only)

```text
update_memory(
  id=<ID>,
  tags="<existing-tags>,validated:YYYY-MM-DD"
)
```

Content is NOT changed — this preserves the embedding. Only the staleness signal is updated.

### For corrections (content rewrite)

Use `update_memory` with the memory ID, new content, and updated tags:

```text
update_memory(
  id=<ID>,
  content="<full updated content — rewrite the entire entry, not just the changed field>",
  tags="updated:YYYY-MM-DD,<topic-tags>",
  summary="<one-line summary of what changed>"
)
```

When multiple entries in the same pool need updating, use `bulk_update` instead — one call is more efficient.

### For new entries

```text
store_memory(
  pool="<pool>",
  source="<source>",
  category="<category>",
  content="<content>",
  tags="updated:YYYY-MM-DD,<topic-tags>",
  summary="<one-line summary>",
  deduplicate=true,
  dedup_threshold=0.85
)
```

### Category conventions

| Data type | Category |
|---|---|
| Machine hardware / software state | `general` |
| DevKit patterns, gotchas, CI quirks | `gotcha` |
| Architecture decisions | `decision` |
| Session summaries | `general` |
| Corrections to previous wrong info | `correction` |

### Tag conventions

Always include `updated:YYYY-MM-DD`. Add topic tags:

| Topic | Tags |
|---|---|
| Ollama models | `ollama,models` |
| Network / IP | `network` |
| Storage / disk | `storage` |
| GPU / hardware | `gpu,hardware` |
| Service version | `version,<service-name>` |
| Session summary | `session,YYYY-MM-DD` |

---

## Step 6 — Report

After all writes complete:

```text
Session sync complete:
  2 corrections (content rewrite + re-embed)
  1 confirmation (tags only, staleness reset)
  1 new entry stored
  1 entry already current, no action
```

---

## Full Content Rewrites

When updating a memory, always rewrite the **entire** content block — do not append or patch. Synapset re-embeds on content change, so partial updates produce incorrect embeddings.

**Wrong:** Append "Updated 2026-03-18: model is now qwen3-coder:30b" to the end.

**Right:** Rewrite the entire entry with current facts, remove stale information.

---

## Example — Updating an Ollama Model Entry

Session contained: "HDH-NZXT native Ollama is running qwen3-coder:30b."

Existing memory #484:
> "HDH-NZXT Ollama is installed but GPU inference was broken due to NVIDIA Container Toolkit/WSL2 issues — fix is native Windows Ollama installation. Target model: qwen2.5-coder:32b."

Correct update:

```text
update_memory(
  id=484,
  content="HDH-NZXT Ollama: native Windows installation (not WSL2/Docker). GPU inference working on RTX 5090. Current model: qwen3-coder:30b. Ollama listens on 192.168.1.202:11434 (LAN). Used for Samverk triage/docs/research routing.",
  tags="updated:2026-03-18,ollama,models,gpu",
  summary="Native Windows Ollama confirmed working, qwen3-coder:30b running"
)
```
