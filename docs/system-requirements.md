# System Requirements

Hardware and software requirements for running Samverk with local AI agents.

Samverk adapts to whatever hardware you have. More GPU means more local work, lower cloud costs, and faster turnaround -- but every tier produces the same result. Your project ships regardless.

## Minimum Requirements

The absolute floor for running one useful local agent alongside the Samverk server.

| Component | Requirement | Notes |
|-----------|-------------|-------|
| **GPU** | NVIDIA, 6 GB+ VRAM | Runs Qwen 2.5 Coder 3B or Llama 3.2 3B (Q4) |
| **CPU** | 4+ cores | Go server + Docker + Ollama overhead |
| **RAM** | 16 GB | OS + Docker + Go server + Ollama runtime |
| **Storage** | 20 GB free | Model files (~2-5 GB each) + Docker images + SQLite |
| **OS** | Linux (primary), Windows with WSL2, macOS with Docker Desktop | See Operating System Notes below |
| **Docker** | Docker Engine or Docker Desktop + NVIDIA Container Toolkit | Required for local agents |

At this tier you get one local agent at a time. The dispatcher routes to a single small model for code generation, and anything requiring deeper reasoning (QC, architecture decisions) falls back to a cloud provider. This maps to **Cost Model Tier 2** -- one cloud subscription plus local.

A 6 GB GPU fits Qwen 2.5 Coder 7B at Q4 quantization (~5 GB) with minimal headroom, or comfortably runs 3B-class models. Expect ~80-100 tokens/second on a 7B model, which is more than fast enough for async background work.

## Recommended Requirements

The sweet spot for 2-3 concurrent local agents running different specialist models.

| Component | Requirement | Notes |
|-----------|-------------|-------|
| **GPU** | NVIDIA, 12-24 GB VRAM (RTX 3060 12 GB to RTX 3090/4090) | 2-3 concurrent models |
| **CPU** | 8+ cores | Comfortable headroom for parallel agent work |
| **RAM** | 32 GB | Multiple Docker containers + Ollama model loading |
| **Storage** | 50 GB free | 3-5 models cached + Docker images + build artifacts |

This is where Samverk's multi-agent architecture comes alive. With 24 GB VRAM, the "Balanced" deployment strategy from the local model survey fits comfortably:

```text
Always loaded:
  Qwen 2.5 Coder 7B Q4    (~5 GB)   -- code generation + tests
  DeepSeek R1-Distill 14B  (~9 GB)   -- QC + reasoning
  Llama 3.2 3B Q4          (~2.3 GB) -- dispatch + classify
Total: ~16.3 GB, leaving ~8 GB headroom
```

At 12 GB VRAM (RTX 3060), you fit two concurrent models -- typically a code-gen model plus a lightweight dispatcher. QC and reasoning tasks rotate in by evicting idle models (5-15 second reload, invisible in async workflows).

This maps to **Cost Model Tier 4** -- local models handle the volume work (code generation, tests, documentation), cloud handles the highest-complexity decisions.

## Optimal Requirements

For full local-first operation with minimal cloud fallback.

| Component | Requirement | Notes |
|-----------|-------------|-------|
| **GPU** | NVIDIA, 24 GB+ VRAM (RTX 3090 Ti, RTX 4090, RTX 5090) | 4-5 concurrent models |
| **CPU** | 8+ cores | |
| **RAM** | 64 GB | Generous headroom for large context windows |
| **Storage** | 100 GB free | Multiple model variants cached at different quantizations |

At 24 GB you can run 4+ concurrent specialist models or load a single 32B model for maximum code quality:

| Configuration | Models | VRAM | Headroom |
|---------------|--------|------|----------|
| Triple agent | Coder 7B + R1 7B + Llama 3B | ~12.3 GB | ~12 GB |
| Dual specialist | Coder 14B + R1 14B | ~18 GB | ~6 GB |
| Quad lightweight | Coder 7B + R1 7B + Phi-4 14B + Llama 3B | ~21.3 GB | ~3 GB |
| Solo quality | Qwen 2.5 Coder 32B (sequential) | ~20 GB | ~4 GB |

The 32B model scores 92.7% HumanEval (matching GPT-4o) but monopolizes the GPU -- no concurrent models. For multi-agent workflows, the 7B-14B class models provide the best throughput-to-quality ratio.

Cloud becomes a pure fallback for edge cases, not a regular dependency. This maps to **Cost Model Tier 1** or **Tier 4** depending on whether you add a cloud subscription for the hardest reasoning tasks.

