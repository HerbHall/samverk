# ADR-019: Self-Hosted-First Development

## Status

Accepted

## Context

The original architecture described GitHub as the primary forge with Gitea as an alternative. However, the founder's actual development environment is a home network with:

- Gitea already running and available for testing
- An RTX 3090 Ti (24GB VRAM) available for a dedicated project server
- Multiple server platform options (Unraid, Proxmox, Windows)

Building against cloud services first (GitHub, Claude API) would mean:

- API rate limits constrain development speed
- Every test run costs money
- The local agent story is deferred to "later"
- The forge abstraction is not validated until a second implementation exists

Building against local infrastructure first inverts these tradeoffs.

## Decision

Samverk development targets self-hosted infrastructure as the primary environment:

- **Gitea** is the primary forge target (first implementation of the forge abstraction)
- **Ollama** on local GPU is the primary model target for specialist agents
- **Cloud APIs** (Claude, GPT-4) are used for complex reasoning and as fallback
- **GitHub** support is the second forge implementation, validating the abstraction

Development and testing happen on the home network. Cloud services are added as the system matures, not as the starting point.

### Infrastructure

- **Available now:** Gitea instance for testing, existing server hardware
- **Planned:** Dedicated project server with RTX 3090 Ti for Ollama
- **Platform decision pending:** Unraid, Proxmox, or Windows as the server OS

### Local Model Capabilities (RTX 3090 Ti, 24GB VRAM)

The 3090 Ti can run substantial models:

- **Code generation:** CodeLlama 34B (Q4 quantized), DeepSeek Coder 33B
- **General reasoning:** Mixtral 8x7B, Llama 3 70B (Q4)
- **Fast/small tasks:** Llama 3 8B, Phi-3, CodeGemma 7B
- **Multiple models simultaneously:** Smaller models can share VRAM

This means local agents are not limited to toy models. Serious code generation, test writing, and documentation tasks can run entirely on local hardware.

## Consequences

- No cloud API costs during early development and testing
- Gitea forge implementation is exercised from day one
- Forge abstraction is validated early (Gitea first, GitHub second)
- Server platform decision becomes a near-term prerequisite
- Network configuration and security are development concerns from the start
- GitHub-specific features (Actions, Copilot integration) are deferred

## References

- [ADR-013: Abstract Git Forge Behind Interface](ADR-013-forge-abstraction.md)
- [ADR-018: Release Versioning and V1 Scope](ADR-018-release-versioning.md)
- [ADR-007: Hybrid Local/Cloud Agent Model](ADR-007-hybrid-local-cloud.md)
