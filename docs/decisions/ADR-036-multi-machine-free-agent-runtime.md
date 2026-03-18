# ADR-036: Multi-Machine Free Agent Runtime

**Status:** Proposed
**Date:** 2026-03-15
**Supersedes:** None

## Context

Samverk's dispatcher runs all agent tasks on CT 202 via Claude CLI using the
Anthropic Max plan. This architecture has three structural problems:

1. **Cost** — every task, regardless of complexity, consumes paid Max plan
   tokens. Triage and labelling cost the same as architectural code generation.

2. **Reliability** — Claude CLI in headless mode hangs intermittently. The
   current mitigations (stream-json output, 15-second zero-output kill, spawn
   staggering) reduce but do not eliminate the problem.

3. **Single point of failure** — CT 202 is the only execution host. If it is
   down or overloaded, no agent work proceeds.

Meanwhile, the local network has multiple machines with GPUs capable of running
Ollama for free inference. The RTX 5090 on the dev PC delivers ~170 tok/s with
the 35B parameter Qwen model — approaching API speed at zero marginal cost.

## Decision

Distribute agent workloads across multiple machines using Ollama as the primary
runtime. Claude Max on CT 202 becomes a fallback reserved for tasks that
require opus-level reasoning.

### Hardware inventory

| Machine | GPU | VRAM | Network | Best Model | Speed | Availability |
|---------|-----|------|---------|------------|-------|--------------|
| HDH-NZXT native | RTX 5090 | 32 GB | LAN 192.168.1.x | qwen3.5:35b-a3b | ~170 tok/s | Dev PC, may be in use |
| VM 300 (Proxmox) | RTX 3090 Ti | 24 GB | LAN 192.168.1.207 | qwen3-coder:30b | ~112 tok/s | Always on |
| CM-ASUS | RTX 2080 Ti | 11 GB | Tailscale 100.88.37.47 | qwen3.5:9b | ~80 tok/s | Sleeps, needs wake |
| HDH-NZXT Docker | CPU i9-14900K | N/A | LAN (same as native) | qwen2.5-coder:3b | ~30 tok/s | Always on with Docker |
| CT 202 | None | N/A | LAN 192.168.1.162 | Claude CLI (Max plan) | API speed | Always on |

### Provider tiering

Two tiers with free-first routing:

- **Tier 0 (Free):** Ollama on any host. Default for all task types. No cost.
- **Tier 1 (Paid):** Claude CLI on CT 202. Reserved for tasks tagged `complex`
  or explicitly requiring opus-level reasoning.

The dispatcher routes every task to Tier 0 first. Tier 1 is used only when
the task chain is `complex`, when all Tier 0 providers are unhealthy, or when
a Tier 0 attempt fails quality gates.

### Health discovery and proactive checking

The dispatcher checks provider health every 60 seconds:

- **Ollama hosts:** `GET /api/tags` — confirms the service is reachable and
  reports loaded models.
- **Claude CLI:** `claude --version` — confirms the binary is functional.

Each provider entry in the registry tracks: endpoint, `last_healthy` timestamp,
`model_loaded` list, and `vram_available` (reported by Ollama's `/api/ps`
endpoint). Unhealthy providers are removed from routing until the next
successful health check. The existing circuit breaker per provider is extended
with these proactive probes rather than relying solely on task failure signals.

### Availability and Wake-on-LAN

Not all hosts are always available:

- **VM 300** is always on and is the most reliable free provider.
- **HDH-NZXT** may have its GPU in use (gaming, other workloads). The health
  check inspects VRAM availability, not just reachability.
- **CM-ASUS** sleeps. The dispatcher sends a Wake-on-LAN magic packet before
  attempting to route work to it, then waits up to 90 seconds for the health
  check to pass.
- **HDH-NZXT Docker** (CPU-only) is always on but slow. Used as a last-resort
  free provider for lightweight tasks.

Graceful degradation sequence: if all Ollama hosts are down, fall back to
Claude Max on CT 202. If Claude Max is also unavailable, pause dispatch
entirely rather than burning retries against unreachable providers.

### Model capability matrix

VRAM limits constrain which models each host can run. The dispatcher maps task
complexity to minimum model capability:

| Task Type | Minimum Model | Preferred Hosts |
|-----------|---------------|-----------------|
| Triage (label, summarise) | Any (3B+) | HDH-NZXT Docker, CM-ASUS |
| QC (verify, audit) | 7B–13B | CM-ASUS, VM 300 |
| Test writing | 7B–13B | CM-ASUS, VM 300 |
| Code generation | 30B+ | HDH-NZXT native, VM 300 |
| Research / analysis | 30B+ | HDH-NZXT native, VM 300 |
| Complex / architectural | Claude opus | CT 202 |

