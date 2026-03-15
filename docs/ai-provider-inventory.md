# AI Provider Inventory

Research document for issue [#442](https://github.com/HerbHall/samverk/issues/442).
Inventories all available AI services, their pricing, capabilities, and integration
path into Samverk's provider registry.

Last updated: 2026-03-14

## Provider Summary

| Provider | Status | Tool Use | Input $/1M | Output $/1M | Auth | Integration Effort |
|----------|--------|----------|------------|-------------|------|--------------------|
| Anthropic (Claude) | Active, primary | Yes | $3-5 | $15-25 | API key | Done |
| OpenAI | Account exists, $0 credits | Yes | $0.15-5 | $0.60-25 | API key | Done |
| Ollama (self-hosted) | Active, RTX 3090 Ti | Model-dependent | Free | Free | None | Done |
| Google Gemini | Free tier available | Yes | Free-$2 | Free-$12 | API key | Medium (new provider) |
| DeepSeek | Sign-up needed | Yes (chat mode) | $0.028-0.28 | $0.42 | API key | Low (OpenAI-compatible) |
| Groq | Sign-up needed | Yes | $0.05-1 | $0.08-3 | API key | Low (OpenAI-compatible) |
| Mistral | Sign-up needed | Yes | $0.02-0.50 | $0.06-2 | API key | Low (OpenAI-compatible) |
| Together.ai | Sign-up needed | Yes (select models) | $0.10-1.20 | $0.10-1.20 | API key | Low (OpenAI-compatible) |
| Fireworks | Sign-up needed | Yes | $0.10-0.26 | $0.10-0.26 | API key | Low (OpenAI-compatible) |
| GitHub Copilot | Active subscription | Via SDK | Included in sub | Included in sub | OAuth/PAT | High (SDK, preview) |
| M365 Copilot | Active subscription | Via Graph API | Credits-based | Credits-based | OAuth2/MSAL | Very High (enterprise) |

## Tier 1: Already Integrated

### Anthropic (Claude)

- **Account URL:** [console.anthropic.com](https://console.anthropic.com/settings/billing)
- **Status:** Active, primary provider for Samverk agent tasks
- **Auth:** API key via `ANTHROPIC_API_KEY` env var
- **Integration:** `internal/provider/claude/` (done) + `internal/provider/claudecli/` (headless CLI)

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| claude-haiku-4-5 | $0.25 | $1.25 | 200K | Yes | Fast, cheap -- good for triage |
| claude-sonnet-4-6 | $3.00 | $15.00 | 200K | Yes | Best value for code tasks |
| claude-opus-4-6 | $5.00 | $25.00 | 1M | Yes | Flagship, highest quality |

**Cost optimization:** Batch API at 50% off; prompt caching at 90% off input for cache hits.

### OpenAI

- **Account URL:** [platform.openai.com](https://platform.openai.com/settings/organization/billing/overview)
- **Status:** Account exists, zero credits (top-up required)
- **Auth:** API key via `OPENAI_API_KEY` env var
- **Integration:** `internal/provider/openai/` (done)

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| gpt-4o-mini | $0.15 | $0.60 | 128K | Yes | Budget option, very capable |
| gpt-4.1 | $2.00 | $8.00 | 1M | Yes | Newest, strong coding |
| gpt-4o | $2.50 | $10.00 | 128K | Yes | General-purpose flagship |
| o1 | $5.00 | $20.00 | 200K | Yes | Reasoning model |

**Cost optimization:** Cached input at 50% off; Batch API at 50% off both input and output.

### Ollama (Self-hosted)

- **Instance:** CT 300 at `192.168.1.207:11434` (Proxmox VM, RTX 3090 Ti)
- **Status:** Active, 296 tok/s throughput
- **Auth:** Basic auth via Caddy reverse proxy on CT 201
- **Integration:** `internal/provider/ollama/` (done)

| Model | Cost | Context | Tool Use | Notes |
|-------|------|---------|----------|-------|
| qwen2.5-coder:7b | Free | 32K | Limited | Currently installed, code-focused |
| nomic-embed-text | Free | 8K | N/A | Embeddings only |
| Any GGUF model | Free | Varies | Varies | Can pull on demand via `ollama pull` |

**Recommendations for Ollama:** Pull `qwen2.5-coder:32b` for higher-quality code tasks
(requires ~20GB VRAM, fits on 3090 Ti). Consider `deepseek-coder-v2:16b` or
`codellama:34b` for variety.

## Tier 2: Low-Effort Integration (OpenAI-Compatible API)

These providers use the OpenAI chat completions API format. Samverk's existing OpenAI
provider can connect to them by changing `base_url` and `api_key_env` in `providers.yaml`.
No new Go code required.

### DeepSeek

- **Account URL:** [platform.deepseek.com](https://platform.deepseek.com/)
- **Auth:** API key via `DEEPSEEK_API_KEY`
- **API Base:** `https://api.deepseek.com/v1`
- **Free tier:** None (pay-as-you-go, but extremely cheap)

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| deepseek-chat (V3.2) | $0.28 | $0.42 | 128K | Yes (chat mode) | Unified chat model |
| deepseek-reasoner (V3.2) | $0.28 | $0.42 | 128K | No (uses chat internally) | Reasoning mode, same model |

**Standout feature:** Automatic context caching -- cache hits cost $0.028/M (90% off).
Tool use in reasoner mode silently falls back to chat mode.

**Recommendation:** Best cost-to-quality ratio for bulk agent tasks. Add second after Gemini.

### Groq

- **Account URL:** [console.groq.com](https://console.groq.com/)
- **Auth:** API key via `GROQ_API_KEY`
- **API Base:** `https://api.groq.com/openai/v1`
- **Free tier:** Yes (rate-limited, no credit card required)

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| llama-3.1-8b-instant | $0.05 | $0.08 | 131K | Yes | Ultra-fast, ultra-cheap |
| qwen3-32b | $0.20 | $0.60 | 131K | Yes | Good mid-range |
| llama-3.3-70b-versatile | $0.59 | $0.79 | 131K | Yes | Strong general-purpose |

**Standout feature:** Fastest inference in the industry (LPU hardware). Batch API at 50% off.

**Recommendation:** Best for latency-sensitive triage and lightweight agent tasks. Free tier
is generous enough for development and testing.

### Mistral

- **Account URL:** [console.mistral.ai](https://console.mistral.ai/)
- **Auth:** API key via `MISTRAL_API_KEY`
- **API Base:** `https://api.mistral.ai/v1`
- **Free tier:** Limited free tier available

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| mistral-nemo | $0.02 | $0.06 | 128K | Yes | Cheapest option available |
| mistral-medium-3 | $0.40 | $2.00 | 128K | Yes | Mid-tier |
| mistral-large-3 (2512) | $0.50 | $1.50 | 128K | Yes | Flagship, strong reasoning |

**Standout feature:** Mistral Large 3 is surprisingly cheap for its capability level.
European provider (data sovereignty option).

**Recommendation:** Good third or fourth provider. Mistral Large 3 at $0.50/$1.50 competes
well with GPT-4o at $2.50/$10.00.

### Together.ai

- **Account URL:** [api.together.xyz](https://api.together.xyz/)
- **Auth:** API key via `TOGETHER_API_KEY`
- **API Base:** `https://api.together.xyz/v1`
- **Free tier:** $5 free credits on sign-up

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| Qwen2.5-72B | $1.20 | $1.20 | 128K | Yes | Strong open model |
| Llama-3.1-70B | $0.88 | $0.88 | 128K | Yes | Meta flagship |
| Mixtral-8x7B | $0.60 | $0.60 | 32K | Yes | Fast mixture-of-experts |

**Standout feature:** 200+ open models available. Good for experimentation and benchmarking.

**Recommendation:** Lower priority -- similar models available cheaper via Groq or Fireworks.

### Fireworks

- **Account URL:** [fireworks.ai](https://fireworks.ai/)
- **Auth:** API key via `FIREWORKS_API_KEY`
- **API Base:** `https://api.fireworks.ai/inference/v1`
- **Free tier:** $1 free credits on sign-up

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| qwen3-8b | $0.20 | $0.20 | 128K | Yes | Fast, cheap |
| gpt-oss-120b | $0.26 | $0.26 | 128K | Yes | Open GPT variant |
| Various fine-tuned | $0.50+ | $0.50+ | Varies | 15/16 models | Fine-tuning available |

**Standout feature:** Batch processing at 40% off. Custom fine-tuning at $0.50/M tokens.

**Recommendation:** Lower priority -- Groq and DeepSeek offer better value for Samverk's
use case.

## Tier 3: Medium-Effort Integration (New Provider Code)

### Google Gemini

- **Account URL:** [aistudio.google.com](https://aistudio.google.com/)
- **Auth:** API key via `GOOGLE_API_KEY`
- **API Base:** `https://generativelanguage.googleapis.com/v1beta`
- **Free tier:** Yes -- 1,000 requests/day, no credit card required

| Model | Input $/1M | Output $/1M | Context | Tool Use | Notes |
|-------|------------|-------------|---------|----------|-------|
| gemini-2.5-flash-lite | $0.10 | $0.40 | 1M | Yes | Cheapest paid option |
| gemini-2.5-flash | $0.15 | $0.60 | 1M | Yes | Great value |
| gemini-2.5-pro | $1.25 | $10.00 | 1M | Yes | Flagship reasoning |
| gemini-3-pro-preview | $2.00 | $12.00 | 1M | Yes | Preview, newest |

**Free tier details:** 5-15 RPM depending on model, 250K tokens/minute, all models
available including Pro. No billing setup required.

**Standout feature:** Generous free tier makes this ideal for development and testing.
1M context window on all models. Batch API at 50% off.

**Integration notes:** Gemini uses its own API format (not OpenAI-compatible natively).
Requires a new `internal/provider/gemini/` package. Google provides official Go SDK
(`google.golang.org/genai`). Tool use is well-supported with function declarations.

**Recommendation:** Add first among new providers. Free tier alone justifies the effort.
Flash models offer the best price/performance for high-volume agent work.

## Tier 4: Subscription-Based (No Direct API Cost)

### GitHub Copilot

- **Account URL:** [github.com/settings/copilot](https://github.com/settings/copilot)
- **Subscription:** Active (Pro or Pro+ plan)
- **Auth:** OAuth token or PAT

**Models available via Copilot subscription:**

- GPT-5 mini, GPT-4.1, GPT-4o (included, unlimited)
- Claude Opus 4.6, Claude Sonnet 4.6 (premium requests)
- Gemini 3 Pro (premium requests)
- Claude Haiku 4.5 (included for quick tasks)

**Copilot SDK (Technical Preview):**

- SDKs for Node.js, Python, Go, and .NET
- Embeds Copilot's agentic execution loop in any app
- Custom tool definitions supported
- [github.com/github/copilot-sdk](https://github.com/github/copilot-sdk)

**Integration assessment:** The SDK is in technical preview. It provides access to
multiple frontier models under one subscription, but:

- Rate limits are subscription-tier dependent
- Model availability varies by plan
- SDK API may change before GA
- Not designed for high-throughput batch processing

**Recommendation:** Monitor for GA release. The subscription model is attractive (pay
once, access multiple models) but the SDK is too early for production integration.

### Microsoft 365 Copilot

- **Account URL:** [admin.microsoft.com](https://admin.microsoft.com/)
- **Subscription:** Active M365 license

**What "agent tokens" actually are:** M365 Copilot uses "Copilot Credits" as a metered
billing unit. Agents that access SharePoint or Copilot connectors consume credits.
Credits are sold in packs of 25,000 at $200/pack/month, or pay-as-you-go at $0.01/credit.

**This is NOT a general-purpose AI API.** M365 Copilot is designed for:

- Building agents that interact with Microsoft Graph data
- Declarative agents grounded in SharePoint/OneDrive
- Enterprise workflow automation within the M365 ecosystem

**Integration assessment:** Not suitable for Samverk's provider model. The API operates
on Copilot Credits (not tokens), requires MSAL/OAuth2 auth against Azure AD, and is
tightly coupled to Microsoft Graph. The AI models (GPT-4/5, Claude) are accessed
indirectly through the Copilot runtime, not as raw completions.

**Recommendation:** Do not integrate. The architecture is fundamentally different from
Samverk's provider model. Use Anthropic/OpenAI APIs directly instead.

## Integration Priority

Based on cost, capability, tool-use support, and integration effort:

### Priority 1: Google Gemini

- **Why:** Free tier (1,000 req/day), excellent tool use, 1M context
- **Effort:** New provider package (~200 lines), Google Go SDK available
- **Use case:** Development/testing without cost, QC validation (cross-model)

### Priority 2: DeepSeek

- **Why:** Cheapest quality provider ($0.28/$0.42), OpenAI-compatible
- **Effort:** Config-only (reuse OpenAI provider with different base_url)
- **Use case:** Bulk agent tasks, cost-sensitive workflows

### Priority 3: Groq

- **Why:** Fastest inference, free tier, OpenAI-compatible
- **Effort:** Config-only (reuse OpenAI provider with different base_url)
- **Use case:** Triage, lightweight analysis, latency-sensitive tasks

### Priority 4: Mistral

- **Why:** Strong models at competitive prices, OpenAI-compatible
- **Effort:** Config-only
- **Use case:** European data sovereignty option, cost diversity

### Not Recommended

- **Together.ai / Fireworks:** Similar models available cheaper elsewhere
- **GitHub Copilot SDK:** Too early (technical preview), not designed for batch work
- **M365 Copilot:** Wrong architecture (credits, not tokens; enterprise, not dev tools)

## Provider Config Examples

### Current providers.yaml structure

```yaml
providers:
  claude:
    type: claude
    api_key_env: ANTHROPIC_API_KEY
    default_model: claude-sonnet-4-6
    timeout_seconds: 300

  openai:
    type: openai
    api_key_env: OPENAI_API_KEY
    default_model: gpt-4o
    timeout_seconds: 120

  ollama:
    type: ollama
    base_url: http://192.168.1.207:11434
    default_model: qwen2.5-coder:7b
    timeout_seconds: 600
```

### Adding OpenAI-compatible providers (config-only)

```yaml
  deepseek:
    type: openai
    api_key_env: DEEPSEEK_API_KEY
    base_url: https://api.deepseek.com/v1
    default_model: deepseek-chat
    timeout_seconds: 120

  groq:
    type: openai
    api_key_env: GROQ_API_KEY
    base_url: https://api.groq.com/openai/v1
    default_model: llama-3.3-70b-versatile
    timeout_seconds: 30

  mistral:
    type: openai
    api_key_env: MISTRAL_API_KEY
    base_url: https://api.mistral.ai/v1
    default_model: mistral-large-latest
    timeout_seconds: 120
```

### Adding Gemini (requires new provider code)

```yaml
  gemini:
    type: gemini
    api_key_env: GOOGLE_API_KEY
    default_model: gemini-2.5-flash
    timeout_seconds: 120
```

### Routing with multiple providers

```yaml
routing:
  default: [claude, openai, gemini, ollama]
  agent:code-gen: [claude, deepseek, openai]
  agent:research: [gemini, claude, groq]
  agent:test: [deepseek, groq, ollama]
  agent:qc: [gemini, claude]          # cross-model validation (ADR-030)
  agent:triage: [groq, gemini, ollama] # fast, cheap triage
```

## Free Tiers and Trial Credits

| Provider | Free Tier | Details |
|----------|-----------|---------|
| Google Gemini | 1,000 req/day | All models, no credit card, indefinite |
| Groq | Rate-limited free | No credit card required |
| Together.ai | $5 credits | One-time sign-up bonus |
| Fireworks | $1 credits | One-time sign-up bonus |
| Mistral | Limited free | Rate-limited access |
| DeepSeek | None | Pay-as-you-go only (very cheap) |
| Anthropic | None | Pay-as-you-go |
| OpenAI | None | Pay-as-you-go (account has $0) |

## Downstream Issues

This inventory enables:

- **Gemini provider implementation** -- new `internal/provider/gemini/` package
- **OpenAI-compatible provider config** -- DeepSeek, Groq, Mistral via config-only
- **Cross-model QC routing** -- ADR-030 implementation with diverse providers
- **Cost-optimized routing** -- route cheap tasks to cheap providers
- **Free-tier development** -- Gemini + Groq for zero-cost dev/test loops
