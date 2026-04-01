# Root Cause Analysis and Continuous Improvement Methodology

## Purpose

Every pipeline failure is an opportunity to improve the system. This document
defines the repeatable process for analyzing failures, correcting root causes,
and preventing recurrence. The goal is not to slow down failures -- it is to
eliminate them.

## Core Principle

**If nothing changed between retries, the retry will produce the same result.**
Before retrying any issue, the system must identify what was wrong and change
something concrete: the prompt, the context, the model, or the issue itself.

## RCA Process (Per Failure)

### Step 1: Classify the Failure (Granular)

Every failure gets a multi-level classification:

```text
Level 1 (Category):    agent | infrastructure | routing | validation | provider | specification | classification
Level 2 (Subcategory): prompt_blind | wrong_model | timeout | oom | lint | test | format | rate_limit
Level 3 (Specific):    missing_file_context | wrong_import_path | hallucinated_api | stale_model
```

Level 1 is for dashboards. Level 3 is for process improvement. All three are
recorded for every failure.

### Step 2: Trace the Full Lifecycle

For the failing issue, read the **complete comment history** and answer:

1. What did the agent actually produce? (Not what it said it produced.)
2. What was the agent's prompt? Did it include the files it needed?
3. What model was assigned? Was it appropriate for the task complexity?
4. What validation caught the failure? Was the validation correct?
5. Did the retry change anything? If not, why was it retried?
6. Were there human comments that the agent never saw?

### Step 3: Identify the Deepest Root Cause

Use the "5 Whys" method. The first answer is almost never the root cause.

Example:
- Why did it fail? Agent used wrong import path.
- Why wrong import? Agent didn't see go.mod.
- Why didn't it see go.mod? go.mod not in file_context.
- Why not in file_context? Issue author didn't list it.
- Why didn't the system add it? No mechanism to auto-include foundational files.

Root cause: **The system assumes issue authors will list every required file,
but foundational files (go.mod, tsconfig.json, package.json) should be
auto-included based on agent type.**

### Step 4: Gap Analysis

Compare documented intent against actual implementation:

| Question | Source |
|----------|--------|
| What SHOULD happen? | Design docs (architecture.md, communication-protocol.md, ADRs) |
| What DOES happen? | Source code (runner.go, router.go, validator.go, prompts.go) |
| What was ASSUMED? | Implicit expectations not written anywhere |
| What was LEARNED? | Actual behavior observed from failure data |

Document each gap with file paths, line numbers, and the specific divergence.

### Step 5: Propose Changes

For each gap, propose changes to BOTH:

1. **Specification** -- Update design docs if the intent was wrong or incomplete
2. **Implementation** -- Update code if it diverges from (corrected) spec

Changes must be scoped to a single issue with clear acceptance criteria.

### Step 6: Human Review Gate

Post the RCA report as a `needs-human` issue containing:

- Failure data (issue numbers, failure counts, comment excerpts)
- Full lifecycle trace
- Root cause (deepest level)
- Gap analysis table
- Proposed spec changes
- Proposed code changes
- Dependencies on other fixes

**No implementation begins until the human approves the report.**

### Step 7: Implement and Verify

After approval:

1. Update specs first (docs/)
2. Implement code changes
3. Run `make ci` locally
4. Feed a known-failing issue through the pipeline
5. Verify E2E that the fix resolves the original failure
6. Document the result

### Step 8: Close the Loop

After verification:

1. Close the RCA issue with results
2. Update failure classification if a new category was discovered
3. Check if the fix applies to other open issues
4. Run `/autolearn` to capture the pattern

## Assumptions Register

Every design decision rests on assumptions. When an assumption proves false,
the dependent decisions must be re-evaluated.

