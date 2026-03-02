# Agent Quality -- Hallucination Rates and QC Effectiveness

Research document for Samverk issue #71. Evaluates whether cross-model
validation (one agent writes, a different agent reviews) produces
reliable quality assurance for autonomous code generation.

## Literature Review -- Code Generation Benchmarks

### HumanEval (Function Synthesis)

HumanEval measures Pass@1: the percentage of problems a model solves
correctly on the first attempt, using 164 Python function-synthesis
tasks with test-case validation. As of early 2026:

| Model | HumanEval Pass@1 | Source |
|-------|-------------------|--------|
| Qwen3 Max | 92.7% | Spectrum AI Lab |
| Claude Sonnet 4 | ~92% (2 failures / 164) | arxiv 2511.04355 |
| Qwen 2.5 Coder 32B | 91.0% | Failing Fast |
| GPT-4o | 90.2% | Multiple sources |
| DeepSeek V3 | 82.6% | DeepSeek technical report |
| Llama 3.3 70B | Lowest of frontier tier | arxiv 2511.04355 |

HumanEval is now largely saturated. Of 164 tasks, 113 are solved
correctly by every frontier model. It differentiates only at the
margins. MBPP (Mostly Basic Python Problems) is even more saturated --
frontier models solve nearly all of it.

### SWE-bench Verified (Real-World Issue Resolution)

SWE-bench tests against real GitHub issues from popular Python
repositories, requiring models to locate, understand, and patch bugs
across full codebases. This is the most relevant benchmark for
Samverk's use case (agents working autonomously on real projects).

| Model | SWE-bench Verified | Notes |
|-------|--------------------|-------|
| Claude Opus 4.5 | 80.9% | Highest as of Feb 2026 |
| Claude Opus 4.6 | 80.8% | Current frontier |
| MiniMax M2.5 | 80.2% | Top open-weight model |
| GPT-5.2 | 80.0% | OpenAI flagship |
| Claude Sonnet 4.5 | 77.2% | Best cost/quality ratio |
| Gemini 3 Pro | 76.2% | Google flagship |
| DeepSeek V3.2 | 73.0% | Open-source |
| Qwen3-Coder | 69.6% | Open-source, agentic focus |
| DeepSeek V3.1 | 66.0% | Open-source |

SWE-bench Pro (harder subset) shows more differentiation: Claude Opus
4.5 leads at 45.89%, Claude Sonnet 4.5 at 43.60%, Gemini 3 Pro at
43.30%. Older models like GPT-4o score only 4.9%, revealing a massive
generational gap.

### LiveCodeBench (Competitive Coding, Contamination-Free)

LiveCodeBench continuously collects new problems to prevent data
contamination. Over 1,000 problems collected since May 2023.

| Model | LiveCodeBench Score |
|-------|---------------------|
| Gemini 3 Pro Preview (high) | 91.7% |
| Gemini 3 Flash Preview (Reasoning) | 90.8% |
| GLM-4.7 | 84.9% |
| Mistral Large | 82.8% |

### Key Benchmark Takeaway for Samverk

HumanEval and MBPP measure isolated function generation -- necessary
but insufficient for Samverk's use case. SWE-bench Verified is the
benchmark that most closely mirrors what Samverk agents do: work on
real codebases, find and fix real bugs, navigate real project
structure. Cloud frontier models solve ~80% of these tasks. Open-source
models solve ~66-73%. The 7-14 percentage point gap is the quality
delta Samverk must account for in its model routing decisions.

## Error Taxonomy -- Categories of Code Generation Failures

Research has identified five primary categories of code hallucination
(CodeMirage, 2024) and three additional categories from practical code
generation studies (FSE 2025, ACM 2025).

### Category 1 -- Syntactic Errors

**Severity:** Low. **Detectability:** Trivially caught by compiler/interpreter.

Code that violates language grammar rules. Frontier models rarely
produce these (HumanEval saturation demonstrates this). Local 7B models
produce them more frequently on complex tasks with nested structures.

**QC detection rate:** ~100%. Any model can catch syntax errors because
the compiler catches them first.

### Category 2 -- API and Library Hallucination

**Severity:** High. **Detectability:** Moderate (requires dependency awareness).

The most studied and dangerous category. LLMs fabricate:

