# User Profile System Design

## Overview

The user profile is a persistent representation of a developer's preferences,
conventions, knowledge, and tooling configuration. It serves as the single
source of truth that Samverk agents consult before making decisions, eliminating
the need for users to repeat themselves across projects and sessions.

The profile concept originates from the observation that
[HerbHall/devkit](https://github.com/HerbHall/devkit) already functions as a
cross-project organizational standard. Samverk formalizes this into a
structured, queryable system stored in its SQLite database.

**Relationship to existing docs:**

- [user-profile.md](user-profile.md) -- Original concept and motivation
- [devkit-profile-analysis.md](devkit-profile-analysis.md) -- Gap analysis
  showing the original 4-section schema covers ~40% of devkit content
- [ADR-016](decisions/ADR-016-user-profile.md) -- Decision to treat user
  profile as first-class
- [ADR-017](decisions/ADR-017-devkit-reference.md) -- Decision to validate
  against devkit

This design document expands the original 4-section schema by nesting the
missing categories as subsections, keeping the top level flat and backward
compatible.

## Schema

The profile schema has four top-level sections, each with nested subsections
that capture the categories identified as gaps in the devkit analysis.

### Section 1 -- Project Conventions

User's standards for how projects are structured, named, and governed.

```yaml
project_conventions:
  directory_structure:
    standard: "cmd/ internal/ pkg/ web/ docs/"
    description: "Go project layout with embedded React SPA"
  file_naming:
    go: "snake_case"
    typescript: "kebab-case"
    docs: "kebab-case"
  git:
    branch_pattern: "feature/issue-{number}-{description}"
    commit_format: "conventional"
    co_author: "Co-Authored-By: Claude <noreply@anthropic.com>"
    merge_strategy: "squash"
    direct_to_main: false
  pr_workflow:
    review_required: true
    review_triggers:
      - multi_file_change
      - auth_or_secrets
      - before_pr
    severity_escalation:
      critical: "block"
      high: "revise"
      medium: "user_decides"
      low: "note"

  # Subsection: Development Process
  # Maps to devkit METHODOLOGY.md (6-phase process)
  process:
    phases:
      - name: "concept"
        gate: "problem statement + success criteria defined"
      - name: "research"
        gate: "competitive analysis + feasibility confirmed"
      - name: "specification"
        gate: "design doc + ADR approved"
      - name: "prototype"
        gate: "proof of concept validates approach"
      - name: "implementation"
        gate: "CI green + review passed"
      - name: "release"
        gate: "tagged + changelog updated"
    error_policy: "fix-forward"
    review_policy: "independent-reviewer"
```

**Devkit sources:** `workflow-preferences.md` (#1, #2, #3), `review-policy.md`,
`METHODOLOGY.md`, `core-principles.md`, `error-policy.md`

### Section 2 -- Technical Preferences

Languages, frameworks, testing, and build tooling the user prefers.

```yaml
technical_preferences:
  languages:
    primary: "go"
    secondary: ["typescript", "python"]
    versions:
      go: "1.24"
      node: "20"
      python: "3.12"
  frameworks:
    go: ["cobra", "chi"]
    typescript: ["react", "vite", "tailwind"]
    python: ["esphome"]
  testing:
    go:
      style: "table-driven"
      utilities: ["testutil.NewStore", "testutil.NewDevice"]
      race_detection: true
      coverage_threshold: null
    typescript:
      framework: "vitest"
      utilities: ["testing-library"]
  build:
    go: "make + goreleaser"
    typescript: "vite"
    ci: "github-actions"
  linting:
    go: "golangci-lint v2"
    typescript: "eslint + typescript-eslint"
    markdown: "markdownlint-cli2"
  editor:
    primary: "vscode"
    config_cascade: "editorconfig"

  # Subsection: Stack Profiles
  # Maps to devkit profiles/*.md
  stack_profiles:
    - name: "go-cli"
      tools: ["go", "golangci-lint", "govulncheck", "swag"]
      extensions: ["golang.go", "eamodio.gitlens"]
      skills: ["go-development", "systematic-debugging"]
    - name: "go-web"
      requires: ["go-cli"]
      tools: ["buf", "grpc-health-probe"]
      extensions: ["zxh404.vscode-proto3", "humao.rest-client"]
      skills: ["go-development", "webapp-testing"]
    - name: "iot-embedded"
      tools: ["python", "uv", "esphome", "esptool"]
      extensions: ["esphome.esphome-vscode", "ms-python.python"]
      skills: ["systematic-debugging"]

  # Subsection: Scaffolding Templates
  # Maps to devkit project-templates/*
  scaffolding:
    project:
      makefile: "project-templates/Makefile.go"
      ci_workflow: "project-templates/ci.yml"
      linter_config: "project-templates/golangci.yml"
      labels: "project-templates/github-labels.json"
      claude_md: "project-templates/claude-md-template.md"
    workspace:
      editorconfig: "devspace/.editorconfig"
      markdownlint: "devspace/.markdownlint.json"
      vscode_fragments: "devspace/shared-vscode/"
    documentation:
      adr: "devspace/templates/adr-template.md"
      design_doc: "devspace/templates/design-template.md"
      test_plan: "devspace/templates/test-plan-template.md"
```

**Devkit sources:** `profiles/*.md`, `project-templates/*`, `devspace/*`,
`workflow-preferences.md` (#5)

### Section 3 -- AI Agent Configuration

How agents behave, what tools they have, and what knowledge they carry.

```yaml
ai_agent_configuration:
  trust_tiers:
    default_stance: "standard"
    cost_threshold_usd: 5.00
    global_overrides:
      merge_staging: "tier1"
    agent_overrides:
      qc:
        close_issue: "tier1"
      code-gen:
        push_branch: "tier3"
  model_routing:
    implementation: "sonnet"
    research: "haiku"
    review: "sonnet"
    test_generation: "sonnet"
  communication:
    style: "concise"
    emojis: false
    time_estimates: false
    technical_accuracy_over_validation: true

  # Subsection: Knowledge Base
  # Maps to devkit autolearn-patterns.md + known-gotchas.md + rules/
  knowledge_base:
    patterns:
      count: 91
      categories:
        - "lint-fix"
        - "ci-config"
        - "architecture-pattern"
        - "testing"
        - "workflow-pattern"
        - "platform-workaround"
        - "correction"
        - "process-pattern"
        - "research-methodology"
        - "tooling-workaround"
        - "frontend-pattern"
    gotchas:
      count: 63
      categories:
        - "platform-workaround"
        - "tooling"
        - "git"
        - "framework-pattern"
        - "ci-fix"
    governance:
      tier_0_immutable:
        - "core-principles"
        - "error-policy"
      tier_1_governed:
        - "workflow-preferences"
        - "review-policy"
        - "markdown-style"
        - "agent-team-coordination"
        - "subagent-ci-checklist"
        - "compaction-recovery"
      tier_2_learned:
        - "autolearn-patterns"
        - "known-gotchas"

  # Subsection: Tool Infrastructure
  # Maps to devkit mcp/servers.md + plugins + hooks
  tool_infrastructure:
    mcp_servers:
      essential:
        - "memory"
        - "sequential-thinking"
        - "context7"
        - "MCP_DOCKER"
      recommended:
        - "github"
        - "sqlite"
      optional:
        - "gitlab"
    hooks:
      session_start: "auto-pull devkit, check symlinks, detect missing CLAUDE.md"
      user_prompt_submit: "surface coordination context"
    memory_seeds: "mcp/memory-seeds.md"
```

**Devkit sources:** `claude/agents/*.md`, `claude/rules/*.md`,
`claude/skills/*`, `claude/hooks/*`, `mcp/*`

### Section 4 -- Standing Decisions

Persistent choices that apply across all projects unless overridden.

```yaml
standing_decisions:
  license: "MIT"
  visibility: "public"
  hosting: null
  security:
    xss_prevention: true
    sql_injection_prevention: true
    command_injection_prevention: true
    validate_at_boundaries: true
  compliance: null

  # Subsection: Environment and Distribution
  # Maps to devkit setup/*.ps1 + .sync-manifest.json
  environment:
    platforms: ["windows"]
    provisioning_method: "powershell-scripts"
    sync_mechanism: "symlink"
    config_tiers:
      - name: "universal"
        description: "Applies to all machines and projects"
      - name: "machine"
        description: "Machine-specific overrides"
      - name: "project"
        description: "Project-specific overrides"
    precedence: "project > machine > universal"
```

**Devkit sources:** `LICENSE`, `setup/*.ps1`, `.sync-manifest.json`,
`core-principles.md` (#9)

### Schema Summary

| Section | Subsections | Devkit Coverage |
|---------|-------------|-----------------|
| Project Conventions | Development Process | Branch, commit, PR, methodology, error/review policy |
| Technical Preferences | Stack Profiles, Scaffolding Templates | Languages, frameworks, testing, build, linting, profiles, templates |
| AI Agent Configuration | Knowledge Base, Tool Infrastructure | Trust tiers, model routing, communication, patterns, gotchas, MCP, hooks |
| Standing Decisions | Environment and Distribution | License, visibility, security, provisioning, sync, config tiers |

This structure captures all 17 previously unmapped categories from the devkit
analysis while keeping the original four top-level sections intact.

## SQLite Storage Design

The profile is stored as a JSON document in Samverk's SQLite database. A
single-document approach is simpler than normalized tables because:

- The profile is read as a whole by agents (no partial queries needed)
- Updates target specific paths within the JSON (SQLite's `json_extract` and
  `json_set` handle this)
- Schema evolution is straightforward (add new keys, old keys remain)
- Export/import is a single JSON blob

### Table Schema

```sql
CREATE TABLE IF NOT EXISTS user_profiles (
    id          TEXT PRIMARY KEY DEFAULT 'default',
    profile     TEXT NOT NULL,  -- JSON document (full profile)
    source_type TEXT NOT NULL,  -- 'devkit', 'repo_analysis', 'onboarding', 'manual'
    source_ref  TEXT,           -- repo URL, path, or null
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS profile_audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id  TEXT NOT NULL REFERENCES user_profiles(id),
    action      TEXT NOT NULL,  -- 'create', 'update', 'import', 'rollback'
    path        TEXT NOT NULL,  -- JSON path that changed (e.g., 'technical_preferences.languages.primary')
    old_value   TEXT,           -- JSON of previous value (null for create)
    new_value   TEXT,           -- JSON of new value
    reason      TEXT NOT NULL,  -- Why the change was made
    source      TEXT NOT NULL,  -- 'agent:code-gen', 'agent:dispatcher', 'user', 'ingestion'
    tier        INTEGER NOT NULL, -- Autonomy tier (1, 2, or 3) that governed this change
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_audit_profile ON profile_audit_log(profile_id);
CREATE INDEX idx_audit_created ON profile_audit_log(created_at);
CREATE INDEX idx_audit_path ON profile_audit_log(path);
```

### Design Notes

- **Single row per user:** Samverk v1 is single-user. The `id` column defaults
  to `'default'`. Multi-user support (if ever needed) adds rows, not tables.
- **Version column:** Incremented on every update. Enables optimistic locking
  (agent reads version N, writes only if version is still N).
- **Audit log:** Every change is recorded with the JSON path, old/new values,
  reason, source agent, and the autonomy tier that governed the change. This
  provides full traceability without external tooling.
- **JSON path convention:** Dot-separated paths matching the YAML schema keys
  (e.g., `project_conventions.git.branch_pattern`,
  `ai_agent_configuration.knowledge_base.patterns.count`).

## Ingestion Flow

How Samverk imports an existing devkit-style repo into the profile store.

### Phase 1 -- Discovery

Samverk reads the source to determine its format.

```text
Input: local path or remote repo URL

1. Check for .sync-manifest.json
   -> DevKit-format repo (structured, authoritative index)
2. Check for CLAUDE.md + claude/ directory
   -> Claude Code config repo (semi-structured)
3. Check for go.mod, package.json, or Cargo.toml
   -> Regular project repo (infer preferences from project structure)
4. None of the above
   -> Unknown format (fall back to onboarding conversation)
```

For a devkit-format repo, `.sync-manifest.json` is the authoritative index
declaring which files are universal, machine-local, and project-scoped.

### Phase 2 -- Extraction

Structured extraction maps devkit files to profile fields.

| Source File/Directory | Target Profile Path | Method |
|---|---|---|
| `claude/rules/workflow-preferences.md` | `project_conventions.git`, `project_conventions.pr_workflow` | Parse markdown sections |
| `claude/rules/review-policy.md` | `project_conventions.pr_workflow.review_triggers` | Parse tables |
| `METHODOLOGY.md` | `project_conventions.process` | Parse phase headings and gate criteria |
| `profiles/*.md` | `technical_preferences.stack_profiles[]` | Parse YAML frontmatter |
| `project-templates/*` | `technical_preferences.scaffolding` | List files, record paths |
| `devspace/*` | `technical_preferences.scaffolding.workspace` | List files, record paths |
| `claude/agents/*.md` | `ai_agent_configuration.model_routing` | Parse YAML frontmatter for model field |
| `claude/rules/autolearn-patterns.md` | `ai_agent_configuration.knowledge_base.patterns` | Count entries, extract categories |
| `claude/rules/known-gotchas.md` | `ai_agent_configuration.knowledge_base.gotchas` | Count entries, extract categories |
| `claude/CLAUDE.md` Communication section | `ai_agent_configuration.communication` | Parse keywords |
| `mcp/servers.md` | `ai_agent_configuration.tool_infrastructure.mcp_servers` | Parse categorized lists |
| `LICENSE` | `standing_decisions.license` | Classify license type |
| `setup/*.ps1` | `standing_decisions.environment` | Detect provisioning approach |
| `.sync-manifest.json` | `standing_decisions.environment.config_tiers` | Parse tier definitions |

### Phase 3 -- Transformation

Convert extracted data into the profile JSON schema.

```text
For each extraction result:
  1. Validate against expected types (string, list, object)
  2. Normalize values (e.g., "MIT License" -> "MIT", "golang" -> "go")
  3. Merge with any existing profile data (new values do not overwrite
     user-modified fields unless forced)
  4. Record the source file and extraction method in metadata
```

### Phase 4 -- Storage

Write the assembled profile to SQLite.

```text
1. Serialize profile object to JSON
2. INSERT into user_profiles (or UPDATE if re-importing)
3. Write audit log entry: action='import', source='ingestion',
   tier=1, reason='Initial import from {source_ref}'
4. Record the source commit hash for incremental re-import
```

### Phase 5 -- Gap Filling

After import, identify empty fields and prompt the user during onboarding.

Example prompts:

- "No hosting preference found in devkit. Where do you typically deploy?"
- "No cost threshold found. Would you like to set a monthly AI spending budget?"
- "Devkit has no explicit trust tier preferences. Use standard defaults?"

Gap-filling responses are stored as manual updates with `source='user'` and
`tier=1`.

## Update Lifecycle

How agents propose and apply profile changes at runtime.

### Tier 2 Auto-Apply Flow

Low-risk profile updates that proceed immediately and are surfaced for review.

```text
Agent discovers something
  -> e.g., "Windows MSYS bash auto-translates paths"

Agent classifies update risk (see Risk Classification below)
  -> Result: Tier 2 (new gotcha pattern, additive, no behavior change)

Agent writes to profile
  -> json_set(profile, '$.ai_agent_configuration.knowledge_base.gotchas.count', 64)
  -> Appends gotcha content to knowledge base storage

Agent writes audit log
  -> action='update', path='ai_agent_configuration.knowledge_base.gotchas',
     reason='Discovered MSYS path translation gotcha during issue #42',
     source='agent:code-gen', tier=2

Check-in digest includes
  -> "Profile updated: added gotcha #64 (MSYS path translation). Review?"

User at check-in
  -> Acknowledges (no action) or reverts (triggers rollback from audit log)
```

### Tier 3 Approval Flow

High-risk profile updates that require explicit confirmation.

```text
Agent proposes a change
  -> e.g., "Switch default test framework from vitest to jest"

Agent classifies update risk
  -> Result: Tier 3 (changes technical preference, affects all future projects)

Agent creates needs-human issue
  -> Title: "Profile update: change default test framework to jest"
  -> Body: what, why, impact, rollback path
  -> Labels: needs-human, profile-update

Agent continues other work (not blocked)

User at check-in
  -> Front-end agent surfaces Tier 3 items first
  -> User approves or rejects with reason

On approval
  -> Profile updated, audit log written with tier=3
  -> needs-human issue closed

On rejection
  -> Audit log records rejection with reason
  -> Agent receives rejection reason for future context
```

### Risk Classification Rules

Each profile update is classified by the agent before application.

**Tier 2 (auto-apply, logged for review):**

- Adding a new pattern or gotcha to the knowledge base
- Updating entry counts or category lists
- Adding a new MCP server to the optional list
- Recording a new stack profile tool version
- Adding a new scaffolding template path
- Updating the source commit hash after re-import

**Tier 3 (requires confirmation):**

- Changing a primary language or framework
- Modifying the default license or visibility
- Changing trust tier defaults or cost thresholds
- Modifying the development process phases or gate criteria
- Changing the review policy or error policy
- Removing any existing profile entry (deletions are never Tier 2)
- Changing the config tier precedence rules
- Modifying security requirements

**General principle:** Additive, low-impact changes are Tier 2. Changes that
alter existing behavior, affect security, or modify standing decisions are
Tier 3.

## Conflict Resolution

When a project's local configuration contradicts the user profile, the
resolution follows three-tier precedence:

```text
1. Explicit project-level override     (highest priority)
2. User profile defaults
3. Samverk system defaults              (lowest priority)
```

This matches `.editorconfig` cascading semantics and the devkit three-tier
architecture (project > machine > universal).

### Resolution Examples

| Profile Says | Project Says | Result |
|---|---|---|
| `license: MIT` | `license: Apache-2.0` | Apache-2.0 (project wins) |
| `go_version: 1.24` | (not specified) | 1.24 (profile fills gap) |
| `commit_format: conventional` | `commit_format: conventional` | conventional (agreement, no conflict) |
| `test_style: table-driven` | (not specified) | table-driven (profile fills gap) |
| (not specified) | (not specified) | Samverk system default |

### How Agents Resolve Conflicts

When an agent needs a preference value, it queries in order:

1. **Project config** (`.samverk/config.yaml` in the project root)
2. **User profile** (SQLite `user_profiles.profile` JSON)
3. **System defaults** (hardcoded in Samverk binary)

The first non-null value wins. Agents never merge conflicting values -- they
pick one. If a project explicitly sets a value, the profile is ignored for that
field even if the profile value is "better."

### Profile-Aware Project Init

When creating a new project, Samverk uses the profile to pre-populate project
config. The user can override during the init flow. This means new projects
start with the user's preferences by default, reducing setup friction.

## Versioning

Profile changes are tracked through two mechanisms.

### Version Counter

The `user_profiles.version` column is an integer that increments on every
update. This serves two purposes:

- **Optimistic locking:** An agent reads version N, then writes only if
  version is still N. If another agent updated in the meantime, the write
  fails and the agent must re-read.
- **Snapshot reference:** Projects can record which profile version they were
  created with, enabling "what conventions applied when this project started?"
  queries.

### Audit Log

The `profile_audit_log` table provides a complete history of every change.

```sql
-- What changed in the last 7 days?
SELECT path, old_value, new_value, reason, source, created_at
FROM profile_audit_log
WHERE created_at > datetime('now', '-7 days')
ORDER BY created_at DESC;

-- Who changed the license?
SELECT * FROM profile_audit_log
WHERE path = 'standing_decisions.license'
ORDER BY created_at DESC;

-- What did agent:code-gen change?
SELECT path, new_value, reason, created_at
FROM profile_audit_log
WHERE source = 'agent:code-gen'
ORDER BY created_at DESC;

-- Roll back to a specific version
-- (read old_value from the audit log entry after the target version)
```

### Rollback

To roll back a change, the system reads the `old_value` from the audit log
entry and writes it back to the profile. A new audit log entry is created with
`action='rollback'` pointing to the original entry. This preserves the full
history -- rollbacks are forward operations, not history rewrites.

## Query Interface

How agents access the profile at runtime.

### Go Interface

```go
// ProfileStore provides read and write access to the user profile.
type ProfileStore interface {
    // Get returns the full profile as a structured object.
    // Returns ErrProfileNotFound if no profile exists.
    Get(ctx context.Context) (*Profile, error)

    // GetField returns a single field value by JSON path.
    // Path uses dot notation: "technical_preferences.languages.primary"
    // Returns nil if the path does not exist.
    GetField(ctx context.Context, path string) (any, error)

    // Update applies a change to a specific field.
    // Handles version checking, audit logging, and tier validation.
    Update(ctx context.Context, req UpdateRequest) error

    // Import replaces the entire profile from an external source.
    // Used during initial ingestion and full re-imports.
    Import(ctx context.Context, profile *Profile, source SourceInfo) error

    // AuditLog returns recent profile changes.
    AuditLog(ctx context.Context, opts AuditLogOpts) ([]AuditEntry, error)

    // Version returns the current profile version number.
    Version(ctx context.Context) (int, error)
}

// UpdateRequest describes a single profile field change.
type UpdateRequest struct {
    Path     string       // JSON path (e.g., "standing_decisions.license")
    Value    any          // New value
    Reason   string       // Why the change is being made
    Source   string       // Who is making the change (e.g., "agent:code-gen", "user")
    Tier     autonomy.Tier // Autonomy tier governing this change
    Version  int          // Expected current version (optimistic lock)
}

// SourceInfo describes the origin of an imported profile.
type SourceInfo struct {
    Type      string // "devkit", "repo_analysis", "onboarding", "manual"
    Reference string // repo URL, local path, or empty
    CommitSHA string // source repo commit hash (for incremental re-import)
}

// AuditLogOpts controls audit log queries.
type AuditLogOpts struct {
    Path   string    // Filter by JSON path prefix (empty = all)
    Source string    // Filter by source (empty = all)
    Since  time.Time // Only entries after this time (zero = all)
    Limit  int       // Max entries to return (0 = default 50)
}

// AuditEntry represents a single profile change record.
type AuditEntry struct {
    ID        int
    Action    string // "create", "update", "import", "rollback"
    Path      string
    OldValue  any
    NewValue  any
    Reason    string
    Source    string
    Tier      autonomy.Tier
    CreatedAt time.Time
}
```

### Usage Pattern

```go
// Agent checking a convention before making a decision
license, err := profileStore.GetField(ctx, "standing_decisions.license")
if err != nil {
    // No profile or field not set -- use system default
    license = "MIT"
}

// Agent recording a discovered pattern
err = profileStore.Update(ctx, profile.UpdateRequest{
    Path:    "ai_agent_configuration.knowledge_base.patterns.count",
    Value:   92,
    Reason:  "Discovered new lint-fix pattern during issue #42",
    Source:  "agent:code-gen",
    Tier:    autonomy.Tier2,
    Version: currentVersion,
})
```

## Implementation Phases

### Phase 1 -- MVP (Manual Import, Read-Only)

**Goal:** Agents can read profile data. Profile is populated via CLI import.

**Deliverables:**

- `Profile` struct matching the schema above
- `ProfileStore` interface with `Get`, `GetField`, `Version` methods
- SQLite table creation (`user_profiles`, `profile_audit_log`)
- `samverk profile import <path>` CLI command that runs the ingestion flow
  against a local devkit-format repo
- `samverk profile show [path]` CLI command to display the profile or a
  specific field
- Read-only integration: agents query the profile but do not write to it

**Not included:** Auto-updates, tier classification, knowledge base content
storage (only metadata/counts).

### Phase 2 -- Auto-Updates (Tier 2 and Tier 3)

**Goal:** Agents can propose and apply profile changes governed by the
autonomy model.

**Deliverables:**

- `Update` and `Import` methods on `ProfileStore`
- Risk classification logic mapping profile paths to autonomy tiers
- Tier 2 auto-apply flow with audit logging
- Tier 3 approval flow creating `needs-human` issues
- `AuditLog` method and `samverk profile log` CLI command
- Rollback support via `samverk profile rollback <entry-id>`
- Check-in digest integration showing Tier 2 profile changes for review

### Phase 3 -- Cross-Project Learning

**Goal:** Patterns learned in one project automatically benefit others.

**Deliverables:**

- Knowledge base content storage (full pattern and gotcha entries, not just
  counts)
- Pattern deduplication (same gotcha discovered independently in two projects)
- Profile-aware project init (new projects pre-populated from profile)
- Incremental re-import (detect devkit changes via commit hash comparison)
- Profile export (`samverk profile export > profile.json`) for backup and
  portability

## References

- [User Profile Concept](user-profile.md)
- [DevKit Profile Analysis](devkit-profile-analysis.md)
- [ADR-016: User Profile as First-Class Concept](decisions/ADR-016-user-profile.md)
- [ADR-017: Devkit as Reference Implementation](decisions/ADR-017-devkit-reference.md)
- [Autonomy Model](autonomy-model.md)
- [ADR-015: Three-Tier Autonomy Model](decisions/ADR-015-three-tier-autonomy.md)