| ID | Assumption | Status | Evidence | Impact |
|----|-----------|--------|----------|--------|
| A1 | Agents will read files listed in file_context | **FALSE** | Agents receive file_context paths but not always contents (Gap 1.1-1.3) | Agents hallucinate code structure |
| A2 | Issue authors list all required files in file_context | **FALSE** | go.mod, tsconfig.json rarely listed; agents use wrong import paths | Wrong dependencies, wrong patterns |
| A3 | Retry with prior error context is sufficient for self-correction | **PARTIAL** | Prior errors are injected (3 max, 300 char each) but without source file contents, agent cannot apply the correction | Retry loops without progress |
| A4 | Complexity labels drive model selection | **PARTIAL** | Complexity is 3rd priority after agent-type and critical label (Gap 3.3) | Wrong model for task complexity |
| A5 | Ollama models can handle code-gen tasks | **FALSE for complex tasks** | Haiku/Ollama on complexity:ambiguous frontend tasks produces hallucinated APIs, wrong patterns | Majority of post_process failures |
| A6 | Validation catches quality issues before merge | **PARTIAL** | Only frontend file_context validated; Go file_context not checked (Gap 4.1) | Bad code reaches PR stage |
| A7 | Dispatcher restart state is clean | **FALSE** | In-memory failure counts lost on restart; orphan recovery re-queues create false failure counts (Gap in metrics) | Inflated failure metrics, infinite loops |
| A8 | Human comments on issues reach the agent | **FALSE** | Issue body is sent but comments added after creation are never fetched -- no ListComments() call in prompt builder | Agent misses clarifications and decisions |

## Failure Classification Taxonomy

### Level 1: Category

| Category | Description |
|----------|-------------|
| agent | Agent produced wrong output given its inputs |
| infrastructure | System failure (OOM, crash, network, disk) |
| routing | Wrong model/provider selected for task |
| validation | Validator rejected output (correctly or incorrectly) |
| provider | AI provider error (timeout, rate limit, model not found) |
| specification | Issue was not implementable as written |
| classification | Issue frontmatter could not be parsed |

### Level 2: Subcategory (examples)

| Subcategory | Parent | Description |
|-------------|--------|-------------|
| prompt_blind | agent | Agent didn't have source files needed |
| wrong_model | routing | Model too small for task complexity |
| hallucinated_api | agent | Agent invented APIs/patterns that don't exist |
| wrong_imports | agent | Used old/wrong module paths |
| lint_fail | validation | golangci-lint failed on agent output |
| test_fail | validation | go test failed on agent output |
| format_error | validation | EDIT blocks malformed |
| rate_limit | provider | Provider or API rate limit hit (transient, not an outage) |
| timeout_provider | provider | Provider hung or was too slow |
| timeout_agent | provider | Agent ran out of time budget |
| model_not_found | provider | Ollama model not pulled |
| oom_kill | infrastructure | Process killed by OOM |
| dispatcher_crash | infrastructure | Dispatcher restarted mid-task |
| orphan_recovery | infrastructure | Issue re-queued after dispatcher restart |
| oversized_scope | specification | Issue requires too many changes for one session |
| missing_decomposition | specification | Issue needs splitting before agent work |
| stale_frontmatter | classification | Frontmatter references moved/renamed files |
| github_reference | agent | Agent used GitHub URLs/paths for Gitea project |

### Level 3: Specific

Free-text field capturing the exact error or pattern. Examples:
- `import "github.com/herbhall/samverk" should be "gitea.herbhall.net/samverk/samverk"`
- `used axios instead of fetch (project uses fetch)`
- `qwen3-coder:30b not pulled on HDH-NZXT`
- `validator only checks web/src/ prefix, missed internal/ file_context`

## Metrics

Track these metrics weekly:

| Metric | Target | Current (2026-04-01) |
|--------|--------|---------------------|
| Total failures | <50/week | 729/week |
| Failures per issue (max) | <=3 | 29 |
| Agent success rate (first attempt) | >50% | ~15% (estimated) |
| Retry success rate | >30% | ~5% (estimated) |
| Time to RCA | <24h per failure class | N/A (new process) |
| False failure rate (infra noise) | <5% | ~20% (orphan recoveries) |

## References

- [Architecture](architecture.md)
- [Communication Protocol](communication-protocol.md)
- [Autonomy Model](autonomy-model.md)
- [Error Policy](../../.devkit-stable/claude/rules/error-policy.md)
- [Core Principles](../../.devkit-stable/claude/rules/core-principles.md)
