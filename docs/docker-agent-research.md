# Docker Containers as Agent Workspaces

Research evaluation for issue #527. Assesses Docker containers as an
alternative or complement to git worktrees for Samverk agent isolation.

## Executive Summary

**Recommendation: Defer implementation. Worktrees remain the right default.**

Docker containers offer stronger isolation and resource limits, but introduce
significant complexity (auth forwarding, image management, LXC nesting
constraints) for marginal benefit given Samverk's current scale and deployment
model. The worktree approach (implemented in #517) is working well, is fast to
create, and provides sufficient isolation for the agent workload.

Docker containers become worth revisiting when:

- Agents need compilation environments that conflict with the host (e.g.,
  different Go versions, conflicting system libraries)
- Runaway agents cause host resource contention that `nice`/`ulimit` cannot
  solve
- Multi-tenant deployment requires hard security boundaries between agents

## 1. Claude CLI in Docker

### Feasibility: Proven

Multiple community projects and official Anthropic documentation confirm Claude
Code CLI runs in headless Docker containers. The standard pattern is:

```dockerfile
FROM node:20-slim
RUN npm install -g @anthropic-ai/claude-code
RUN useradd -m agent
USER agent
ENTRYPOINT ["claude"]
```

### Authentication Options

| Method | Mechanism | Billing | Samverk Fit |
|--------|-----------|---------|-------------|
| `ANTHROPIC_API_KEY` env var | Pass API key at `docker run` | API pay-as-you-go | Good for API providers |
| OAuth volume mount | Mount `~/.claude/` into container | Max plan (subscription) | Good for CLI provider |
| `ANTHROPIC_BASE_URL` override | Point to local proxy or Ollama | Depends on backend | Good for free-first routing |

Samverk's `claudecli` provider currently strips `ANTHROPIC_API_KEY` and relies
on OAuth credentials in `~/.claude/`. For Docker, the equivalent is:

```bash
docker run -v ~/.claude:/home/agent/.claude:ro \
  --env ANTHROPIC_API_KEY= \
  samverk-agent claude --print --dangerously-skip-permissions
```

The `--dangerously-skip-permissions` flag is required for headless operation.
The official [devcontainer documentation](https://code.claude.com/docs/en/devcontainer)
endorses this pattern when running inside isolated containers.

**Caveat:** `--dangerously-skip-permissions` is blocked when running as root
(see KG#129). The container must use a non-root user.

### Security Note

Volume-mounting `~/.claude/` read-only prevents credential modification but
still exposes OAuth tokens to the container. A compromised agent could
exfiltrate tokens. For untrusted workloads, use `ANTHROPIC_API_KEY` with a
scoped key instead.

## 2. Startup Overhead

### Estimated Timeline: Container Start to First File Edit

| Phase | Worktree (current) | Docker (cached image) | Docker (cold pull) |
|-------|--------------------|-----------------------|--------------------|
| Workspace creation | ~1s (`git worktree add`) | ~1-2s (`docker run`) | 30-120s (image pull) |
| Repo access | Instant (shared .git) | 3-8s (shallow clone) | 3-8s (shallow clone) |
| Tool initialization | N/A (host tools) | ~1s (pre-installed) | ~1s (pre-installed) |
| Claude CLI startup | ~2s | ~2s | ~2s |
| **Total** | **~3s** | **~7-13s** | **~37-133s** |

Key observations:

- **Worktrees are 3-4x faster** for warm starts because they share the host's
  `.git` directory and require no clone
- **Shallow clone** (`git clone --depth 1`) reduces the Docker penalty to
  seconds rather than minutes for most repos
- **Image caching** is critical. First pull of a ~500MB agent image is the
  dominant cost. After that, container start is sub-second
- **Volume-mounting the repo** instead of cloning eliminates the clone overhead
  but weakens isolation (agent writes affect the host filesystem)

### Recommendation

For Samverk's repo size (~50MB), shallow clone adds ~5s. This is acceptable
for code-gen tasks that run for minutes, but excessive for triage tasks that
complete in seconds. This reinforces the hybrid approach: worktrees for fast
tasks, containers for heavy tasks.

## 3. Resource Limits

Docker provides fine-grained resource controls that worktrees lack entirely:

```bash
docker run \
  --cpus 2 \
  --memory 4g \
  --memory-swap 4g \
  --pids-limit 256 \
  --ulimit nofile=1024:1024 \
  --network samverk-net \
  samverk-agent
```

| Resource | Docker Flag | Worktree Equivalent |
|----------|------------|---------------------|
| CPU | `--cpus N` | `nice -n 19` (advisory only) |
| Memory | `--memory Ng` | `ulimit -v` (per-process, not group) |
| Swap | `--memory-swap` | None |
| Process count | `--pids-limit` | None |
| Disk I/O | `--blkio-weight` | `ionice` (advisory) |
| Network | `--network` | None |
| Filesystem | Container layer (ephemeral) | Shared host filesystem |

### Current Risk Assessment

Samverk agents on CT 202 (8 GB RAM, 6 cores) run at most 2 concurrent tasks.
The Claude CLI process itself uses ~200-400 MB RAM. Go compilation in a
worktree peaks at ~1-2 GB. Current utilization is well within limits.

Resource limits become necessary when:

- Scaling beyond 3-4 concurrent agents on CT 202
- Running agents that invoke `npm install` or large Go compilations
- A provider bug causes infinite output or infinite tool loops

### Interim Alternative

Before Docker, cgroups v2 can limit worktree-based agents on Linux:

```bash
systemd-run --scope --user -p MemoryMax=4G -p CPUQuota=200% \
  claude --print --dangerously-skip-permissions
```

This provides memory and CPU limits without container overhead.

## 4. Decision Matrix: Worktree vs Container

| Dimension | Worktree | Docker Container |
|-----------|----------|-----------------|
| **Startup time** | ~3s | ~7-13s (cached) |
| **Isolation** | Branch-level (shared filesystem) | Full (separate filesystem, PID, network) |
| **Resource limits** | None (host-level only) | Fine-grained (CPU, memory, PID, I/O) |
| **Cleanup** | `git worktree remove` + prune | `docker rm` (atomic) |
| **Disk overhead** | ~50 MB (hardlinks to .git) | ~500 MB base image + ~50 MB clone |
| **Concurrent scaling** | Limited by git lock contention | Limited by host resources only |
| **Host tool access** | Full (Go, node, git on host) | Must be pre-installed in image |
| **Network isolation** | None | Full (bridge/custom networks) |
| **Complexity** | Low (git-native) | Medium (image build, auth, networking) |
| **LXC compatibility** | Native | Requires Docker-in-LXC or separate VM |

### Task-Type Routing

| Task Type | Recommended | Rationale |
|-----------|-------------|-----------|
| Triage (`agent:triage`) | Worktree | Completes in seconds; container overhead unjustified |
| Docs (`agent:docs`) | Worktree | Read-heavy, no compilation, low risk |
| Research (`agent:research`) | Worktree | No file writes, comment-only output |
| Code-gen (`agent:code-gen`) | Worktree (default), Container (future) | Compilation benefits from limits but worktree works today |
| Test (`agent:test`) | Worktree (default), Container (future) | Test execution could benefit from isolation |
| Infrastructure (`agent:infra`) | Container (when available) | SSH/deployment tasks benefit from network isolation |

## 5. Container Image Design

### Proposed Base Image

```dockerfile
FROM node:20-slim AS base

# System dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    openssh-client \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Go toolchain (for code-gen agents)
COPY --from=golang:1.24 /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

# Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Non-root agent user
RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent/workspace

ENTRYPOINT ["claude", "--print", "--dangerously-skip-permissions"]
```

### Size Estimates

| Layer | Size |
|-------|------|
| `node:20-slim` base | ~180 MB |
| Git + system tools | ~50 MB |
| Go 1.24 toolchain | ~250 MB |
| Claude Code CLI (`@anthropic-ai/claude-code`) | ~50 MB |
| **Total (compressed)** | **~350 MB** |
| **Total (uncompressed on disk)** | **~530 MB** |

### Variant Images

For tasks that do not need Go (triage, docs, research), a slim variant
without the Go toolchain saves ~250 MB:

| Image | Contents | Size |
|-------|----------|------|
| `samverk-agent:full` | Node + Go + git + Claude CLI | ~530 MB |
| `samverk-agent:slim` | Node + git + Claude CLI | ~280 MB |
| `samverk-agent:frontend` | Node + git + Claude CLI + pnpm | ~300 MB |

## 6. GPU Passthrough

### Requirements

Docker GPU passthrough requires:

1. NVIDIA driver installed on the host
2. `nvidia-container-toolkit` installed
3. `--gpus` flag at `docker run`

```bash
docker run --gpus all -e OLLAMA_HOST=http://host.docker.internal:11434 \
  samverk-agent
```

### LXC Constraint (CT 202)

CT 202 is an LXC container on Proxmox. Running Docker inside LXC (nested
containerization) is officially unsupported and fragile:

- Requires `features: nesting=1` in LXC config
- GPU passthrough through LXC to Docker requires manual device node mapping
  and `no-cgroups = true` in NVIDIA container runtime config
- Storage driver conflicts between LXC overlay and Docker overlay2
- Community reports of data corruption under heavy I/O

**CT 202 verdict: Do not run Docker-in-LXC for agent workspaces.**

### Viable GPU Paths

| Option | Host | GPU | Feasibility |
|--------|------|-----|-------------|
| Docker on VM 300 | Proxmox VM | RTX 3090 Ti | High -- VM supports native Docker + GPU |
| Docker on HDH-NZXT | Bare metal Windows | RTX 5090 | Medium -- Docker Desktop + WSL2 GPU |
| Docker on dedicated VM | New Proxmox VM | Passed-through GPU | High -- clean setup |

For Samverk's current Ollama routing, GPU passthrough into agent containers is
unnecessary. Agents call Ollama over HTTP (`OLLAMA_HOST`), not locally. GPU
passthrough would only matter if running a model inside the agent container
itself, which is not the current architecture.

## 7. Networking

### Requirements

Agent containers need outbound access to:

| Service | Protocol | Location |
|---------|----------|----------|
| Samverk MCP | HTTPS | `samverk.herbhall.net` (Cloudflare Tunnel) |
| Synapset MCP | HTTPS | `synapset.herbhall.net` |
| GitHub API | HTTPS | `api.github.com` |
| Gitea API | HTTPS | `gitea.herbhall.net` / `192.168.1.160:3000` |
| Ollama | HTTP | `192.168.1.202:11434`, `192.168.1.207:11434` |
| Anthropic API | HTTPS | `api.anthropic.com` |

### Network Options

| Mode | Pros | Cons |
|------|------|------|
| `--network host` | Simplest; full host network access | No isolation; defeats purpose |
| `--network bridge` (default) | Container gets own IP; NAT for outbound | Must expose/map ports for inbound |
| Custom bridge | Named network; containers can discover each other | Slightly more setup |
| `--network none` + explicit | Maximum isolation; allowlist outbound | Complex iptables rules |

### Recommendation

Use the default bridge network. Agent containers only need outbound HTTPS
and HTTP. No inbound ports are required (agents push results via API calls,
not by receiving connections). DNS resolution works out of the box on bridge
networks.

For the firewall-hardened pattern from the official devcontainer docs:

```bash
# Allow DNS, Anthropic API, and local services only
iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 443 -j ACCEPT
iptables -A OUTPUT -p tcp -d 192.168.1.0/24 -j ACCEPT
iptables -A OUTPUT -j DROP
```

This prevents a compromised agent from reaching arbitrary external hosts.

## 8. Cost/Benefit Analysis

### Benefits

| Benefit | Impact | When It Matters |
|---------|--------|-----------------|
| Hard resource limits | Prevents runaway agents from starving host | 3+ concurrent agents |
| Atomic cleanup | `docker rm` vs worktree prune + branch delete | Always (minor improvement) |
| No git lock contention | Each container has its own repo | 4+ concurrent code-gen agents |
| Reproducible environments | Same image across dev/staging/prod | Multi-host deployment |
| Network isolation | Prevent agents from accessing unintended services | Security-sensitive tasks |
| Filesystem isolation | Agent cannot read/write host files | Untrusted agent code |

### Costs

| Cost | Impact | Mitigation |
|------|--------|------------|
| Startup overhead | +5-10s per task (cached) | Pre-warm containers; use worktrees for fast tasks |
| Disk usage | ~530 MB per image + ~50 MB per container | Image sharing; periodic `docker system prune` |
| Image maintenance | Must rebuild when Go/Node versions change | CI pipeline for image builds |
| Auth complexity | OAuth token forwarding; credential security | Read-only volume mount |
| LXC incompatibility | CT 202 cannot run Docker natively | Separate Docker host (VM 300 or new VM) |
| Operational complexity | Docker daemon, image registry, networking | Adds a new infrastructure dependency |
| Shallow clone overhead | ~5s per container start | Volume-mount for trusted agents |

### Break-Even Analysis

The Docker approach becomes cost-effective when:

1. **Concurrent agents exceed 3-4** on a single host (resource contention)
2. **Agent tasks require different toolchains** (Go 1.24 vs Go 1.23,
   Node 20 vs Node 22)
3. **Security model requires hard boundaries** (multi-tenant, untrusted code)
4. **Cleanup reliability matters** (stale worktrees are a known nuisance --
   see `PruneStaleWorktrees` in `workspace.go`)

None of these conditions are pressing today. CT 202 runs 1-2 concurrent
agents, all using the same Go/Node versions, in a single-tenant setup.

## Implementation Roadmap (If Pursued)

### Phase 1: Image and Local Testing

- Build `samverk-agent:full` and `samverk-agent:slim` images
- Test Claude CLI headless operation with OAuth volume mount
- Benchmark startup time (container create + shallow clone + first edit)
- Validate on Docker Desktop (Windows) and Docker Engine (Linux VM)

### Phase 2: Runner Integration

- Add `ContainerWorkspace` alongside `CreateWorkspace` in `workspace.go`
- New config field: `workspace_mode: "worktree" | "container"`
- Task-type routing: triage/docs use worktree, code-gen/test use container
- Docker SDK integration (`github.com/docker/docker/client`)

### Phase 3: Infrastructure

- Deploy Docker Engine on VM 300 or a new Proxmox VM
- Set up private image registry (or use Gitea's built-in registry)
- CI pipeline for image builds on Go/Node version bumps
- Monitoring: container resource usage metrics in host dashboard

### Phase 4: GPU and Scaling

- GPU passthrough for Ollama-backed agents on VM 300
- Auto-scaling: spawn containers across multiple Docker hosts
- Container pool: pre-warmed containers for instant startup

## References

- [Claude Code Devcontainer Docs](https://code.claude.com/docs/en/devcontainer) -- official reference devcontainer setup
- [claudebox](https://github.com/RchGrav/claudebox) -- community Docker environment for Claude Code
- [claude-code-container](https://github.com/tintinweb/claude-code-container) -- minimal headless container
- [Docker Sandboxes](https://www.docker.com/blog/docker-sandboxes-run-claude-code-and-other-coding-agents-unsupervised-but-safely/) -- Docker's official agent sandboxing
- [Claude Code Docker Tutorial](https://www.datacamp.com/tutorial/claude-code-docker) -- DataCamp walkthrough
- [Proxmox LXC GPU Passthrough](https://digitalspaceport.com/proxmox-lxc-docker-gpu-passthrough-setup-guide/) -- LXC + Docker + GPU guide
- [Claude Code Auth Issue #1736](https://github.com/anthropics/claude-code/issues/1736) -- Docker re-authentication discussion
- [Docker GPU Passthrough Docs](https://docs.docker.com/config/containers/resource_constraints/#gpu) -- official GPU resource constraints
- Samverk workspace implementation: [`internal/agent/workspace.go`](../internal/agent/workspace.go)
- Samverk Claude CLI provider: [`internal/provider/claudecli/claudecli.go`](../internal/provider/claudecli/claudecli.go)