- Non-existent packages (20% of recommendations across models)
- Non-existent functions or methods within real packages
- Incorrect function signatures or parameter orders
- Deprecated APIs treated as current

Key findings from package hallucination research:

- GPT-series models: 5.2% hallucination rate for packages
- Open-source models: 21.7% hallucination rate (4x higher)
- Prompts mentioning "2025" produce hallucinated libraries in up to 84% of tasks
- Single-character misspellings in library names: up to 26% hallucination rate
- Multi-character misspellings: up to 79% hallucination rate

This category has direct security implications. Malicious actors can
register the hallucinated package names ("slopsquatting"), turning an
AI code generation error into a supply chain attack vector.

**QC detection rate:** ~70-85%. A reviewing model can catch non-existent
imports if it has accurate training data about the library ecosystem.
Cross-provider review helps because different training data means
different blind spots. However, both models may agree that a
hallucinated API exists if their training data contains similar
misinformation.

### Category 3 -- Logical Errors

**Severity:** High. **Detectability:** Low to moderate.

Code that executes without errors but produces incorrect results.
Includes:

- Off-by-one errors in loops and boundaries
- Incorrect conditional logic (wrong boolean operators, inverted conditions)
- Edge case failures (empty input, null values, overflow)
- Algorithm correctness failures (wrong sorting order, incorrect aggregation)

This is the hardest category for QC to catch because the code
"looks right" on inspection. Static analysis and test generation are
more effective than code review alone.

**QC detection rate:** ~40-60%. A reviewing model catches obvious logic
errors but frequently misses subtle ones. The CodeX-Verify study found
75% detection for correctness issues, but this was with specialized
correctness-focused agents, not general review.

### Category 4 -- Dead and Unreachable Code

**Severity:** Low. **Detectability:** High.

Code segments that are never executed: unreachable branches after
unconditional returns, unused variables, vestigial code blocks. More
of a maintainability issue than a functional bug.

**QC detection rate:** ~80-90%. Static analysis tools catch this
reliably; LLM reviewers also perform well since the pattern is
visually distinct.

### Category 5 -- Security Vulnerabilities

**Severity:** Critical. **Detectability:** Moderate.

Includes OWASP Top 10 patterns, CWE-classified vulnerabilities,
hardcoded credentials, SQL injection, command injection, path
traversal, improper input validation. LLMs frequently generate
insecure patterns because they optimize for functionality, not
security.

**QC detection rate:** ~87.5% with specialized security agents
(CodeX-Verify). General-purpose reviewers catch ~50-65%. SecureFalcon
achieved 94% binary detection accuracy. Cross-model review is
especially valuable here because different models have different
security awareness patterns.

### Category 6 -- Project Context Conflicts

**Severity:** High. **Detectability:** Low.

Generated code that is individually correct but conflicts with the
broader project context:

- Violating existing architectural patterns
- Duplicating functionality that already exists
- Using different naming conventions than the project
- Ignoring project-specific configuration or constants
- Breaking interface contracts with other modules

This category is the hardest for any reviewer (human or AI) without
full project context. It is also the most relevant to Samverk's
autonomous operation, where agents work on projects over extended
periods.

**QC detection rate:** ~30-50%. Requires the reviewer to have deep
project context, which is expensive in tokens and attention.

### Category 7 -- Robustness and Resource Issues

**Severity:** Medium. **Detectability:** Moderate.

Memory leaks, resource handle leaks (unclosed files, connections),
unbounded allocation, missing timeout handling, missing retry logic
for transient failures.

**QC detection rate:** ~60% with specialized performance agents.

### Category 8 -- Task Requirement Conflicts

**Severity:** High. **Detectability:** Moderate.

Code that works correctly but doesn't solve the stated problem.
The model misinterprets the task requirements or addresses a different
problem than the one specified. Common when task descriptions are
ambiguous or multi-part.

**QC detection rate:** ~55-70%. Requires the reviewer to understand
the original task intent, not just the code mechanics.

### Summary Table