## CPU-Only Fallback

What happens when you have no NVIDIA GPU.

Ollama can run models on CPU, but inference is 10-50x slower than GPU. A 7B model that generates 80-100 tokens/second on an RTX 3090 produces 2-5 tokens/second on CPU. For async background work this is technically functional -- a task that takes 30 seconds on GPU takes 5-10 minutes on CPU.

**Practical CPU-only models:**

| Model | RAM Required | CPU Speed (approx) | Use Case |
|-------|-------------|-------------------|----------|
| Llama 3.2 1B Q4 | ~2 GB | ~10-20 tok/s | Dispatch, classification |
| Qwen 2.5 Coder 3B Q4 | ~3 GB | ~5-10 tok/s | Simple code generation |
| Llama 3.2 3B Q4 | ~3 GB | ~5-10 tok/s | General purpose |

At this tier, local agents are supplementary. Cloud providers (Anthropic Claude, OpenAI) become the primary execution engine, and the cost model shifts to **Tier 2 or Tier 3** -- one or more cloud subscriptions handling the bulk of agent work.

CPU-only is better than nothing. Your project still ships. It just costs more in cloud API fees and tasks take longer to complete.

## Cloud-Only Mode

No local agents at all. The Samverk server runs as a lightweight coordinator.

| Component | Requirement | Notes |
|-----------|-------------|-------|
| **GPU** | None | |
| **CPU** | 2+ cores | Go server + SQLite |
| **RAM** | 4 GB | Minimal -- no model loading |
| **Storage** | 5 GB free | SQLite database + Docker |

All agent work routes to cloud providers (Anthropic, OpenAI, Gemini). The Samverk server handles dispatching, state management, and the check-in interface. Ollama is not required.

**Trade-offs:**

- Zero hardware investment beyond a basic machine
- Higher monthly cost ($50-150/month depending on project activity)
- Subject to cloud provider rate limits and outages
- Full feature parity -- cloud-only does not limit what Samverk can build, only what it costs

This maps to **Cost Model Tier 2 or Tier 3** with cloud handling all work.

## Requirements by Use Case

Quick reference for deciding what you need.

| Use Case | GPU VRAM | RAM | Storage | Concurrent Local Agents | Cost Model Tier |
|----------|----------|-----|---------|------------------------|-----------------|
| Cloud-only (no GPU) | None | 4 GB | 5 GB | 0 (cloud only) | Tier 2-3 |
| Hobby (evenings/weekends) | 6-8 GB | 16 GB | 20 GB | 1 local + cloud | Tier 2 |
| Active development | 12-24 GB | 32 GB | 50 GB | 2-3 local | Tier 4 |
| Full local-first | 24 GB+ | 64 GB | 100 GB | 4-5 local | Tier 1 or 4 |

**Key insight:** Every tier builds the same project. More hardware means faster completion and lower cloud costs, not different features.

## VRAM Budget Reference

How much VRAM each model consumes at common quantization levels. All values include Ollama runtime overhead (~500 MB) and CUDA context (~300 MB) shared across loaded models.

### Model Sizes by Quantization

| Model | Q4_K_M | Q5_K_M | Q8_0 | FP16 |
|-------|--------|--------|------|------|
| Llama 3.2 1B | ~1.2 GB | - | - | - |
| Llama 3.2 3B | ~2.3 GB | - | - | - |
| Llama 3.1 8B | ~5 GB | - | - | - |
| Qwen 2.5 Coder 7B | ~5 GB | ~6 GB | ~8 GB | ~14 GB |
| DeepSeek R1-Distill 7B | ~5 GB | - | - | - |
| Phi-4 14B | ~9 GB | - | - | - |
| Qwen 2.5 Coder 14B | ~9 GB | ~11 GB | ~15 GB | ~28 GB |
| DeepSeek R1-Distill 14B | ~9 GB | - | - | - |
| DeepSeek R1-Distill 32B | ~15 GB | - | - | - |
| Qwen 2.5 Coder 32B | ~20 GB | ~23 GB | ~34 GB | ~64 GB |

### KV Cache Overhead

Context length is the hidden VRAM cost. KV cache grows linearly with context and scales with model size.

