# ADR-038: MAX Plan Token Policy

## Status

Accepted

## Date

2026-03-25

## Context

Samverk uses multiple AI providers: local Ollama models (free after hardware) and
cloud Claude models. Claude offers two consumption modes:

1. **API credits** -- pay-per-token via `ANTHROPIC_API_KEY`, billed to the Anthropic account
2. **MAX plan** -- subscription with included token budget, accessed via Claude CLI OAuth

The project needs a clear policy on which mode to use and when, to prevent accidental
API credit consumption and to optimize the token budget.

## Decision

**All Claude providers in production use MAX plan tokens exclusively.** API credits
are disabled by default and require explicit project owner approval to enable.

### Implementation

- Claude providers use `type: claude-cli` (not `type: claude`)
- The `claude-cli` provider strips `ANTHROPIC_API_KEY` from subprocess environment
- `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` are commented out in production env
- Each disabled key has an intent comment explaining why and when it may be re-enabled

### Priority tree

```text
Quality > Cost > Speed
```

Ollama (free) handles volume work. Claude CLI (MAX plan) handles complexity.
Speed is only prioritized when delay causes cascading harm.

## Alternatives Considered

1. **API-first** -- Rejected. Unpredictable cost, no budget ceiling, burns tokens
   that could be used for interactive development sessions.
2. **Ollama-only** -- Rejected for now. Ollama cannot produce structured output
   (EDIT blocks) reliably enough for code-gen. Quality too low for complex tasks.
3. **Mixed API + MAX** -- Deferred. May be appropriate when project revenue covers
   API spend. Requires documented cost plan before enabling.

## Consequences

- Claude CLI tasks consume MAX plan token budget (shared with interactive use)
- Ollama must be improved to handle more task types (reduces Claude dependency)
- API features (streaming, function calling) are unavailable until API is enabled
- No surprise bills -- all AI spend is subscription-based or free (local hardware)