| Category | Severity | QC Catch Rate | Best Detection Method |
|----------|----------|---------------|----------------------|
| Syntax errors | Low | ~100% | Compiler/interpreter |
| API hallucination | High | ~70-85% | Dependency resolver + cross-model |
| Logical errors | High | ~40-60% | Test generation + specialized agent |
| Dead code | Low | ~80-90% | Static analysis |
| Security vulns | Critical | ~65-87% | Specialized security agent |
| Context conflicts | High | ~30-50% | Full-project-aware reviewer |
| Resource issues | Medium | ~60% | Specialized performance agent |
| Requirement conflicts | High | ~55-70% | Task-aware reviewer |

## QC Effectiveness Analysis

### Single-Agent vs Multi-Agent Review

The CodeX-Verify study (arxiv 2511.16708, December 2025) provides the
strongest evidence for Samverk's cross-model validation approach.

**Single-agent baseline:** 32.8% bug detection rate.

**Multi-agent results (cumulative):**

| Agents | Detection Rate | Marginal Gain |
|--------|---------------|---------------|
| 1 | 32.8% | -- |
| 2 | 47.7% | +14.9pp |
| 3 | 61.2% | +13.5pp |
| 4 | 72.4% | +11.2pp |

The mathematical basis is submodularity of mutual information under
conditional independence: combining agents with different detection
patterns finds more bugs than any single agent. Agent correlation
measured at rho = 0.05 to 0.25, confirming they detect different bugs.

**This directly validates Samverk's cross-model QC assumption.** Using
a different model (or differently prompted model) as a reviewer catches
different errors than the generating model would catch on self-review.

### Cross-Provider vs Same-Provider Review

No study directly compares cross-provider (Claude reviews GPT output)
vs same-provider (Claude reviews Claude output) for code review. However,
the evidence strongly suggests cross-provider is more effective:

1. **Training data diversity:** Different providers train on different
   data distributions. Their hallucination patterns differ. A package
   hallucinated by GPT (5.2% rate) may not be hallucinated by an
   open-source model, and vice versa (21.7% rate but different specific
   packages).

2. **Agent correlation:** The CodeX-Verify study shows that agents with
   lower correlation (rho = 0.05-0.25) find more combined bugs. Different
   providers inherently have lower correlation than same-provider models
   of different sizes.

3. **LLM Output Drift study (2025):** Cross-provider validation catches
   behavioral differences that same-provider testing misses. While
   focused on financial workflows, the principle applies: diversity
   in the review pipeline catches more systematic errors.

**Recommendation for Samverk:** Cross-provider review (e.g., Claude
generates, Ollama/DeepSeek reviews, or vice versa) should be the
default QC mode when available. Same-provider different-model review
(e.g., Claude Opus generates, Claude Haiku reviews) is the fallback.

### LLM-as-Judge Accuracy for Code

Research on LLM-as-judge for code evaluation reveals important
limitations:

- GPT-4o achieves 68.5% accuracy classifying code as correct/incorrect
  (with problem descriptions)
- Code-to-requirement conformance rates range from 52% to 78%
- Models frequently "hallucinate" bugs that do not exist (false positives)
- Models fine-tuned specifically to be judges performed poorly on code,
  "often nearing random guessing"
- Chain-of-thought reasoning models (o1, QwQ) "drastically outperform"
  standard instruction-tuned models as judges

**Implication for Samverk:** The QC agent should be a reasoning-capable
model, not a fast/cheap model. Using Haiku-class models for QC
introduces unacceptable false positive rates. Sonnet-class or better
is the minimum for QC review. This contradicts the intuition to use
cheap models for review -- the reviewer needs to be at least as capable
as the generator.

### Detection Rates by Error Category (CodeX-Verify)

| Category | Detection Rate | Sample |
|----------|---------------|--------|
| Resource management | 100% (7/7) | File handles, connections |
| Security | 87.5% (7/8) | OWASP patterns |
| Correctness | 75% (18/24) | Logic bugs |
| Performance | 60% (6/10) | Algorithmic complexity |
| **Overall** | **76.1%** | **99 samples** |

Existing non-multi-agent tools catch 65% of bugs with 35% false
positives. The multi-agent approach matches or exceeds this while
reducing false positives through consensus.

## Retry-to-Success Analysis

### Does the Original Agent Fix Rejected Code

Research on iterative debugging with LLMs reveals a critical pattern:

- GPT-4o corrects rejected code 67.8% of the time
- Gemini 2.0 Flash corrects rejected code 54.3% of the time
- Without "hard deterministic feedback" (test results, compiler errors),
  the system can diverge -- the agent rephrases the same bug

