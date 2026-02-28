# DevKit Profile Analysis

Validation of the Samverk user profile schema (docs/user-profile.md) against
[HerbHall/devkit](https://github.com/HerbHall/devkit) as the first real-world
test case, per ADR-017.

## Executive Summary

The profile schema partially works for devkit. The four proposed sections
(Project Conventions, Technical Preferences, AI Agent Configuration, Standing
Decisions) capture roughly 40% of what devkit actually contains. The remaining
60% falls into categories the schema does not account for: executable skills
and agent templates, machine-provisioning automation, multi-tier configuration
precedence, a learning feedback loop, and an explicit development methodology.

The schema needs expansion, not replacement. The four existing sections are
valid; they are just insufficient. Six additional profile sections are
recommended below.

## DevKit Inventory

### Root Files

| File | Purpose |
|------|---------|
| `CLAUDE.md` | Repo-level build commands, structure, gotchas |
| `METHODOLOGY.md` | 6-phase development process (Concept through Release) |
| `README.md` | Public-facing overview and quick start |
| `VERSION` | Semver version of devkit itself |
| `CHANGELOG.md` | Version history |
| `LICENSE` | Project license (MIT) |
| `.sync-manifest.json` | Three-tier config manifest (universal, machine, project) |
| `.markdownlint-cli2.jsonc` | Markdownlint runner config |
| `.markdownlint.json` | Markdownlint rules |

### `claude/` -- Claude Code Configuration

**Global instructions** (`claude/CLAUDE.md`): System-prompt-level rules loaded
every session. Covers workflow, coding principles, git safety, communication
style, MCP tool inventory, and environment details. This is the single most
important file in the repo -- it shapes all AI agent behavior.

**Rules** (`claude/rules/`, 10 files): Auto-loaded pattern files organized by
tier:

- Tier 0 (immutable): `core-principles.md`, `error-policy.md`
- Tier 1 (governed): `workflow-preferences.md`, `review-policy.md`,
  `markdown-style.md`, `agent-team-coordination.md`,
  `subagent-ci-checklist.md`, `compaction-recovery.md`
- Tier 2 (learned): `autolearn-patterns.md` (91+ entries),
  `known-gotchas.md` (63+ entries)

**Skills** (`claude/skills/`, 18 skills): Invokable capabilities with YAML
frontmatter, routing tables, and workflow sub-files. Examples:

- `go-development` -- Go idioms, patterns, conventions
- `autolearn` (reflect) -- Session retrospective and knowledge extraction
- `devkit-sync` -- Multi-machine synchronization operations
- `quality-control` -- CI monitoring, PR health checks
- `manage-github-issues` -- Issue triage, phase generation, auditing
- `requirements-generator` -- Structured requirements creation
- `plan-review`, `code-review`, `security-review` -- Independent review gates

**Agents** (`claude/agents/`, 7 agents): Subagent templates with model
selection, tool restrictions, and behavioral instructions:

- `go-test-writer.md` -- Table-driven Go test generation (model: sonnet)
- `plan-reviewer.md` -- Independent plan review
- `review-code.md` -- Code review with fresh context
- `security-analyzer.md` -- Security-focused review
- `portfolio-analyzer.md` -- Cross-project portfolio analysis
- `vscode-test-writer.md` -- VS Code extension tests
- `vscode-translation-manager.md` -- i18n management

**Hooks** (`claude/hooks/SessionStart.sh`): Runs on every session start.
Auto-pulls devkit updates, checks symlink health, detects missing CLAUDE.md
in new projects.

**Settings** (`claude/settings.template.json`): Template for
`~/.claude/settings.json` with permissions, hook config, and enabled plugins.

**Other**: `CLAUDE.md.template`, `CLAUDE.local.md.template`,
`AGENT-WORKFLOW-GUIDE.md`, `AUTOMATION-SETUP.md`, `SKILLS-ECOSYSTEM.md`,
`claude-functions.sh`

### `profiles/` -- Stack Profiles

Three profiles defining tech-stack-specific tool requirements:

| Profile | Description | Dependencies |
|---------|-------------|--------------|
| `go-cli.md` | Go CLI/daemon development | None |
| `go-web.md` | Go web/gRPC services | `go-cli` |
| `iot-embedded.md` | ESP32/ESPHome IoT devices | None |

Each profile uses YAML frontmatter declaring: winget packages, manual install
commands, VS Code extensions, and Claude skills. The markdown body provides
usage guidance.

### `project-templates/` -- Scaffolding Templates

| File | Purpose |
|------|---------|
| `Makefile.go` | Go project Makefile (build, test, lint, hooks) |
| `ci.yml` | GitHub Actions CI workflow for Go |
| `golangci.yml` | golangci-lint v2 configuration |
| `github-labels.json` | Standardized issue label taxonomy |
| `claude-md-template.md` | CLAUDE.md boilerplate for new projects |
| `workspace-claude-md-template.md` | Workspace-level CLAUDE.md |
| `concept-brief.md` | Phase 0 concept template |
| `rules-local-template.md` | Template for local rule overrides |

### `devspace/` -- Workspace Configuration

| File | Purpose |
|------|---------|
| `.editorconfig` | Auto-cascading editor formatting rules |
| `.markdownlint.json` | Auto-cascading markdown lint rules |
| `shared-vscode/` | Copyable VS Code settings fragments (Go, TS, extensions) |
| `templates/` | Starter templates (ADR, design doc, test plan, CLAUDE.md) |

### `setup/` -- Machine Provisioning

| File | Purpose |
|------|---------|
| `bootstrap.ps1` | Phase 1: OS detection, core tools, git config |
| `stack.ps1` | Phase 2: Profile-based stack installation |
| `new-project.ps1` | Phase 3: Project scaffolding from templates |
| `setup.ps1` | Menu entry point dispatching to phases |
| `verify.ps1` | Post-install verification |
| `backup.ps1` | Machine state snapshot |
| `sync.ps1` | Symlink creation and DevKit sync |
| `lib/*.ps1` | Shared PowerShell libraries (checks, credentials, install, profiles, UI) |
| `legacy/*.sh` | Deprecated bash equivalents |

### `mcp/` -- MCP Server Configuration

| File | Purpose |
|------|---------|
| `servers.md` | MCP server inventory (essential, recommended, optional) |
| `memory-seeds.md` | Bootstrap entities for MCP Memory knowledge graph |
| `claude-desktop.template.json` | Claude Desktop MCP server config template |

### `machine/` -- Machine State Snapshots

| File | Purpose |
|------|---------|
| `vscode-extensions.txt` | Installed VS Code extensions list |
| `winget.json` | Installed winget packages snapshot |
| `git-config.template` | Git configuration template |
| `manual-requirements.md` | Tools requiring manual installation |

### `docs/` -- Architecture Decisions

| File | Purpose |
|------|---------|
| `ADR-0011-sync-architecture.md` | Symlink-based synchronization design |
| `ADR-0012-three-tier-architecture.md` | Universal/Machine/Project tier model |
| `ADR-0013-dual-language-scripting.md` | PowerShell + Bash strategy |
| `BOOTSTRAP.md` | Machine bootstrap documentation |
| `DECISIONS.md` | Decision index |
| `PROFILES.md` | Stack profile format specification |
| `cross-platform-guide.md` | Windows/Linux/macOS compatibility |
| `forge-abstraction.md` | Git forge abstraction (GitHub/Gitea) |
| `multi-workstation-guide.md` | Multi-machine sync guide |
| `project-registry-schema.md` | Project registry format |

### `tests/` -- Review Pipeline Tests

End-to-end validation for the plan-review and code-review agents, with known-bad
input and expected findings.

## Mapping Matrix

| DevKit Content | Profile Section | Quality | Notes |
|---|---|---|---|
| **Branch naming conventions** (`workflow-preferences.md` #1) | Project Conventions | Direct | Branch pattern `feature/issue-NNN-desc` |
| **Commit message format** (`workflow-preferences.md` #2) | Project Conventions | Direct | Conventional commits with co-author tag |
| **PR/review workflow** (`review-policy.md`) | Project Conventions | Direct | Mandatory review triggers, severity escalation |
| **Directory structure standards** (`CLAUDE.md` project structure) | Project Conventions | Interpretive | Standards are per-project, not in a single profile field |
| **Explore-Plan-Code-Commit flow** (`workflow-preferences.md` #3) | Project Conventions | Direct | Development workflow sequence |
| **Primary languages** (`profiles/*.md` frontmatter) | Technical Preferences | Direct | Go, Python, TypeScript per profile |
| **Preferred frameworks** (`profiles/*.md` body) | Technical Preferences | Direct | Cobra, React, ESPHome per profile |
| **Testing approach** (`workflow-preferences.md` #5) | Technical Preferences | Direct | Table-driven tests, testutil patterns |
| **Build/CI tooling** (`project-templates/ci.yml`, `Makefile.go`) | Technical Preferences | Direct | GitHub Actions, golangci-lint, Make |
| **Linter configuration** (`project-templates/golangci.yml`) | Technical Preferences | Direct | Specific linter set and exclusion rules |
| **VS Code extensions** (`profiles/*.md` frontmatter) | Technical Preferences | Direct | Per-stack extension recommendations |
| **Trust tier preferences** | AI Agent Configuration | **Gap** | Not in devkit; defined in Samverk's autonomy-model.md |
| **Model routing** (`agents/*.md` model field) | AI Agent Configuration | Interpretive | Agents declare model (sonnet/haiku) but no routing rules |
| **Cost thresholds** | AI Agent Configuration | **Gap** | Not in devkit |
| **Communication style** (`CLAUDE.md` Communication section) | AI Agent Configuration | Direct | Concise, no emojis, no time estimates |
| **Default license** (`LICENSE`) | Standing Decisions | Implicit | MIT license present but not declared as a preference |
| **Open source default** | Standing Decisions | Implicit | All repos are public GitHub |
| **Preferred hosting** | Standing Decisions | **Gap** | No explicit hosting preference |
| **Security requirements** (`core-principles.md` #9) | Standing Decisions | Interpretive | Principle-level guidance, not specific requirements |
| **Skills** (18 skills with workflows) | **Unmapped** | N/A | No profile section for executable capabilities |
| **Agent templates** (7 agents) | **Unmapped** | N/A | No profile section for subagent definitions |
| **SessionStart hooks** | **Unmapped** | N/A | No profile section for automation hooks |
| **Autolearn patterns** (91+ entries) | **Unmapped** | N/A | No profile section for accumulated learnings |
| **Known gotchas** (63+ entries) | **Unmapped** | N/A | No profile section for platform-specific issues |
| **Core principles** (10 immutable rules) | **Unmapped** | N/A | No profile section for fundamental values |
| **Error policy** (fix-forward workflow) | **Unmapped** | N/A | No profile section for error handling philosophy |
| **Development methodology** (6 phases) | **Unmapped** | N/A | No profile section for process definition |
| **Machine provisioning** (`setup/*.ps1`) | **Unmapped** | N/A | No profile section for environment setup |
| **Multi-machine sync** (`devkit-sync` skill) | **Unmapped** | N/A | No profile section for distribution mechanism |
| **MCP server inventory** (`mcp/servers.md`) | **Unmapped** | N/A | No profile section for tool infrastructure |
| **MCP memory seeds** (`mcp/memory-seeds.md`) | **Unmapped** | N/A | No profile section for knowledge bootstrap |
| **Stack profiles** (`profiles/*.md`) | **Unmapped** | N/A | No profile section for stack-specific toolchains |
| **Project templates** (`project-templates/*`) | **Unmapped** | N/A | No profile section for scaffolding assets |
| **Workspace config** (`devspace/*`) | **Unmapped** | N/A | No profile section for editor/workspace settings |
| **Rule governance tiers** (0/1/2 system) | **Unmapped** | N/A | No profile section for rule mutability policy |
| **Subagent CI checklists** | **Unmapped** | N/A | No profile section for CI verification templates |
| **Three-tier architecture** (Universal/Machine/Project) | **Unmapped** | N/A | No profile section for configuration precedence tiers |
| **Review pipeline tests** (`tests/`) | **Unmapped** | N/A | No profile section for validation test suites |

### Mapping Summary

- **Direct mappings**: 10 items (schema captures them cleanly)
- **Interpretive mappings**: 4 items (schema can represent them with some
  stretching)
- **Implicit mappings**: 2 items (inferable but not explicitly declared)
- **Gaps**: 3 items (schema expects them but devkit lacks them)
- **Unmapped**: 17 categories (devkit has them, schema has no section)

## Gap Analysis

### What DevKit Has That the Profile Schema Does Not Account For

**1. Executable AI Capabilities (Skills + Agents)**

DevKit's 18 skills and 7 agents are not static preferences -- they are
executable capability definitions with routing tables, workflow files, model
selection, and tool restrictions. The profile schema treats AI agent
configuration as simple settings (trust tiers, model routing, cost thresholds)
but ignores that users build complex, composable AI workflows.

This is the largest gap. Skills like `autolearn`, `quality-control`, and
`manage-github-issues` represent hundreds of lines of behavioral specification
that fundamentally shape how agents work. They are not preferences; they are
capabilities.

**2. Accumulated Knowledge Base (Autolearn Patterns + Known Gotchas)**

DevKit's `autolearn-patterns.md` (91+ entries) and `known-gotchas.md` (63+
entries) represent months of empirical learning. These are not preferences or
conventions -- they are a curated knowledge base of corrections, platform
workarounds, lint fixes, and architectural patterns. The profile schema has no
concept of learned knowledge that grows over time.

**3. Development Methodology**

`METHODOLOGY.md` defines a 6-phase process (Concept, Research, Specification,
Prototype, Implementation, Release) with gate criteria, time budgets, and
artifact templates. The profile schema captures conventions and preferences but
not process -- the "how" of development, not just the "what."

**4. Machine Provisioning and Distribution**

DevKit is not just a configuration file -- it is a deployment system. The
`setup/` directory contains PowerShell scripts that bootstrap machines, install
stacks, scaffold projects, verify installations, and create symlinks. The
`.sync-manifest.json` defines a three-tier architecture
(Universal/Machine/Project) with precedence rules and promotion paths.

The profile schema mentions sources (devkit repo, repo analysis, onboarding,
manual config) but does not address how profile content reaches agents at
runtime or how it stays synchronized across machines.

**5. Rule Governance and Mutability Tiers**

DevKit classifies rules into three tiers: immutable (Tier 0, human-PR only),
governed (Tier 1, requires devkit issue), and learned (Tier 2, autolearn can
add). The profile schema has no concept of which parts of the profile can be
changed by agents versus which require human approval.

**6. MCP Infrastructure Configuration**

The `mcp/` directory defines server inventories, Claude Desktop config
templates, and memory bootstrap seeds. These are not user preferences -- they
are infrastructure definitions that determine what tools are available to
agents. The profile schema does not account for tool infrastructure.

### What the Profile Schema Expects That DevKit Lacks

**1. Trust Tier Preferences**

The schema expects explicit trust tier settings (from Samverk's autonomy model).
DevKit has core principles about autonomy bounds (Principle #10) but no
granular trust tier configuration per task type.

**2. Cost Thresholds and Budget Limits**

The schema expects cost management settings. DevKit has no cost tracking -- it
operates in a context where Claude Code is used via subscription, not per-token
billing.

**3. Preferred Hosting and Infrastructure**

The schema expects hosting preferences. DevKit is agnostic about hosting --
projects individually choose their deployment targets.

## Schema Adjustment Recommendations

### Keep Existing Sections (Rename for Clarity)

1. **Project Conventions** -- rename to **Workflow and Conventions**. Add
   sub-fields for: development methodology reference, review policy, error
   handling policy.
2. **Technical Preferences** -- keep as-is. Add sub-fields for: stack profile
   references (linking to profile definitions), editor/workspace config.
3. **AI Agent Configuration** -- rename to **Agent Capabilities**. Expand
   beyond settings to include: skill inventory, agent template inventory, hook
   definitions, MCP server requirements.
4. **Standing Decisions** -- keep as-is. Mark cost thresholds and hosting
   preferences as optional (they are Samverk-specific, not universal).

### Add New Sections

**5. Knowledge Base**

A growing collection of learned patterns, known gotchas, and corrections. This
section is append-only during normal operation and curated through explicit
review.

```yaml
knowledge_base:
  patterns:
    source: "autolearn-patterns.md"
    entry_count: 91
    categories: [lint-fix, ci-config, architecture-pattern, testing, workflow-pattern, ...]
  gotchas:
    source: "known-gotchas.md"
    entry_count: 63
    categories: [platform-workaround, tooling, git, framework-pattern, ...]
  governance:
    tier_0_immutable: ["core-principles.md", "error-policy.md"]
    tier_1_governed: ["workflow-preferences.md", "review-policy.md", ...]
    tier_2_learned: ["autolearn-patterns.md", "known-gotchas.md"]
```

**6. Development Process**

The methodology that governs how work flows from idea to release. This is
distinct from conventions (which are formatting and naming rules) and from
agent configuration (which is how agents are set up).

```yaml
process:
  methodology: "METHODOLOGY.md"
  phases: [concept, research, specification, prototype, implementation, release]
  gate_criteria: true
  review_policy: "review-policy.md"
  error_policy: "error-policy.md"
```

**7. Environment and Distribution**

How the profile reaches machines and stays synchronized. This addresses the
deployment aspect that the current schema ignores.

```yaml
environment:
  provisioning: "setup/"
  sync_mechanism: "symlink"
  sync_manifest: ".sync-manifest.json"
  tiers: [universal, machine, project]
  precedence: "project > machine > universal"
  platforms: [windows, linux, darwin]
```

**8. Stack Profiles**

Reusable definitions of tech-stack-specific toolchains. These are referenced
by projects but defined at the profile level. They include tool versions,
install commands, VS Code extensions, and Claude skill associations.

```yaml
stack_profiles:
  - name: go-cli
    version: "1.0"
    tools: [go, golangci-lint, staticcheck, govulncheck, swag]
    extensions: [golang.go, eamodio.gitlens]
    skills: [go-development, systematic-debugging, security-review]
  - name: go-web
    version: "1.0"
    requires: [go-cli]
    tools: [buf, grpc-health-probe]
    extensions: [zxh404.vscode-proto3, humao.rest-client]
    skills: [go-development, webapp-testing, security-review]
  - name: iot-embedded
    version: "1.0"
    tools: [python, uv, esphome, esptool]
    extensions: [esphome.esphome-vscode, ms-python.python]
    skills: [systematic-debugging, bash-linux]
```

**9. Scaffolding Templates**

Ready-to-copy files that standardize new project creation. These are not
preferences -- they are concrete artifacts that encode preferences into
reusable form.

```yaml
templates:
  project:
    makefile: "project-templates/Makefile.go"
    ci: "project-templates/ci.yml"
    linter: "project-templates/golangci.yml"
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

**10. Tool Infrastructure**

MCP servers, plugins, and knowledge graph seeds that define what tools agents
have access to.

```yaml
tool_infrastructure:
  mcp_servers:
    essential: [memory, sequential-thinking, context7, MCP_DOCKER]
    recommended: [github, sqlite, docker-local]
    optional: [gitlab, hass-mcp]
  plugins:
    enabled: [taches-cc-resources, context7, code-review, superpowers, ...]
  memory_seeds: "mcp/memory-seeds.md"
```

### Revised Section Count

The revised profile schema has 10 sections instead of 4:

| # | Section | Original | Status |
|---|---------|----------|--------|
| 1 | Workflow and Conventions | Project Conventions | Expanded |
| 2 | Technical Preferences | Technical Preferences | Minor additions |
| 3 | Agent Capabilities | AI Agent Configuration | Major expansion |
| 4 | Standing Decisions | Standing Decisions | Unchanged |
| 5 | Knowledge Base | -- | New |
| 6 | Development Process | -- | New |
| 7 | Environment and Distribution | -- | New |
| 8 | Stack Profiles | -- | New |
| 9 | Scaffolding Templates | -- | New |
| 10 | Tool Infrastructure | -- | New |

## Recommended Ingestion Approach

How should Samverk programmatically read a devkit-style repo and populate a
user profile?

### Phase 1 -- Discovery

Samverk reads the repo root to identify what kind of profile source it is:

```text
1. Check for .sync-manifest.json -> DevKit-format repo (structured)
2. Check for CLAUDE.md + claude/ directory -> Claude Code config repo (semi-structured)
3. Check for package.json or go.mod -> Regular project repo (infer preferences)
4. None of the above -> Unknown format (fall back to onboarding conversation)
```

For a devkit-format repo, the `.sync-manifest.json` is the authoritative index.
It declares exactly which files are universal, which are machine-local, and
which are project-scoped. Parse this file first.

### Phase 2 -- Structured Extraction

For each section in the sync manifest:

| Manifest Key | Profile Section | Extraction Method |
|---|---|---|
| `tiers.universal.rules` | Knowledge Base, Workflow and Conventions | Parse markdown headers and YAML frontmatter for tier/category metadata |
| `tiers.universal.skills` | Agent Capabilities | Parse SKILL.md YAML frontmatter for name, description, tool list |
| `tiers.universal.agents` | Agent Capabilities | Parse agent YAML frontmatter for model, tools, role |
| `tiers.universal.hooks` | Agent Capabilities | Identify hook type and trigger from filename and shebang |
| `tiers.universal.files` | Workflow and Conventions, Standing Decisions | Parse CLAUDE.md sections for workflow, communication, git safety |
| `tiers.universal.reference` | Multiple | Route by directory (profiles -> Stack Profiles, project-templates -> Scaffolding, etc.) |

### Phase 3 -- Interpretive Extraction

Some profile fields require interpretation rather than direct parsing:

- **Default license**: Read `LICENSE` file, classify type (MIT, Apache-2.0,
  BSL-1.1, etc.)
- **Primary languages**: Count file extensions across the user's repos, or
  read stack profile names
- **Communication style**: Extract from CLAUDE.md Communication section
  (keywords: concise, no emojis, no time estimates)
- **Testing approach**: Extract from workflow-preferences.md Testing Approach
  section

### Phase 4 -- Gap Filling

After extraction, identify which profile fields are still empty. Present these
to the user during onboarding as questions:

- "DevKit doesn't specify a default hosting preference. Where do you typically
  deploy? (GitHub Pages / Cloudflare / self-hosted / other)"
- "No cost thresholds found. Would you like to set a monthly AI spending
  budget?"
- "No trust tier preferences found. How much autonomy should agents have by
  default? (conservative / standard / full)"

### Implementation Notes

- **File reading**: Use `gh api repos/{owner}/{repo}/contents/{path}` for
  remote repos or direct filesystem access for local clones
- **YAML frontmatter parsing**: Use Go's `encoding/json` on the JSON output
  from the GitHub API, or a lightweight YAML parser for local files. DevKit
  profiles use `---` delimited YAML frontmatter
- **Incremental updates**: After initial ingestion, Samverk should watch for
  devkit repo changes (new commits to main) and re-extract only changed files
- **Profile versioning**: Store a snapshot hash of the devkit commit that was
  ingested. When the devkit repo is updated, diff against the snapshot to
  identify what changed
- **Precedence**: Respect the three-tier precedence from
  `.sync-manifest.json`: project overrides machine overrides universal. When
  a project specifies a different linter config than the profile, the project
  wins

## References

- [User Profile](user-profile.md)
- [ADR-016: User Profile as First-Class Concept](decisions/ADR-016-user-profile.md)
- [ADR-017: Devkit as Reference Implementation](decisions/ADR-017-devkit-reference.md)
- [HerbHall/devkit](https://github.com/HerbHall/devkit)
