# Local Model Survey: RTX 3090 Ti (24 GB VRAM)

Survey of local AI models for Samverk agent roles, constrained to 24 GB VRAM.

## VRAM Estimation

Baseline formula for GGUF models: **M = P x (Q/8) x 1.2** where P = parameters (billions), Q = bits per weight, 1.2 = ~20% overhead for KV cache and runtime buffers. Q4_K_M averages ~4.5 bits per weight (~0.7-0.8 GB per billion parameters). Framework overhead adds 0.5-2 GB.

## Models That Fit in 24 GB

### Tier A: Best-in-Class (Recommended)

#### Qwen 2.5 Coder Family

| Variant | Q4_K_M | Q5_K_M | Q8_0 | FP16 | Ollama |
|---------|--------|--------|------|------|--------|
| 7B | ~5 GB | ~6 GB | ~8 GB | ~14 GB | `qwen2.5-coder:7b` |
| 14B | ~9 GB | ~11 GB | ~15 GB | ~28 GB | `qwen2.5-coder:14b` |
| 32B | **~20 GB** | ~23 GB | ~34 GB | ~64 GB | `qwen2.5-coder:32b` |

**Performance (RTX 3090):**

- 7B Q4: ~80-100 tok/s
- 32B Q4: ~20-32 tok/s

**Quality:** Best local coding model family as of 2025. 7B scores **88.4% HumanEval** (beats CodeStral 22B at 81.1%). 32B scores **92.7% HumanEval**, matching GPT-4o. Supports 92+ languages, 128K context, fill-in-the-middle for autocomplete.

**24 GB verdict:** 32B Q4 fits with ~4 GB headroom but monopolizes GPU. 7B and 14B allow concurrent loading.

#### DeepSeek R1 Distilled (Reasoning)

| Variant | Base | Q4_K_M | Ollama |
|---------|------|--------|--------|
| 1.5B | Qwen2.5-1.5B | ~1.2 GB | `deepseek-r1:1.5b` |
| 7B | Qwen2.5-7B | ~5 GB | `deepseek-r1:7b` |
| 14B | Qwen2.5-14B | ~9 GB | `deepseek-r1:14b` |
| 32B | Qwen2.5-32B | **~15 GB** | `deepseek-r1:32b` |
| 70B | Llama-3.3-70B | ~40 GB | Does not fit |

**Quality:** R1-Distill-Qwen-32B achieves 72.6% AIME 2024, 94.3% MATH-500, CodeForces rating 1691. Outperforms OpenAI o1-mini on reasoning tasks. Excels at algorithm design, debugging, and architecture decisions. Slower inference due to chain-of-thought traces.

**Key distinction from Qwen 2.5 Coder:** R1 models excel at reasoning-heavy tasks (debugging complex logic, architecture decisions). Qwen 2.5 Coder excels at code generation (function writing, boilerplate, refactoring). Different tools for different agent roles.

### Tier B: Strong Alternatives

#### DeepSeek Coder V2 Lite (MoE)

| Property | Value |
|----------|-------|
| Architecture | MoE (16B total, 2.4B active per token) |
| Q4_K_M | ~9 GB |
| HumanEval | 90.2% |
| Context | 128K |
| Ollama | `deepseek-coder-v2:lite` |

Efficient MoE architecture means low VRAM for high quality. Strong alternative to Qwen 2.5 Coder 14B.

#### Phi-4 (Microsoft)

| Variant | Q4_K_M | FP16 | Ollama |
|---------|--------|------|--------|
| 14B | ~9 GB | ~28 GB | `phi4` |

Strong math/logic performance. Good for validation and review agent roles. Not a code specialist.

#### Llama 3.x (General Purpose)

| Variant | Q4_K_M | tok/s (RTX 3090) | Ollama |
|---------|--------|-------------------|--------|
| 3.2 1B | ~1.2 GB | ~100+ | `llama3.2:1b` |
| 3.2 3B | ~2.3 GB | ~100+ | `llama3.2:3b` |
| 3.1 8B | ~5 GB | ~80-112 | `llama3.1:8b` |

Good general-purpose models. 1B and 3B are excellent for fast routing and classification. 8B is strong for general agent tasks.

### Tier C: Not Recommended

| Model | Q4_K_M | Why Not |
|-------|--------|---------|
| CodeLlama 34B | ~20 GB | HumanEval ~70% -- beaten by Qwen 2.5 Coder 7B at ~5 GB |
| DeepSeek Coder V1 33B | ~16 GB | Outdated, replaced by V2 Lite |
| CodeGemma 7B | ~5 GB | HumanEval 60.4% -- significantly behind Qwen 2.5 Coder 7B (88.4%) |
| Mixtral 8x7B | ~26-28 GB | Does not fit without CPU offloading; not code-specialized |
| Llama 3.3 70B | ~40-43 GB | Does not fit in 24 GB at any practical quantization |
| Mistral Large 2 123B | ~73 GB | Needs 4x 24 GB GPUs |