**The generate-test-fix loop is essential.** When the QC agent rejects
code with only a natural language explanation, the generating agent
frequently produces a cosmetically different version of the same bug.
When rejection includes failing test cases or compiler errors, the
fix rate jumps significantly because the feedback is unambiguous.

**Recommendation for Samverk:** QC rejection must include:

1. The specific error category (from the taxonomy above)
2. A failing test case or concrete reproduction step where possible
3. The expected vs actual behavior
4. A suggested fix direction (not a complete fix -- that biases the generator)

Without this structure, retry cycles waste tokens and time without
converging on correct code.

### Retry Budget

Based on the 54-68% single-retry fix rate:

- After 1 retry: ~60% cumulative fix rate
- After 2 retries: ~84% cumulative fix rate (0.6 + 0.4 * 0.6)
- After 3 retries: ~94% cumulative fix rate

Three retries is a reasonable budget before escalating to a different
model or to the user. Diminishing returns and divergence risk make
more than 3 retries counterproductive.

## Task Complexity Threshold Analysis

### Where Local Models Become Unreliable

Research and benchmarks identify clear complexity thresholds:

**Local 7B models (Qwen 2.5 Coder 7B, DeepSeek Coder 6.7B):**

- Reliable for: autocomplete, simple functions, bash scripts, CLI
  utilities, boilerplate generation, test scaffolding, documentation
- Unreliable for: multi-file changes, architectural reasoning, edge
  case handling, security-sensitive code
- SWE-bench performance: not competitive (below 20%)

**Local 32B models (Qwen 2.5 Coder 32B, DeepSeek Coder 33B):**

- Reliable for: all 7B tasks plus moderate-complexity single-file
  tasks, API endpoint implementation, data transformation, unit test
  writing