| Context Length | 7B Model (f16 cache) | 7B Model (q8_0 cache) | 14B Model (f16 cache) | 14B Model (q8_0 cache) |
|---------------|---------------------|----------------------|----------------------|----------------------|
| 2K | ~100-200 MB | ~50-100 MB | ~200-400 MB | ~100-200 MB |
| 4K | ~200-400 MB | ~100-200 MB | ~400-800 MB | ~200-400 MB |
| 8K | ~400-800 MB | ~200-400 MB | ~800-1600 MB | ~400-800 MB |
| 32K | ~3-5 GB | ~1.5-2.5 GB | ~5-8 GB | ~2.5-4 GB |

**Recommendation:** Set `OLLAMA_KV_CACHE_TYPE=q8_0` and limit `num_ctx` in Modelfiles (4096 for code-gen, 2048 for dispatch). Agent tasks rarely need long contexts -- reserve 32K+ contexts for cloud API calls.

### Concurrent Loading Limits

Models must fit entirely in VRAM for concurrent GPU loading. Ollama does not split concurrent models across GPU and system RAM.

| VRAM | Practical Concurrent Models | Example Configuration |
|------|-----------------------------|----------------------|
| 6 GB | 1 (7B class) | Qwen 2.5 Coder 7B Q4 |
| 8 GB | 1-2 (7B + 1B/3B) | Coder 7B + Llama 3B dispatch |
| 12 GB | 2 (7B + 7B, or 7B + 3B) | Coder 7B + R1 7B, or Coder 7B + Llama 3B |
| 16 GB | 2-3 (7B class) | Coder 7B + R1 7B + Llama 3B |
| 24 GB | 3-4 (7B-14B class) | Coder 7B + R1 14B + Llama 3B (~16 GB) |

When VRAM is full, Ollama evicts the least-recently-used idle model to make room. Model reload takes 5-15 seconds for 7B, 15-30 seconds for 32B. This latency is invisible in Samverk's async workflow.

## Docker Requirements

Samverk uses Docker to run the Ollama inference server as a sidecar container.

### Required Software

| Component | Purpose | Install |
|-----------|---------|---------|
| Docker Engine (Linux) or Docker Desktop (Windows/macOS) | Container runtime | [Docker install guide](https://docs.docker.com/get-docker/) |
| Docker Compose v2 | Service orchestration | Included with Docker Desktop; `apt install docker-compose-plugin` on Linux |
| NVIDIA Container Toolkit | GPU passthrough to containers | [NVIDIA install guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) |

### NVIDIA Container Toolkit

Required for GPU-accelerated local agents. Without it, Ollama falls back to CPU inference.

**Linux:**

```bash
# Add NVIDIA container toolkit repository
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# Install and configure
sudo apt-get update && sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

**Windows (WSL2):** Docker Desktop handles GPU passthrough automatically when WSL2 backend is enabled and NVIDIA drivers are installed on the host.

**macOS:** Apple Silicon Macs use Metal for GPU acceleration. Docker GPU passthrough is not supported -- Ollama should run natively (not in Docker) on macOS for GPU access.

### Verification

```bash
# Verify Docker can access the GPU
docker run --rm --gpus all nvidia/cuda:12.0-base nvidia-smi

# Verify Ollama starts with GPU
docker run --rm --gpus all ollama/ollama:latest ollama --version
```

## Operating System Notes

| OS | GPU Support | Notes |
|----|-------------|-------|
| **Linux** | Full NVIDIA CUDA via Container Toolkit | Primary target. Best performance and simplest setup. |
| **Windows** | NVIDIA CUDA via WSL2 + Docker Desktop | Requires WSL2 backend. Native Windows Docker does not support GPU passthrough. |
| **macOS (Apple Silicon)** | Metal (native Ollama only) | Docker cannot access Metal GPU. Run Ollama natively for GPU; Samverk server in Docker. |
| **macOS (Intel)** | CPU only | No CUDA support. Cloud-only or CPU fallback. |

## Related Documents

- [Local Model Survey](local-model-survey.md) -- model VRAM data, HumanEval scores, agent role mapping
- [Ollama Container Orchestration](ollama-orchestration.md) -- Docker Compose config, VRAM management, dispatcher integration
- [Cost Model](cost-model.md) -- tiered cost structure by hardware investment
- [Concept](concept.md) -- target user and value proposition
- [ADR-007: Hybrid Local/Cloud Agents](decisions/ADR-007-hybrid-local-cloud.md)
- [ADR-019: Self-Hosted-First Development](decisions/ADR-019-self-hosted-first.md)