## Concurrent Model Loading

### Ollama Behavior

- `OLLAMA_MAX_LOADED_MODELS` controls max concurrent models (default: 3 per GPU)
- When VRAM is full, idle models are evicted to load new ones
- New models must completely fit in remaining VRAM -- no partial loading for concurrent models
- KV cache scales with context length: 7B at 4K context uses ~5 GB, at 32K context ~8-10 GB. Use `OLLAMA_KV_CACHE_TYPE=q8_0` or `q4_0` to reduce cache memory.

### Practical Concurrent Configurations

| Scenario | Models | VRAM Total | Free |
|----------|--------|-----------|------|
| Solo 32B | Qwen 2.5 Coder 32B Q4 | ~20 GB | ~4 GB (no room for concurrent) |
| Dual specialist | Qwen 2.5 Coder 14B + R1-Distill 14B | ~18 GB | ~6 GB |
| Triple agent | Qwen Coder 7B + R1-Distill 7B + Llama 3B | ~12.3 GB | ~12 GB |
| Quad lightweight | Coder 7B + R1 7B + Phi-4 14B + Llama 3B | ~21.3 GB | ~3 GB |

**Key constraint:** The 32B models monopolize the GPU. For multi-agent architectures requiring concurrent models, use 7B-14B class models.

## Agent Role Mapping

### Recommended Assignments

| Role | Task Type | Recommended Model | VRAM | Rationale |
|------|-----------|-------------------|------|-----------|
| **Code-gen** | Write code from specs | Qwen 2.5 Coder 32B Q4 (solo) or 7B Q4 (concurrent) | 20 / 5 GB | Best HumanEval scores; 7B is 88.4% at 1/4 the VRAM |
| **Test** | Write and run tests | Qwen 2.5 Coder 7B Q4 | 5 GB | Fast (80+ tok/s), strong code quality, allows concurrent loading |
| **Docs** | Generate documentation | Llama 3.1 8B Q4 | 5 GB | General-purpose writing, fast, low VRAM |
| **QC** | Validate agent output | DeepSeek R1-Distill 14B Q4 | 9 GB | Strong reasoning for code review and validation |
| **Research** | Search and summarize | Llama 3.1 8B Q4 or Phi-4 14B Q4 | 5 / 9 GB | General-purpose comprehension and synthesis |
| **Dispatcher** | Route and classify | Llama 3.2 3B Q4 | 2.3 GB | Ultra-fast (100+ tok/s), minimal VRAM, classification-focused |

### Deployment Strategies

#### Strategy A: Maximum Quality (Sequential)

Load one model at a time. Best code quality but no concurrency.

```text
Code-gen task → Load Qwen 2.5 Coder 32B Q4 (20 GB) → Generate code
QC task       → Evict coder, load R1-Distill 32B Q4 (15 GB) → Review
Docs task     → Evict R1, load Llama 8B (5 GB) → Generate docs
```

Model loading takes 5-15 seconds. Acceptable for async work.

#### Strategy B: Balanced (2-3 Concurrent)

Keep specialist models loaded. Best throughput for multi-task workflows.

```text
Always loaded:
  - Qwen 2.5 Coder 7B Q4 (5 GB)    -- code-gen + tests
  - DeepSeek R1-Distill 14B Q4 (9 GB) -- QC + reasoning
  - Llama 3.2 3B Q4 (2.3 GB)         -- dispatch + classify
Total: ~16.3 GB, leaving ~8 GB headroom
```

This is the recommended default for Samverk. Three concurrent agents with distinct specializations.

#### Strategy C: Speed-Optimized (3-4 Concurrent, Smaller Models)

Maximize concurrent models for parallel agent work.

```text
Always loaded:
  - Qwen 2.5 Coder 7B Q4 (5 GB)     -- code-gen
  - Qwen 2.5 Coder 7B Q4 (test copy) -- tests (separate instance)
  - Llama 3.1 8B Q4 (5 GB)           -- docs + research
  - Llama 3.2 3B Q4 (2.3 GB)         -- dispatch
Total: ~17.3 GB
```

## Evaluation Plan

Before committing to a model assignment, benchmark each candidate on Samverk-specific tasks:

1. **Code-gen benchmark**: Generate a Go HTTP handler from a spec (measure correctness, compilation success, test passage)
2. **Test benchmark**: Write table-driven Go tests for an existing function (measure coverage, edge case detection)
3. **QC benchmark**: Review a PR diff and identify bugs/issues (measure precision and recall)
4. **Dispatch benchmark**: Classify 50 sample issues by agent type and priority (measure accuracy)

These benchmarks use project-specific prompts, not generic benchmarks. Real-world performance varies from HumanEval scores.

## Related Decisions

- [ADR-007: Hybrid Local/Cloud Agents](decisions/ADR-007-hybrid-local-cloud.md)
- [ADR-019: Self-Hosted-First Development](decisions/ADR-019-self-hosted-first.md)