The dispatcher selects the smallest adequate model on the fastest available
host. A 35B task is never routed to the 11 GB CM-ASUS machine.

### Resource contention

VRAM is an exclusive resource — two 35B models cannot run simultaneously on
the same GPU. Ollama handles this via internal model queuing, but latency
increases significantly when models must be swapped.

Specific contention points:

- **VM 300** also runs `nomic-embed-text` for Synapset. The embedding model
  must coexist with the code generation model. Health checks report VRAM
  usage, not just reachability, so the dispatcher can detect when the code
  model would force a swap.
- **HDH-NZXT** is the dev PC. User activity (gaming, other GPU workloads)
  takes priority. The health check must detect elevated VRAM pressure and
  deprioritise the host rather than competing for the GPU.

### Networking

- **LAN hosts (192.168.1.x):** Low latency, high bandwidth, reliable. Primary
  routing targets.
- **Tailscale hosts (100.x):** 5–10 ms latency, NAT traversal. May disconnect
  if the remote machine sleeps or loses network. Treated as opportunistic
  capacity.
- **Ollama binding:** Each host must bind Ollama to `0.0.0.0` (not localhost)
  to accept remote requests.
- **Security:** Ollama has no built-in authentication. Access is restricted via
  host firewall rules to LAN and Tailscale subnets only. Cloudflare Tunnel is
  not used for Ollama — it remains internal-only.

### Model management

Models must be pulled on each host independently. There is no central registry
or cross-host sync mechanism in Ollama.

- A deploy script ensures consistent model versions across hosts.
- Model updates follow pull-verify-update: pull the new version, verify it
  loads and responds, then update `providers.yaml` to reference it.
- Model drift between hosts is caught by the health check reporting
  `model_loaded` lists.

### Quality gate

Free models may produce lower-quality output than Claude. The dispatcher
applies a post-completion quality check:

- If the output is empty, truncated, or below a minimum token threshold,
  re-route the task to a higher-tier model.
- If a PR created by a free model fails CI, escalate to Claude Max for the
  fix attempt.
- ADR-030 (cross-model QA) already defines the validation pattern. This ADR
  extends it with automatic tier escalation on quality failure.

### Claude Code with Ollama backend

Ollama supports the Anthropic Messages API (since January 2026). Claude Code
CLI can use Ollama as its inference backend via environment variables:

```bash
ANTHROPIC_BASE_URL=http://host:11434
ANTHROPIC_AUTH_TOKEN=ollama
ANTHROPIC_API_KEY=""
```

This gives Claude Code's full tool loop (Read, Edit, Bash) with free
inference. If tool calling works reliably through the Ollama compatibility
layer, this becomes the preferred provider type — the dispatcher spawns
Claude CLI pointed at an Ollama host rather than at the Anthropic API.

This requires validation: tool calling reliability, function signature
support, and multi-turn conversation stability through the compatibility
layer must be tested before production use.

### Claude CLI reliability (parallel track)

Claude CLI headless mode improvements continue as a parallel track:

- Stream-json output format, 15-second zero-output kill, spawn staggering.
- Max plan remains valuable for opus-level reasoning tasks that exceed local
  model capability.
- Claude CLI is not abandoned — it is deprioritised relative to free
  alternatives for routine work.

## Consequences

**Positive:**

- 80%+ cost reduction by routing routine tasks to free local inference.
- Multi-machine redundancy eliminates CT 202 as a single point of failure.
- RTX 5090 delivers near-API-speed inference at zero marginal cost.
- Capability-aware routing ensures task quality matches model capacity.
- Graceful degradation preserves dispatch availability across failure modes.

**Negative:**

- More infrastructure to manage — four Ollama hosts plus Claude CLI, each
  with independent model installations and network configurations.
- Quality variance between models (3B vs 35B vs Claude opus) requires
  active quality gating to prevent regressions.
- Network-dependent — LAN and Tailscale must be reliable for multi-host
  routing to function.
- Dev PC availability is unpredictable — the most powerful GPU is on a
  machine that may be doing other work.

**Risks:**

- Ollama's Anthropic API compatibility layer may not support all tool calling
  features required by Claude Code. Mitigation: validate before production
  use; fall back to direct Ollama API if needed.
- User activity on dev PCs conflicts with inference workloads. Mitigation:
  proactive VRAM health checks deprioritise busy hosts.
- Model version drift across hosts. Mitigation: deploy script and health
  check model reporting.

## Related

- [ADR-019: Self-Hosted First](ADR-019-self-hosted-first.md)
- [ADR-030: Cross-Model QA Validation](ADR-030-cross-model-qa.md)
- [ADR-031: Dual-Forge Operational Model](ADR-031-dual-forge-operational-model.md)
- [docs/adaptive-scaling-plan.md](../adaptive-scaling-plan.md)