- Competitive with: GPT-4o on HumanEval (91.0% vs 90.2%)
- Unreliable for: multi-concern tasks ("refactor this authentication
  flow while maintaining backward compatibility with the legacy API
  and handling the edge case where tokens expire mid-request"),
  cross-module reasoning, security review
- SWE-bench performance: competitive but below frontier (~55-65%)

**Cloud frontier models (Claude Opus/Sonnet, GPT-5.x, Gemini 3):**

- Reliable for: all above plus multi-file changes, architectural
  decisions, debugging subtle issues, security review
- SWE-bench performance: 73-81%
- Required for: QC arbitration, ambiguity resolution, cross-domain
  reasoning

### Complexity Heuristics for Task Routing

| Complexity Signal | Route To |
|-------------------|----------|
| Single function, clear spec | Local 7B-32B |
| Single file, standard patterns | Local 32B |
| Multi-file, existing patterns | Local 32B or Cloud Sonnet |
| Multi-file, new patterns | Cloud Sonnet |
| Architectural decisions | Cloud Opus |
| Security-sensitive changes | Cloud Opus + specialized review |
| Ambiguous requirements | Cloud Opus (or escalate to user) |
| QC review of any code | Cloud Sonnet minimum |

### Package Hallucination Risk by Provider Tier

| Provider Tier | Package Hallucination Rate |
|---------------|--------------------------|
| GPT-series (cloud) | 5.2% |
| Open-source models (local) | 21.7% |

This 4x difference is significant for Samverk. Local models generating
code must have their imports validated -- either by a dependency
resolver (deterministic, cheap) or by a cloud QC agent (expensive but
catches more). A hybrid approach (dependency resolver for imports,
LLM reviewer for logic) is optimal.

## QC Confidence Calibration

### Can QC Express Reliable Confidence

Research from ICSE 2025 ("Calibration and Correctness of Language
Models for Code") demonstrates that **LLM confidences are poor
predictors of code correctness.** Key findings:

- LLMs exhibit systematic overconfidence: expressed confidence is
  higher than actual accuracy
- Next-token training objectives reward confident guessing over
  calibrated uncertainty, so models "learn to bluff"
- Chain-of-thought prompting sometimes worsens calibration (model
  becomes more confident, not more accurate)
- Distractor-augmented evaluation can reduce miscalibration

**Overconfidence in LLM-as-a-Judge (2025):** When LLMs evaluate code
generated by other LLMs, they exhibit the same overconfidence pattern.
High expressed confidence does not reliably correlate with correct
assessment.

### Implications for Samverk

1. **Do not trust raw confidence scores.** A QC agent saying "I am 95%
   confident this code is correct" should not bypass further review.

2. **Calibrate via test execution.** The only reliable confidence signal
   is deterministic: does the code compile, do tests pass, does
   lint pass. These are binary and unambiguous.

3. **Use confidence as a routing signal, not a gate.** Low expressed
   confidence should trigger additional review (different model,
   user escalation). High expressed confidence should not reduce
   review rigor.

4. **Multi-agent consensus is more reliable than single-agent
   confidence.** If three agents with low inter-correlation all agree
   code is correct, that is more trustworthy than one agent expressing
   high confidence.

## Model Routing Recommendations

Based on the research above, here are Samverk's recommended model
combinations by task type.

### Tier 1 -- Local Only (Free After Hardware)

| Task | Generator | QC Reviewer | Expected Quality |
|------|-----------|-------------|------------------|
| Boilerplate / scaffolding | Qwen 32B | Compile + lint | High (>90%) |
| Documentation | Qwen 7B | Qwen 32B | High |
| Test scaffolding | Qwen 32B | Test execution | High |
| Simple bug fixes | Qwen 32B | Compile + test | Moderate (70-80%) |
| Code formatting | Qwen 7B | Lint only | Very high (>95%) |

**Risk:** 21.7% package hallucination rate. Must validate imports with
a dependency resolver.

### Tier 2 -- One Cloud Provider + Local

| Task | Generator | QC Reviewer | Expected Quality |
|------|-----------|-------------|------------------|
| Feature implementation | Claude Sonnet | Local 32B + tests | High |
| Complex bug fixes | Claude Sonnet | Compile + test | High |
| API endpoint implementation | Local 32B | Claude Sonnet | High |
| QC arbitration | -- | Claude Sonnet | Moderate-High |
| Architecture decisions | Claude Opus | Claude Sonnet | High |

**Strategy:** Cloud generates complex work, local generates volume work.
Cloud reviews local output; local + tests validate cloud output.

### Tier 3 -- Multiple Cloud Providers + Local

| Task | Generator | QC Reviewer | Expected Quality |
|------|-----------|-------------|------------------|
| Security-sensitive code | Claude Opus | GPT-5 + security scan | Very high |
| Cross-module refactoring | Claude Sonnet | DeepSeek + Claude Haiku | High |
| Critical bug fixes | GPT-5 | Claude Sonnet | Very high |
| QC arbitration (tie-break) | -- | Third provider | High |

**Strategy:** Cross-provider review for maximum diversity. Different
training data catches different hallucinations. Reserve third provider
for tie-breaking when primary generator and reviewer disagree.

### Tier 4 -- Full Stack

| Task | Generator | QC Reviewer | Expected Quality |
|------|-----------|-------------|------------------|
| Volume work | Local 32B (GPU-accelerated) | Compile + test + lint | High |
| Mid-complexity | Local 32B | Cloud Sonnet (spot check) | High |
| High-complexity | Cloud Opus/GPT-5 | Cross-provider cloud | Very high |
| QC of QC | -- | Different cloud provider | High |

**Strategy:** Local GPU absorbs 60-80% of task volume. Cloud reserved
for complexity and cross-validation. Maximum throughput at minimum
cost per task.

## Recommendations for Samverk Implementation

### Mandatory QC Layers (All Tiers)

1. **Deterministic validation first.** Compile, test, lint before any
   LLM-based review. This catches Category 1 (syntax) and much of
   Category 4 (dead code) at zero token cost.

2. **Dependency validation.** Resolve all imports against actual package
   registries. This catches Category 2 (API hallucination) deterministically.
   Do not rely on LLM review for import correctness.

3. **LLM review second.** Use a different model than the generator.
   Minimum Sonnet-class capability for the reviewer. Include structured
   feedback (error category, reproduction step, expected behavior) in
   rejection messages.

4. **Test execution third.** Run generated tests against generated code.
   This catches Category 3 (logic errors) that LLM review misses.

### Retry Policy

- Maximum 3 retries per task after QC rejection
- Each retry must include the specific failure reason and any failing
  test cases
- After 3 retries, escalate: try a different generator model
- After model switch fails, escalate to user via Tier 3 block

### Quality Metrics to Track

- Pass@1 rate per model per task complexity bucket
- QC rejection rate per model (generator quality signal)
- Retry-to-success rate per model (self-correction ability)
- False positive rate of QC (over-rejection wastes tokens)
- Time from task start to QC-approved merge
- Package hallucination rate per model (tracked via dependency validation)

### Confidence Policy

- Never auto-approve based on LLM confidence scores alone
- Require deterministic validation (compile + test + lint) for all code
- Use multi-agent consensus (2+ reviewers agree) for security-sensitive
  and irreversible actions
- Surface QC confidence in check-in digest for user awareness, not as
  a gate

## Draft ADR-030 -- Cross-Model Quality Assurance

See [ADR-030: Cross-Model Quality Assurance](decisions/ADR-030-cross-model-qa.md).

## Sources

### Benchmarks and Leaderboards

- [SWE-bench Verified Leaderboard (Feb 2026)](https://www.marc0.dev/en/leaderboard)
- [LiveCodeBench Official Leaderboard](https://livecodebench.github.io/leaderboard.html)
- [LiveCodeBench on Artificial Analysis](https://artificialanalysis.ai/evaluations/livecodebench)
- [LLM Benchmarks 2026 -- Complete Evaluation Suite](https://llm-stats.com/benchmarks)
- [SWE-bench Pro Leaderboard (Scale AI)](https://scale.com/leaderboard/swe_bench_pro_public)

### Code Hallucination Research

- [CodeMirage: Hallucinations in Code Generated by LLMs](https://arxiv.org/abs/2408.08333)
- [Library Hallucinations in LLMs (2025)](https://arxiv.org/pdf/2509.22202)
- [Importing Phantoms: Measuring LLM Package Hallucination Vulnerabilities](https://arxiv.org/html/2501.19012v1)
- [LLM Hallucinations in Practical Code Generation (ACM FSE 2025)](https://dl.acm.org/doi/abs/10.1145/3728894)
- [API Hallucination Mitigation with Hierarchical Dependencies (FSE 2025)](https://conf.researchr.org/details/fse-2025/fse-2025-industry-papers/41/)

### Multi-Agent and QC Effectiveness

- [Multi-Agent Code Verification via Information Theory (CodeX-Verify)](https://arxiv.org/abs/2511.16708)
- [Evaluating Multi-Agent AI Systems for Automated Bug Detection](https://www.ijraset.com/best-journal/evaluating-multiagent-ai-systems-for-automated-bug-detection-and-code-refactoring)
- [LLM-Based Multi-Agent Systems for Software Engineering (ACM TOSEM)](https://dl.acm.org/doi/10.1145/3712003)

### LLM-as-Judge and Confidence Calibration

- [Evaluating Large Language Models for Code Review (2025)](https://arxiv.org/html/2505.20206v1)
- [AXIOM: Benchmarking LLM-as-a-Judge for Code](https://arxiv.org/html/2512.20159v1)
- [Calibration and Correctness of Language Models for Code (ICSE 2025)](https://www.software-lab.org/publications/icse2025_calibration.pdf)
- [Overconfidence in LLM-as-a-Judge (2025)](https://arxiv.org/html/2508.06225v2)

### Model Comparisons and Local Models

- [Local AI Models for Coding: Is It Realistic in 2026?](https://failingfast.io/local-coding-ai-models/)
- [LLM Coding Benchmark Showdown 2026](https://tolearn.blog/blog/llm-coding-benchmark-comparison-2026)
- [Best AI Model for Coding 2026 (Morph)](https://www.morphllm.com/best-ai-model-for-coding)
- [Where Do LLMs Still Struggle? Analysis of Code Generation Benchmarks](https://arxiv.org/html/2511.04355v1)
- [DeepSeek V3 for Code Review (Propel)](https://www.propelcode.ai/blog/deepseek-v3-code-review-capabilities-complete-analysis)

### Code Review Workflows

- [Rethinking Code Review Workflows with LLM Assistance (2025)](https://arxiv.org/html/2505.16339v1)
- [Benchmarking and Studying LLM-based Code Review (2025)](https://arxiv.org/html/2509.01494v1)
- [State of AI Coding 2025 (Greptile)](https://www.greptile.com/state-of-ai-coding-2025)
