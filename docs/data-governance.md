# Data Governance

**Issue**: [#67](https://github.com/HerbHall/samverk/issues/67)
**Status**: Design (pre-implementation)
**Scope**: Code transit inventory, provider data policies, audit logging, selective routing, and privacy notices for Samverk's AI provider integrations.

## Code Transit Map

Samverk sends user code and project context to AI providers through a well-defined set of components. Every transit path flows through the `provider.Provider` interface (`internal/provider/provider.go`), which standardizes the `Chat` method across all backends.

### Components That Send Data to Providers

```text
USER'S PROJECT REPO
    |
    v
DISPATCHER (internal/dispatcher/)
    Reads: issue bodies, frontmatter metadata, labels
    Sends to provider: issue classification prompts (agent_type determination)
    |
    +---> AGENT: code-gen (internal/agent/ -- future)
    |     Reads: source code files from repo clone
    |     Sends to provider: code context + generation prompts
    |     Data volume: HIGH (full file contents, multi-file context)
    |
    +---> AGENT: test (internal/agent/ -- future)
    |     Reads: source code + existing tests
    |     Sends to provider: code under test + test generation prompts
    |     Data volume: HIGH (source + test files)
    |
    +---> AGENT: docs (internal/agent/ -- future)
    |     Reads: source code + existing documentation
    |     Sends to provider: code + doc generation prompts
    |     Data volume: MEDIUM (code excerpts, not full files)
    |
    +---> AGENT: qc (internal/agent/ -- future)
    |     Reads: agent output (diffs, test results, generated code)
    |     Sends to provider: code review prompts with full diffs
    |     Data volume: MEDIUM-HIGH (diffs + acceptance criteria)
    |
    +---> AGENT: research (internal/agent/ -- future)
    |     Reads: project context, issue descriptions
    |     Sends to provider: research queries (less code, more prose)
    |     Data volume: LOW (primarily natural language)
    |
    +---> FRONT-END AGENT (Claude via MCP)
          Reads: issue state, project status via MCP tools
          Sends to provider: conversational context (summaries, not raw code)
          Data volume: LOW-MEDIUM (digests, status, user questions)
```

### Data Flow Through the Provider Interface

All AI communication flows through `provider.Chat()`:

```go
type ChatRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Stream   bool      `json:"stream"`
}
```

The `Messages` field is the code transit vector. It contains system prompts (Samverk-authored, no user code), user messages (may contain source code, file contents, diffs), and assistant messages (prior model responses in multi-turn conversations).

### What Data Transits

| Data Type | Example | Transit Frequency | Sensitivity |
|-----------|---------|-------------------|-------------|
| Source code files | `main.go`, `app.tsx` | Every code-gen and test task | HIGH |
| Git diffs | Unified diff output | Every QC review | HIGH |
| Issue bodies | Frontmatter + task description | Every issue classification | MEDIUM |
| Test output | Pass/fail results, error messages | Every test task | MEDIUM |
| File paths | Directory structure, file names | Most tasks | LOW-MEDIUM |
| Project metadata | Labels, assignees, status | Dispatcher operations | LOW |
| User preferences | Profile conventions, tech stack | Agent initialization | LOW |

### What Does NOT Transit

- Git credentials (never included in prompts)
- API keys for other services (injected via env vars, not in prompt context)
- `.samverk/` configuration files (local only)
- Files matching exclusion rules (see [Selective Routing](#selective-routing-design))

## Provider Data Policy Comparison

Research conducted March 2026. Policies change -- verify current terms before deployment.

### Policy Matrix

| Policy | Claude API (Anthropic) | OpenAI API | Gemini API (Google) | Ollama (Local) |
|--------|----------------------|------------|---------------------|----------------|
| **Uses API data for training** | No | No (opt-in available) | No (paid tier) | N/A -- no external transit |
| **Default data retention** | 7 days | 30 days | 55 days | Zero -- local only |
| **Zero-retention option** | Yes (ZDR addendum) | Yes (approval required) | Yes (Vertex AI ZDR) | Inherent |
| **Abuse monitoring** | Yes, retained separately | Yes, 30-day logs | Yes, 55-day logs | None |
| **Data deletion on request** | Yes (GDPR/CCPA) | Yes (GDPR/CCPA) | Yes (GDPR/CCPA) | N/A |
| **Cross-border transfer** | US-based processing | US-based processing | Regional options | No transfer |
| **SOC 2 / ISO 27001** | Yes | Yes | Yes | N/A |
| **GDPR compliant** | Yes (DPA available) | Yes (DPA available) | Yes (DPA available) | Inherent |
| **Consumer vs API distinction** | Yes -- consumer may train, API never | Yes -- consumer may train, API never | Yes -- free tier may train, paid never | N/A |

### Provider-Specific Details

#### Claude API (Anthropic)

- **Training**: API data is never used for model training under commercial terms. Consumer accounts (free/Pro) use an opt-out model as of September 2025.
- **Retention**: API logs retained for 7 days (reduced from 30 days in September 2025). Enterprise plans offer custom retention windows.
- **Zero-data-retention**: Available via Zero-Data-Retention (ZDR) addendum. Logs processed for real-time abuse detection only, then immediately discarded.
- **Flagged content**: Prompts flagged for potential policy violations may be stored up to 2 years. Classifier scores may be retained up to 7 years.
- **Deletion**: Available under GDPR/CCPA. Contact privacy team or use account settings.

**Source**: [Anthropic Privacy Center](https://privacy.claude.com/en/articles/10023548-how-long-do-you-store-my-data), [API Training Policy](https://privacy.claude.com/en/articles/10023580-is-my-data-used-for-model-training)

#### OpenAI API

- **Training**: API data is not used for training by default since March 2023. Opt-in available for organizations that want to contribute.
- **Retention**: Abuse monitoring logs retained for up to 30 days by default. After 30 days, inputs and outputs are removed.
- **Zero-data-retention**: Available with approval. Eligible customers can exclude content from abuse monitoring logs entirely.
- **Legal holds**: A 2025 court order required retention of some API content pending litigation. This is an exception to standard policy.
- **Deletion**: Available under GDPR/CCPA. Standard 30-day automatic deletion for API data.

**Source**: [OpenAI Data Controls](https://developers.openai.com/api/docs/guides/your-data/), [Training Policy](https://help.openai.com/en/articles/5722486-how-your-data-is-used-to-improve-model-performance)

#### Gemini API (Google)

- **Training**: Paid API data is not used for training. Free-tier data may be used to improve models.
- **Retention**: API responses retained for 55 days for abuse monitoring. Vertex AI uses in-memory caching with 24-hour TTL.
- **Zero-data-retention**: Available via Vertex AI Zero Data Retention. Enterprise controls support custom retention windows.
- **Regional processing**: Vertex AI offers regional data residency options. Standard Gemini API processes in Google's default regions.
- **Deletion**: Available under GDPR. Admins can shorten or disable prompt storage for Enterprise domains.

**Source**: [Gemini API Terms](https://ai.google.dev/gemini-api/terms), [Vertex AI ZDR](https://docs.google.com/vertex-ai/generative-ai/docs/vertex-ai-zero-data-retention), [Abuse Monitoring](https://ai.google.dev/gemini-api/docs/usage-policies)

#### Ollama (Local)

- **Training**: Impossible -- no data leaves the host machine.
- **Retention**: Zero external retention. Local inference logs are under user control.
- **Privacy**: Complete data sovereignty. No network calls after initial model download.
- **Compliance**: Simplifies GDPR/CCPA compliance because there is no data transfer to third parties.
- **Trade-off**: Model quality is limited by local hardware (VRAM, compute). See [System Requirements](system-requirements.md) for hardware tiers.

### Samverk's Recommendation by Tier

| Cost Tier | Recommended Approach | Rationale |
|-----------|---------------------|-----------|
| Tier 1 (local only) | Ollama exclusively | Zero data transit. Full privacy. |
| Tier 2 (one cloud + local) | Claude API (ZDR) + Ollama | Shortest retention. ZDR available. |
| Tier 3 (multi-cloud + local) | Claude + OpenAI (ZDR) + Ollama | Both offer ZDR. Failover diversity. |
| Tier 4 (full stack) | All providers + capable local GPU | Local handles most volume; cloud for complexity only. |

## Audit Log Design

Every code transit event must be logged for user transparency and regulatory compliance. This extends the audit log schema defined in the [Security Model](security-model.md#audit-log-schema).

### What to Log

| Field | Description | Example |
|-------|-------------|---------|
| `timestamp` | RFC3339 timestamp of the API call | `2026-03-15T14:30:00Z` |
| `provider` | Which provider received the data | `claude`, `openai`, `ollama` |
| `model` | Specific model used | `claude-sonnet-4-20250514`, `gpt-4o` |
| `agent_type` | Which agent initiated the call | `code-gen`, `qc`, `test` |
| `project` | Project context | `samverk`, `dockpulse` |
| `issue_number` | Related issue (if applicable) | `67` |
| `prompt_tokens` | Input token count | `4521` |
| `completion_tokens` | Output token count | `1203` |
| `files_referenced` | List of file paths included in context | `["cmd/main.go", "internal/server/"]` |
| `exclusions_applied` | Whether selective routing filtered content | `true` / `false` |
| `transit_destination` | `cloud` or `local` | `cloud` |
| `cost_estimate` | Estimated cost in USD | `0.023` |
| `duration_ms` | Round-trip time in milliseconds | `2340` |

### Audit Log Schema Extension

```sql
CREATE TABLE code_transit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    project TEXT NOT NULL,
    issue_number INTEGER,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    files_referenced TEXT,
    exclusions_applied INTEGER NOT NULL DEFAULT 0,
    transit_destination TEXT NOT NULL CHECK(transit_destination IN ('cloud', 'local')),
    cost_estimate_usd REAL,
    duration_ms INTEGER,
    request_hash TEXT
);

CREATE INDEX idx_transit_timestamp ON code_transit_log(timestamp);
CREATE INDEX idx_transit_provider ON code_transit_log(provider);
CREATE INDEX idx_transit_project ON code_transit_log(project);
```

### Log Retention and Access

- **Retention**: Transit logs retained for 90 days by default. Configurable via `.samverk/server.yaml`.
- **Access**: Users can view transit logs via the web dashboard (Audit tab) or CLI (`samverk audit transit --project samverk --last 7d`).
- **Export**: `samverk audit export --format csv` for regulatory compliance or personal record-keeping.
- **Integrity**: Logs are append-only in SQLite. The `request_hash` field stores a SHA-256 hash of the prompt content (not the content itself) for tamper detection without storing sensitive data twice.

### What Is NOT Logged

- Full prompt content (too large, duplicates sensitive data)
- Model responses (stored in issue comments via the forge, not in transit logs)
- API keys or authentication tokens

## Selective Routing Design

Users can define rules that prevent specific files or paths from being sent to cloud providers. This is critical for projects containing secrets, proprietary algorithms, or PII in test data.

### Exclusion Rules Configuration

```yaml
# .samverk/data-governance.yaml
selective_routing:
  # Files matching these patterns are NEVER sent to cloud providers.
  # They can still be processed by local Ollama models.
  cloud_exclusions:
    - "*.secret"
    - "*.key"
    - "*.pem"
    - "*.env"
    - "*.env.*"
    - "internal/auth/*"
    - "internal/crypto/*"
    - "**/*_secret*"
    - "**/credentials*"
    - "**/testdata/pii/*"

  # Files matching these patterns are never sent to ANY provider
  # (including local). They are completely excluded from AI context.
  full_exclusions:
    - ".samverk/secrets.yaml"
    - ".samverk/auth.yaml"
    - "**/*.pfx"
    - "**/*.p12"

  # When a cloud-excluded file is needed for a task, behavior options:
  # - "local_fallback": route the entire task to Ollama (default)
  # - "redact": strip excluded files from context, proceed with cloud
  # - "block": fail the task and escalate to needs-human
  on_cloud_exclusion: "local_fallback"
```

### Routing Decision Flow

```text
Agent prepares context for provider.Chat()
    |
    v
Collect file paths referenced in context
    |
    v
Check each path against full_exclusions
    |
    +-- Match --> Remove file from context entirely
    |
    v
Check each path against cloud_exclusions
    |
    +-- Match + target is cloud provider
    |       |
    |       +-- on_cloud_exclusion: "local_fallback"
    |       |       --> Re-route entire task to Ollama
    |       |
    |       +-- on_cloud_exclusion: "redact"
    |       |       --> Strip matched files, proceed with cloud
    |       |
    |       +-- on_cloud_exclusion: "block"
    |               --> Fail task, add status:needs-human
    |
    +-- No match --> Proceed normally
    |
    v
Log transit event to code_transit_log
    |
    v
Call provider.Chat()
```

### Default Exclusion Patterns

Samverk ships with sensible defaults that protect common secret patterns. Users can extend but not weaken the defaults (the built-in patterns always apply).

Built-in cloud exclusions (always active, cannot be removed):

- `*.key`, `*.pem`, `*.pfx`, `*.p12` -- cryptographic material
- `*.env`, `*.env.*` -- environment variable files
- `.samverk/secrets.yaml`, `.samverk/auth.yaml` -- Samverk credentials

User-defined exclusions are additive. The configuration file adds patterns on top of the built-in set.

## Local-Only Mode

Users can run Samverk entirely on Ollama with zero cloud calls (Tier 1). This section documents what works and what degrades.

### Full Capability (Local)

- Code generation (boilerplate, CRUD, tests, formatting)
- Documentation generation
- Schema validation
- Issue classification and routing
- Frontmatter parsing (deterministic, no AI needed)
- Dependency graph resolution (deterministic)
- Heartbeat monitoring (deterministic)

### Degraded Capability (Local)

| Capability | Cloud Quality | Local Quality | Impact |
|------------|--------------|---------------|--------|
| Complex architecture decisions | High | Medium-Low | May need more human check-ins |
| Cross-domain reasoning | High | Low | User provides more guidance |
| QC arbitration (ambiguous cases) | High | Medium | More false positives/negatives |
| Research and feasibility analysis | High | Low | Slower, less comprehensive |
| Ambiguity resolution | High | Low | More `needs-human` escalations |
| Multi-file refactoring | High | Medium | Smaller change sets per task |

### Not Available (Local)

- Nothing is completely unavailable. Every feature works at some quality level with local models.
- The cost tier model (see [Cost Model](cost-model.md)) explicitly states: "No one gets locked out. Every tier produces a working result."

### Local-Only Configuration

```yaml
# .samverk/server.yaml
providers:
  ollama:
    url: "http://192.168.1.207:11434"
    enabled: true

  claude:
    enabled: false

  openai:
    enabled: false

  gemini:
    enabled: false

# Force all routing to local
routing:
  cloud_enabled: false
```

## Privacy Notice

This section is a draft privacy notice that Samverk should display when a user connects a repository for the first time.

### Draft Notice Text

```text
SAMVERK DATA GOVERNANCE NOTICE

Before Samverk agents begin working on your project, you should
understand how your code is handled.

WHAT DATA IS SENT TO AI PROVIDERS

Samverk agents send portions of your source code, git diffs, issue
descriptions, and test output to AI providers for processing. The
specific files sent depend on the task (code generation, testing,
review, documentation).

WHICH PROVIDERS RECEIVE YOUR DATA

Based on your current configuration:
- [Provider list populated from .samverk/server.yaml]
- Local processing via Ollama: [enabled/disabled]
- Cloud providers: [list of enabled cloud providers]

PROVIDER DATA POLICIES (as of March 2026)

- Claude API: 7-day retention, no training on API data
- OpenAI API: 30-day retention, no training on API data
- Gemini API: 55-day retention (paid), no training on paid API data
- Ollama: Zero external transit, all processing local

YOUR CONTROLS

- Exclude files from cloud processing: .samverk/data-governance.yaml
- Run entirely local (zero cloud): set cloud_enabled: false
- View all data transit events: samverk audit transit
- Export audit logs: samverk audit export

WHAT SAMVERK DOES NOT DO

- We do not store your source code beyond what git already stores
- We do not send your API keys, passwords, or .env files to providers
- We do not share data between different projects you manage
- We do not have access to your provider accounts

For full details, see: docs/data-governance.md
```

### When to Display

- First time a project is registered with Samverk (`samverk project add`)
- When a new cloud provider is enabled in configuration
- When selective routing rules are modified (confirmation prompt)
- Accessible at any time via `samverk privacy` CLI command

## Regulatory Considerations

### GDPR (EU/EEA Users)

Source code can contain personal data when it includes:

- PII in test fixtures (names, emails, phone numbers in seed data)
- User-facing strings with personal references
- Configuration files with real hostnames or IP addresses
- Comments mentioning real people
- Git commit metadata (author names, emails)

**Samverk's GDPR posture:**

- **Lawful basis**: Legitimate interest (user explicitly connects their repo and configures providers)
- **Data minimization**: Selective routing excludes sensitive paths. Agents request only files needed for the current task.
- **Right to erasure**: Users can delete all transit logs (`samverk audit purge`). Provider-side deletion follows provider policy (7-55 days depending on provider, or immediate with ZDR).
- **Data portability**: Transit logs exportable in CSV format.
- **Processor agreements**: Users are responsible for having appropriate DPAs with their chosen cloud providers. Samverk itself does not process data -- it orchestrates the user's own API keys and provider accounts.

### CCPA (California Users)

- **Right to know**: Transit logs provide full visibility into what was sent where.
- **Right to delete**: Same as GDPR erasure -- transit log purge + provider retention expiry.
- **Right to opt out of sale**: Samverk does not sell data. Cloud providers do not sell API data.
- **No discrimination**: Local-only mode provides full functionality (degraded quality, not missing features).

### Practical Recommendations

1. **Scrub PII from test data** before connecting a repo. Use faker libraries instead of real data.
2. **Add PII-containing paths to cloud exclusions** if real data must exist in the repo.
3. **Use local-only mode** for projects with regulatory sensitivity (healthcare, finance).
4. **Review transit logs monthly** to verify exclusion rules are working as expected.
5. **Negotiate ZDR addenda** with cloud providers if your project handles regulated data.

## ADR-026: Data Governance for AI Provider Interactions

### Status

Proposed

### Context

Samverk sends user source code to AI providers (cloud and local) for code generation, testing, review, and documentation tasks. Users need to understand and control what data leaves their infrastructure, where it goes, how long it is retained, and what regulatory obligations apply.

Three aspects require governance:

1. **Transit transparency**: Users must know which code files are sent to which providers.
2. **Selective control**: Users must be able to exclude sensitive files from cloud transit.
3. **Audit accountability**: Every transit event must be logged for compliance and user trust.

### Decision

Implement a three-layer data governance system:

**Layer 1 -- Selective Routing**: A pattern-based exclusion system (`.samverk/data-governance.yaml`) that prevents sensitive files from reaching cloud providers. Built-in exclusions for cryptographic material and credential files cannot be removed. User exclusions are additive. When excluded files are needed, the task falls back to local Ollama, is redacted, or escalates to `needs-human` (user-configurable).

**Layer 2 -- Transit Audit Log**: Every `provider.Chat()` call is logged to a `code_transit_log` SQLite table with provider, model, file paths, token counts, cost estimates, and timing. Full prompt content is not logged (too large, duplicates sensitive data). A SHA-256 hash of the prompt is stored for tamper detection.

**Layer 3 -- Privacy Notice**: A user-facing notice displayed at project registration and provider configuration changes. The notice itemizes what data is sent, which providers receive it, each provider's retention policy, and the user's available controls.

### Consequences

**Positive:**

- Users have full visibility into what code leaves their infrastructure
- Sensitive files are protected by default with built-in exclusion patterns
- Audit trail supports GDPR/CCPA compliance requirements
- Local-only mode provides a zero-transit option with no feature lockout
- Provider policy matrix gives users informed consent

**Negative:**

- Selective routing adds latency to every provider call (pattern matching)
- Transit logging increases SQLite write volume (one row per API call)
- Provider policies change -- the comparison matrix requires periodic updates
- Local fallback for excluded files may degrade task quality
- Users must configure exclusion rules manually for project-specific paths

**Mitigations:**

- Pattern matching uses compiled glob patterns cached at startup (negligible latency)
- SQLite write volume is modest (agents make tens, not thousands, of API calls per hour)
- Provider policy dates are documented; the privacy notice includes a "last verified" timestamp
- Local fallback quality is acceptable for most excluded-file scenarios (auth code review, secret handling)
- Default exclusion patterns cover the most common sensitive file types

### Alternatives Considered

| Alternative | Why Rejected |
|-------------|-------------|
| No transit logging | Users cannot verify what was sent. Regulatory non-compliance. |
| Log full prompt content | Duplicates sensitive data in a second location. Storage cost. |
| Cloud-only (no local option) | Locks out privacy-conscious users. Violates "no one gets locked out" principle. |
| Provider-level encryption (client-side) | Providers cannot process encrypted code. Adds complexity without privacy benefit (provider already has plaintext during inference). |
| Automatic PII detection and redaction | False positives break code context. Adds significant complexity. Better as a future enhancement than a launch requirement. |

### References

- [Security Model](security-model.md) -- authentication, authorization, secret management
- [Cost Model](cost-model.md) -- provider tiers and work distribution
- [Architecture](architecture.md) -- agent hierarchy and data flow
- [Provider Interface](../internal/provider/provider.go) -- `Chat()` method definition
- [Ollama Implementation](../internal/provider/ollama/ollama.go) -- local provider
- [Anthropic Privacy Center](https://privacy.claude.com/en/articles/10023548-how-long-do-you-store-my-data)
- [OpenAI Data Controls](https://developers.openai.com/api/docs/guides/your-data/)
- [Gemini API Terms](https://ai.google.dev/gemini-api/terms)
- [Vertex AI Zero Data Retention](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/vertex-ai-zero-data-retention)
