# Issue Batch Format

## Overview

The issue batch format is a JSON file that defines multiple GitHub issues for bulk creation. It enables a workflow where you brainstorm tasks in a Claude conversation, export them as structured JSON, and ingest them into GitHub with a single command.

This solves the problem of translating free-form planning into properly formatted Samverk issues with YAML frontmatter, labels from the taxonomy, and milestone assignments.

## JSON Schema

A batch file has two top-level keys: `metadata` (project-level defaults) and `issues` (array of issue definitions).

```json
{
  "metadata": {
    "project": "owner/repo",
    "milestone": "Phase 0: Foundation",
    "schema_version": "1.0.0"
  },
  "issues": [
    {
      "title": "Issue title",
      "labels": ["agent:code-gen", "type:task", "priority:normal"],
      "milestone": "Phase 0: Foundation",
      "frontmatter": {
        "schema_version": "1.0.0",
        "type": "task",
        "agent_type": "code-gen",
        "priority": "normal",
        "parent_issue": null,
        "depends_on": [],
        "estimated_tokens": 2000
      },
      "sections": {
        "summary": "One sentence description.",
        "context": "What the agent needs to know.",
        "acceptance_criteria": ["Criterion 1", "Criterion 2"]
      }
    }
  ]
}
```

## Field Reference

### Metadata

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `project` | string | Yes | GitHub owner/repo (e.g., `HerbHall/samverk`) |
| `milestone` | string | No | Default milestone title for issues that omit their own |
| `schema_version` | string | Yes | Batch format version (`1.0.0`) |

### Issue

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `title` | string | Yes | Issue title |
| `labels` | string[] | Yes | Labels to apply (must exist in repo) |
| `milestone` | string | No | Milestone title (overrides metadata default) |
| `frontmatter` | object | Yes | YAML frontmatter fields for the issue body |
| `sections` | object | Yes | Markdown body sections |

### Frontmatter Fields

These map directly to the Samverk issue schema defined in [communication-protocol.md](communication-protocol.md).

| Field | Type | Required | Valid Values |
| ----- | ---- | -------- | ------------ |
| `schema_version` | string | Yes | `1.0.0` |
| `type` | enum | Yes | `task`, `question`, `result`, `block` |
| `agent_type` | enum | Yes | `orchestrator`, `dispatcher`, `code-gen`, `test`, `docs`, `research`, `qc`, `human` |
| `priority` | enum | Yes | `critical`, `high`, `normal`, `low` |
| `parent_issue` | int | No | Issue number of the parent in the decomposition tree |
| `depends_on` | int[] | No | Issue numbers that must close before this starts |
| `estimated_tokens` | int | No | Token budget estimate for the work |

### Sections

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| `summary` | string | Yes | One sentence human-readable description |
| `context` | string | Yes | What the agent needs to know -- files, decisions, parent issue |
| `acceptance_criteria` | string[] | Yes (for `task`) | Specific, testable conditions rendered as a checklist |

## Label Taxonomy

Labels must exist in the repository before the script runs. The script validates all labels and warns on missing ones.

### Agent Type (who should pick this up)

- `agent:orchestrator` -- High-level task decomposition and planning
- `agent:dispatcher` -- Issue routing and dependency management
- `agent:code-gen` -- Code generation and implementation
- `agent:test` -- Test writing and execution
- `agent:docs` -- Documentation generation
- `agent:research` -- Research and analysis tasks
- `agent:qc` -- Quality control validation
- `agent:human` -- Requires user input or decision

### Status (lifecycle state)

- `status:queued` -- Ready to be picked up
- `status:claimed` -- Agent has claimed, work starting
- `status:in-progress` -- Active work underway
- `status:blocked` -- Waiting on dependency
- `status:needs-qc` -- Awaiting QC validation
- `status:needs-human` -- Escalated to user
- `status:done` -- Complete and validated

### Priority

- `priority:critical` -- Blocks multiple work streams
- `priority:high` -- Important, schedule next
- `priority:normal` -- Standard priority
- `priority:low` -- Nice to have

### Complexity (routing hint)

- `complexity:local` -- Safe to run on local model
- `complexity:cloud` -- Requires cloud model
- `complexity:ambiguous` -- Dispatcher needs to evaluate

### Type

- `type:task` -- Implementation work
- `type:question` -- Needs a decision or answer
- `type:result` -- Output from completed work
- `type:block` -- Work is blocked

## Usage

Create issues from a batch file:

```bash
bash scripts/create-issues.sh scripts/sample-batch.json
```

Preview what would be created without actually creating issues:

```bash
bash scripts/create-issues.sh --dry-run scripts/sample-batch.json
```

The script:

1. Detects the GitHub repo from the git remote
2. Resolves milestone titles to milestone numbers via the GitHub API
3. Validates that all labels in the batch exist in the repo
4. For each issue, assembles YAML frontmatter + markdown body and calls `gh issue create`
5. Prints a summary of created and failed issues

## Claude Prompt Template

Use this prompt to generate a batch file from a brainstorming conversation. Paste it into a Claude conversation along with your project context and task ideas.

````text
Generate a Samverk issue batch JSON file from the tasks we discussed.

Use this exact JSON structure:

```json
{
  "metadata": {
    "project": "HerbHall/samverk",
    "milestone": "<milestone title or null>",
    "schema_version": "1.0.0"
  },
  "issues": [
    {
      "title": "<concise imperative title>",
      "labels": ["agent:<type>", "type:<type>", "priority:<level>"],
      "milestone": "<milestone title or null>",
      "frontmatter": {
        "schema_version": "1.0.0",
        "type": "task|question|result|block",
        "agent_type": "orchestrator|dispatcher|code-gen|test|docs|research|qc|human",
        "priority": "critical|high|normal|low",
        "parent_issue": null,
        "depends_on": [],
        "estimated_tokens": 2000
      },
      "sections": {
        "summary": "One sentence.",
        "context": "What the agent needs to know. Reference files, decisions, parent issues.",
        "acceptance_criteria": ["Testable criterion 1", "Testable criterion 2"]
      }
    }
  ]
}
```

Rules:
- Every issue MUST have: title, labels, frontmatter (with all required fields), and sections
- Labels must use the Samverk taxonomy: agent:*, type:*, priority:*, and optionally status:queued and complexity:*
- agent_type in frontmatter must match the agent:* label
- priority in frontmatter must match the priority:* label
- type in frontmatter must match the type:* label
- Acceptance criteria should be specific and testable, not vague
- Set depends_on to issue numbers when tasks have ordering constraints
- Estimate tokens conservatively (1000-5000 for most tasks)
- Output valid JSON only -- no markdown wrapping, no commentary
````
