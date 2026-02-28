# Ollama Container Orchestration

Research into managing multiple Ollama containers, model loading, job dispatch, and GPU sharing for local AI agent workloads on a single RTX 3090 Ti (24 GB VRAM).

## Executive Summary

**Recommendation: Single Ollama instance with Docker Compose, managed by Samverk's custom dispatcher.**

For Samverk's use case -- a single developer running async background agents on one GPU -- a single Ollama instance behind Docker Compose is the right starting point. Ollama's built-in model scheduling (load, evict, queue) handles 2-3 concurrent models within 24 GB VRAM without external orchestration. Samverk's dispatcher already knows which agent needs which model, making it the natural routing layer. Kubernetes adds operational complexity with zero benefit at single-node scale. Multi-instance Ollama is unnecessary until throughput demands exceed what one instance can queue.

vLLM should be evaluated as a future upgrade path if concurrent request throughput becomes a bottleneck, but its single-model-per-instance design and heavier setup make it premature for the initial implementation.

## Orchestration Comparison

### Docker Compose (Recommended)

**Pros:**

- Single-file declarative config (`docker-compose.yml`)
- Native GPU passthrough via NVIDIA Container Toolkit
- Health checks, restart policies, volume persistence built-in
- Zero learning curve for the target audience (hobbyist devs)
- Samverk already ships as a single binary -- Compose manages only the Ollama sidecar
- Model files persist across container restarts via named volumes

**Cons:**

- No built-in horizontal scaling (not needed for single-node)
- No automatic failover to other nodes (irrelevant for single-machine)

**Example configuration:**

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    environment:
      - OLLAMA_MAX_LOADED_MODELS=3
      - OLLAMA_NUM_PARALLEL=2
      - OLLAMA_KEEP_ALIVE=10m
      - OLLAMA_KV_CACHE_TYPE=q8_0
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:11434/"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped

volumes:
  ollama_data:
```

### Kubernetes

**Pros:**

- Dynamic Resource Allocation (DRA) for fine-grained GPU scheduling (v1.34+)
- Horizontal Pod Autoscaling (HPA) for multi-instance scaling
- StatefulSets for persistent model storage
- In-place resource resize (v1.33+, KEP-1287)

**Cons:**

- Massive operational overhead for a single-node, single-GPU setup
- Requires NVIDIA GPU Operator, device plugin, and container toolkit stack
- K3s/MicroK8s reduce complexity but still add an orchestration layer with no benefit at this scale
- Target user (hobbyist dev) should not need to learn Kubernetes

**Verdict:** Defer entirely. If Samverk ever targets multi-node clusters with multiple GPUs, revisit. For single-machine deployment, Kubernetes is pure overhead.

### Custom Orchestration (Samverk Dispatcher)

**Pros:**

- Samverk already has a dispatcher agent that routes work to specialist agents
- The dispatcher knows task type, model requirements, and priority -- it is the natural scheduling layer
- Can preload models via Ollama's API before dispatching work
- Can monitor VRAM usage and queue tasks intelligently

**Cons:**

- Must implement model lifecycle management (preload, evict, health check)
- Must handle Ollama API error cases (503 queue full, model load failures)

**Verdict:** This is not an alternative to Docker Compose -- it runs alongside it. Docker Compose manages the Ollama container lifecycle. The Samverk dispatcher manages model routing within the running Ollama instance.

### Comparison Matrix

| Factor | Docker Compose | Kubernetes | Custom (Dispatcher) |
|--------|---------------|------------|---------------------|
| Setup complexity | Low | High | Medium (code) |
| GPU management | Native (NVIDIA toolkit) | DRA/device plugin | Delegates to Ollama |
| Scaling | Manual | Automatic | N/A (single instance) |
| Model routing | None (Ollama handles) | Pod-level | Application-level |
| Target fit | Single node, single GPU | Multi-node cluster | Complements Compose |
| Maintenance burden | Minimal | Significant | Moderate |

## Ollama Architecture Deep-Dive

### API Surface

Ollama exposes an HTTP API on port 11434 with OpenAI-compatible endpoints:

| Endpoint | Purpose | Relevance to Samverk |
|----------|---------|---------------------|
| `POST /api/generate` | Text generation (streaming) | Agent execution |
| `POST /api/chat` | Chat completions (streaming) | Agent execution |
| `POST /api/pull` | Download model from registry | Initial setup, model updates |
| `POST /api/create` | Create model from Modelfile | Custom agent profiles |
| `DELETE /api/delete` | Remove model | Cleanup |
| `POST /api/show` | Model metadata | VRAM estimation |
| `GET /api/tags` | List loaded models | Health monitoring |
| `GET /api/ps` | List running models + VRAM | Resource monitoring |

The `/api/chat` and `/api/generate` endpoints accept a `keep_alive` parameter controlling how long a model stays loaded after the last request. Set to `-1` to keep loaded indefinitely, `0` to unload immediately, or a duration string like `10m`.

### Model Scheduling Internals

Ollama's scheduler operates as follows:

1. **Request arrives** -- scheduler checks if the requested model is loaded
2. **Model loaded** -- request is processed immediately (or queued if `OLLAMA_NUM_PARALLEL` slots are full)
3. **Model not loaded, VRAM available** -- model loads, request queues until ready (5-15 seconds for 7B, 15-30 seconds for 32B)
4. **Model not loaded, VRAM insufficient** -- idle models are evicted (LRU), then the new model loads
5. **Queue full** -- returns HTTP 503 (configurable via `OLLAMA_MAX_QUEUE`, default 512)

Key behaviors:

- Models must fit **entirely** in VRAM for concurrent GPU loading -- no partial GPU allocation
- Ollama prefers single-GPU placement to minimize PCI bus traffic
- `OLLAMA_NUM_PARALLEL` defaults to 4 (or 1 if memory-limited); each parallel slot consumes additional KV cache memory proportional to context length
- Batching: when multiple requests for the same model arrive concurrently, they are grouped for joint processing (no intentional wait to fill batches)

### Configuration Reference

| Variable | Default | Purpose |
|----------|---------|---------|
| `OLLAMA_MAX_LOADED_MODELS` | 3 x GPU count | Max concurrent models in memory |
| `OLLAMA_NUM_PARALLEL` | 4 (or 1 if constrained) | Max parallel requests per model |
| `OLLAMA_MAX_QUEUE` | 512 | Max queued requests before 503 |
| `OLLAMA_KEEP_ALIVE` | 5m | Default model idle timeout |
| `OLLAMA_KV_CACHE_TYPE` | f16 | KV cache quantization (`q8_0`, `q4_0`) |
| `OLLAMA_MAX_MEMORY` | Auto-detected | Override VRAM limit |

### Preloading Models via API

The dispatcher can preload models before assigning work by sending an empty request:

```bash
# Preload a model (non-blocking after initial load)
curl http://localhost:11434/api/chat -d '{"model": "qwen2.5-coder:7b", "keep_alive": "-1"}'

# Check which models are loaded and their VRAM usage
curl http://localhost:11434/api/ps
```

This is the mechanism Samverk should use: the dispatcher preloads the model for the next task while the current task is still running, minimizing cold-start latency.

## Multi-Model Strategies

### Single Instance, Multiple Models (Recommended)

Run one Ollama instance with `OLLAMA_MAX_LOADED_MODELS=3`. The dispatcher manages which models are loaded.

**How it works:**

- Dispatcher sends preload requests before dispatching agent work
- Ollama's internal scheduler handles VRAM allocation and eviction
- KV cache quantization (`q8_0`) reduces per-model memory overhead
- `keep_alive=-1` for actively-used models, `0` for one-shot tasks

**When to use:** Default strategy. Handles the "Balanced" and "Speed-Optimized" deployment strategies from the local model survey without additional infrastructure.

**24 GB VRAM concurrent scenarios** (from [local-model-survey.md](local-model-survey.md)):

| Configuration | Models | VRAM | Headroom |
|---------------|--------|------|----------|
| Triple agent | Coder 7B + R1 7B + Llama 3B | ~12.3 GB | ~12 GB |
| Dual specialist | Coder 14B + R1 14B | ~18 GB | ~6 GB |
| Quad lightweight | Coder 7B + R1 7B + Phi-4 + Llama 3B | ~21.3 GB | ~3 GB |
| Solo quality | Coder 32B (sequential) | ~20 GB | ~4 GB |

### Multiple Ollama Instances (Separate Containers)

Run separate Ollama containers, each bound to the same GPU via `NVIDIA_VISIBLE_DEVICES=0`.

**How it works:**

- Each container gets its own port (11434, 11435, etc.)
- Each loads a dedicated model with `keep_alive=-1`
- Dispatcher routes to the correct port based on task type

**When to use:** Only if Ollama's single-instance model scheduling proves unreliable (e.g., unexpected eviction, contention between agents). Current evidence suggests the single-instance approach is sufficient.

**Trade-offs:**

| Aspect | Single Instance | Multiple Instances |
|--------|----------------|-------------------|
| VRAM overhead | ~0.5 GB (one runtime) | ~0.5 GB per instance |
| Model eviction control | Ollama manages (LRU) | Full control (dedicated) |
| Configuration complexity | One container | N containers + port mapping |
| Monitoring | One health endpoint | N health endpoints |
| VRAM fragmentation risk | Lower (shared allocator) | Higher (competing allocators) |

**Recommendation:** Start with single instance. The 0.5 GB per-instance overhead and VRAM fragmentation from multiple allocators competing for the same GPU make multi-instance wasteful on a single 24 GB card.

### Model-Per-GPU (Future, Multi-GPU)

If the user adds a second GPU, each GPU gets a dedicated Ollama instance with pinned models:

```yaml
services:
  ollama-code:
    image: ollama/ollama:latest
    environment:
      - NVIDIA_VISIBLE_DEVICES=0
      - OLLAMA_KEEP_ALIVE=-1
    ports:
      - "11434:11434"

  ollama-review:
    image: ollama/ollama:latest
    environment:
      - NVIDIA_VISIBLE_DEVICES=1
      - OLLAMA_KEEP_ALIVE=-1
    ports:
      - "11435:11434"
```

This is the natural scaling path: add GPUs, add dedicated Ollama instances, let the dispatcher route by model specialty.

## VRAM Management

### Ollama's Eviction Behavior

- **LRU eviction**: when a new model request arrives and VRAM is insufficient, the least-recently-used idle model is unloaded first
- **No partial loading**: concurrent GPU models must fit entirely in VRAM -- Ollama will not split a model across GPU and system RAM for concurrent loading
- **Spillover to RAM**: a single model can spill to system RAM if it exceeds VRAM, but this disables GPU concurrent loading for other models and severely impacts performance
- **Known issue (2025)**: Ollama may struggle to evict models when other programs share VRAM (see [ollama/ollama#9926](https://github.com/ollama/ollama/issues/9926)). Keep the GPU dedicated to Ollama to avoid this.

### Practical VRAM Budget for 24 GB

```text
Total VRAM:                    24,576 MB (24 GB)
Ollama runtime overhead:         ~500 MB
CUDA context overhead:           ~300 MB
Available for models:         ~23,776 MB (~23.2 GB)

Per-model KV cache overhead (4K context, q8_0):
  7B model:   ~200-400 MB
  14B model:  ~400-800 MB
  32B model:  ~800-1600 MB

Per-model KV cache overhead (32K context, f16):
  7B model:   ~3-5 GB (!)
  14B model:  ~5-8 GB (!)
```

**Key insight:** Context length is the hidden VRAM killer. For concurrent multi-model loading, use short contexts (4K-8K) and KV cache quantization (`OLLAMA_KV_CACHE_TYPE=q8_0`). Agent tasks (code generation, test writing) rarely need long contexts -- reserve long contexts for cloud API calls.

### Recommended VRAM Strategy

1. Set `OLLAMA_KV_CACHE_TYPE=q8_0` to halve KV cache memory
2. Use `num_ctx` in Modelfiles to limit context per model (4096 for code-gen, 2048 for dispatch)
3. Set `OLLAMA_MAX_LOADED_MODELS=3` for the balanced strategy
4. Monitor with `/api/ps` and alert when free VRAM drops below 2 GB
5. Keep the GPU dedicated to Ollama -- no other CUDA workloads

## Alternatives Comparison

### Feature Matrix

| Feature | Ollama | vLLM | llama.cpp server | LocalAI |
|---------|--------|------|-------------------|---------|
| **Multi-model concurrent** | Yes (built-in) | No (one model per instance) | No (one model per process) | Yes (multi-backend) |
| **Model management** | Built-in registry + pull | Manual (HF download) | Manual (GGUF files) | Config files per model |
| **OpenAI API compat** | Partial (chat, generate) | Full (streaming, tools) | Partial (chat, completions) | Full (drop-in replacement) |
| **Function/tool calling** | Basic (no streaming tools) | Full (parallel tools) | Basic | Full |
| **GGUF support** | Native | No (PyTorch/Safetensors) | Native | Native (via llama-cpp backend) |
| **GPU memory optimization** | Standard | PagedAttention (-50% fragmentation) | Standard | Depends on backend |
| **Concurrent throughput** | ~1-3 req/s (13B, concurrent) | ~120-160 req/s (continuous batching) | ~1-3 req/s (single model) | Depends on backend |
| **Setup complexity** | Minimal (one binary/container) | Moderate (Python, CUDA deps) | Low (single binary) | Moderate (config per model) |
| **Docker support** | Official images | Official images | Community images | Official images |
| **Model switching cost** | 5-15s (load from disk) | N/A (one model per instance) | Must restart process | Backend-dependent |
| **Windows support** | Native | Linux only (WSL2 for Windows) | Native | Docker (Linux containers) |
| **Community/ecosystem** | Largest (300k+ GitHub stars) | Growing (60k+ stars) | Large (80k+ stars) | Moderate (30k+ stars) |

### Ollama (Recommended for Samverk)

**Strengths:** Built-in multi-model scheduling, model registry with simple `pull` commands, Modelfile customization for per-agent profiles, minimal setup, and the largest community for troubleshooting. Its async model loading and eviction is a natural fit for Samverk's async work model -- the 5-15 second model load time is irrelevant when the user is not watching.

**Weaknesses:** Limited concurrent throughput per model (~1-3 req/s at 13B with concurrent requests). No PagedAttention optimization. Tool calling lacks streaming and `tool_choice` parameter. Known VRAM management issues when sharing GPU with other programs.

**Samverk fit:** Excellent. Samverk agents run sequentially or with 2-3 concurrent models. Throughput is not the bottleneck -- agent tasks take minutes, not milliseconds. Model management simplicity matters more than raw req/s.

### vLLM

**Strengths:** PagedAttention reduces memory fragmentation by 50%+, continuous batching delivers 35x+ throughput vs llama.cpp at high concurrency, full OpenAI API compatibility including parallel function calling, production-grade monitoring.

**Weaknesses:** One model per instance (must run separate processes for multi-model). No GGUF support (requires PyTorch/Safetensors format). Linux-only (WSL2 on Windows). Heavier resource footprint. More complex setup (Python environment, CUDA dependencies).

**Samverk fit:** Overkill for current needs. The throughput advantage is meaningless at 1-3 concurrent agent requests. The single-model-per-instance design requires running multiple vLLM processes, each consuming additional VRAM for runtime overhead. Consider as an upgrade path if Samverk scales to multi-user or high-throughput scenarios.

### llama.cpp Server

**Strengths:** Minimal dependencies, fastest startup, runs on any hardware (CPU, GPU, ARM), most efficient memory usage for single-model inference. GGUF native. Excellent for edge deployment.

**Weaknesses:** One model per process. No built-in model management -- must handle model files manually. Limited API compared to Ollama. No model eviction or scheduling.

**Samverk fit:** Poor as a primary runtime. Ollama is built on llama.cpp internally but adds the model management, scheduling, and API layer that Samverk needs. Using llama.cpp directly means reimplementing what Ollama already provides.

### LocalAI

**Strengths:** Universal API hub routing requests to multiple backends (llama-cpp, vLLM, transformers, stablediffusion, whisper). Full OpenAI API drop-in replacement. Multi-modal support (text, images, audio, video). gRPC plugin architecture. MCP integration (v3.10+, January 2026).

**Weaknesses:** Added complexity (another orchestration layer between Samverk and the inference engine). Configuration overhead (YAML config per model). Performance depends on backend choice. Smaller community than Ollama.

**Samverk fit:** Interesting as a long-term option if Samverk needs multi-modal capabilities (image generation for UI mockups, audio transcription for voice check-ins). For text-only code generation and review agents, it adds unnecessary indirection over Ollama.

### Recommendation Path

```text
Phase 1 (Alpha):  Ollama -- simplest setup, built-in multi-model, sufficient throughput
Phase 2 (Beta):   Ollama + evaluate vLLM for high-throughput agents if needed
Phase 3 (v1.0):   Consider LocalAI if multi-modal agent capabilities are required
```

## GPU Sharing Technologies

### Applicability to RTX 3090 Ti

| Technology | RTX 3090 Ti Support | Usefulness for Samverk |
|------------|--------------------|-----------------------|
| **MIG** | Not supported (A100/H100 only) | N/A |
| **Time-slicing** | Supported (all NVIDIA GPUs) | Low -- adds latency, Ollama handles scheduling |
| **MPS** | Supported (Linux only, compute 3.5+) | Low -- Ollama is the only CUDA consumer |
| **CUDA_VISIBLE_DEVICES** | Supported | Useful for multi-GPU only |

### MIG (Multi-Instance GPU)

MIG partitions a physical GPU into up to 7 isolated instances with dedicated compute, memory, and L2 cache. **Not available on consumer GPUs** -- requires A100, A800, H100, H200, or H800.

**Verdict:** Not applicable. RTX 3090 Ti does not support MIG.

### Time-Slicing

Time-slicing shares a GPU among multiple processes by rapidly switching between them (similar to CPU time-sharing). The NVIDIA GPU Operator (v25.3.2+) can configure time-sliced GPU replicas in Kubernetes.

**How it works:**

- Multiple containers see "virtual" GPUs (e.g., 4 slices of one physical GPU)
- Each slice gets alternating compute time
- No memory isolation -- a crash in one slice can affect others
- Adds context-switching overhead (10-30% throughput reduction)

**Verdict:** Not recommended for Samverk. Ollama's internal model scheduling already handles concurrent access to the GPU. Adding time-slicing introduces overhead and fragility with no benefit. Time-slicing is designed for multi-tenant Kubernetes clusters where different teams need isolated GPU access -- the opposite of Samverk's single-user model.

### MPS (Multi-Process Service)

CUDA MPS allows multiple processes to share a GPU context without context-switching overhead. Useful when each process individually under-utilizes the GPU.

**How it works:**

- A daemon process manages GPU context sharing
- Multiple CUDA applications submit work concurrently
- Lower overhead than time-slicing (no context switch)
- Linux-only

**Verdict:** Potentially useful if running multiple Ollama instances on the same GPU, but the single-instance approach eliminates this need entirely. MPS adds operational complexity (daemon management, Linux-only) for marginal benefit. Consider only if multi-instance Ollama becomes necessary.

### Summary

None of the GPU sharing technologies provide meaningful benefit for Samverk's single-user, single-GPU, single-Ollama-instance architecture. They are designed for multi-tenant or multi-process GPU sharing at the infrastructure level -- a problem Samverk does not have. Ollama's application-level model scheduling is the right abstraction.

## Recommended Architecture for Samverk

### Component Diagram

```text
User (phone/laptop)
    |
Claude (MCP client)
    |
Samverk Server (Go binary)
    ├── MCP endpoint (/mcp)
    ├── REST API (/api/v1/)
    ├── Dispatcher
    │   ├── Issue watcher (forge adapter)
    │   ├── Task router (model selection)
    │   └── Ollama client (preload, generate, monitor)
    └── Agent runtime
        ├── Code-gen agent → Ollama (Qwen 2.5 Coder 7B)
        ├── Test agent     → Ollama (Qwen 2.5 Coder 7B)
        ├── QC agent       → Ollama (DeepSeek R1 14B) or Cloud API
        ├── Docs agent     → Ollama (Llama 3.1 8B)
        └── Dispatch classifier → Ollama (Llama 3.2 3B)
                                    |
                             Ollama Container (Docker)
                                    |
                              RTX 3090 Ti (24 GB)
```

### Dispatcher-Ollama Integration

The Samverk dispatcher manages the Ollama model lifecycle:

1. **Task arrives** -- dispatcher determines agent type and required model
2. **Check model state** -- `GET /api/ps` to see if the model is loaded
3. **Preload if needed** -- `POST /api/chat` with empty message and `keep_alive=-1`
4. **Wait for ready** -- poll `/api/ps` until model appears (5-15 seconds)
5. **Dispatch work** -- agent sends its prompt via `/api/chat` or `/api/generate`
6. **Monitor** -- track VRAM usage, queue depth, agent completion
7. **Cleanup** -- set `keep_alive=0` for one-shot tasks to free VRAM promptly

### Model Profiles (Modelfiles)

Each agent type gets a custom Modelfile limiting context and tuning parameters:

```dockerfile
# Modelfile.codegen
FROM qwen2.5-coder:7b
PARAMETER num_ctx 4096
PARAMETER temperature 0.2
PARAMETER top_p 0.9
SYSTEM "You are a Go code generation agent. Write clean, idiomatic Go code..."
```

```dockerfile
# Modelfile.dispatch
FROM llama3.2:3b
PARAMETER num_ctx 2048
PARAMETER temperature 0.1
SYSTEM "You are a task classifier. Given an issue, determine the agent type..."
```

Lower `num_ctx` reduces KV cache memory, allowing more concurrent models.

## Implementation Considerations

### Phase 1: Minimal Viable Orchestration

1. **Ollama client in Go** -- HTTP client for `/api/chat`, `/api/generate`, `/api/ps`, `/api/pull`
2. **Model preloading** -- dispatcher preloads the next model while the current task runs
3. **Health monitoring** -- periodic `/api/ps` checks, restart container on unresponsive
4. **Docker Compose** -- single `docker-compose.yml` shipped with Samverk
5. **Configuration** -- `.samverk/models.yaml` mapping agent roles to Ollama model names

### Phase 2: Resilience

1. **Retry with backoff** -- handle Ollama 503 (queue full) and model load failures
2. **Fallback chain** -- if local model fails, route to cloud API (per ADR-007)
3. **VRAM monitoring** -- alert when free VRAM drops below threshold
4. **Model pull automation** -- `samverk setup` downloads required models on first run

### Phase 3: Optimization

1. **Speculative preloading** -- predict next model based on task pipeline (e.g., code-gen always followed by QC)
2. **Adaptive strategy selection** -- switch between "solo 32B" and "triple 7B" based on task queue depth
3. **Performance telemetry** -- track tokens/second, model load times, VRAM utilization per agent
4. **vLLM evaluation** -- benchmark vLLM vs Ollama on Samverk-specific workloads if throughput becomes a bottleneck

### Go Client Interface

```go
// OllamaClient wraps the Ollama HTTP API for agent use.
type OllamaClient interface {
    // Chat sends a chat completion request.
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

    // Generate sends a text generation request.
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)

    // Preload loads a model into VRAM without generating output.
    Preload(ctx context.Context, model string, keepAlive string) error

    // Unload removes a model from VRAM.
    Unload(ctx context.Context, model string) error

    // ListRunning returns currently loaded models and their VRAM usage.
    ListRunning(ctx context.Context) ([]RunningModel, error)

    // Pull downloads a model from the Ollama registry.
    Pull(ctx context.Context, model string) error

    // Healthy returns true if the Ollama server is responsive.
    Healthy(ctx context.Context) bool
}
```

### Known Risks

| Risk | Mitigation |
|------|-----------|
| Ollama VRAM eviction bugs with shared GPU | Keep GPU dedicated to Ollama; no gaming/rendering while agents work |
| Model load time spikes (15-30s for 32B) | Preload models; async architecture absorbs latency |
| Ollama API breaking changes | Pin Ollama Docker image version; test upgrades |
| KV cache memory explosion with long contexts | Limit `num_ctx` in Modelfiles; use `q8_0` KV cache |
| Single Ollama instance is a SPOF | Docker restart policy (`unless-stopped`); cloud API fallback |

## Sources

- [Ollama FAQ](https://docs.ollama.com/faq) -- official configuration reference
- [Ollama Docker Documentation](https://docs.ollama.com/docker) -- container deployment guide
- [How Ollama Handles Parallel Requests](https://www.glukhov.org/post/2025/05/how-ollama-handles-parallel-requests/) -- scheduling internals
- [Running Ollama in Production: Docker, Kubernetes, and Scaling](https://dasroot.net/posts/2025/12/running-ollama-production-docker-kubernetes-scaling/) -- production deployment patterns
- [Local LLM Hosting Comparison Guide](https://www.glukhov.org/post/2025/11/hosting-llms-ollama-localai-jan-lmstudio-vllm-comparison/) -- feature-by-feature tool comparison
- [vLLM vs Ollama vs llama.cpp vs TGI vs TensorRT-LLM](https://itecsonline.com/post/vllm-vs-ollama-vs-llama.cpp-vs-tgi-vs-tensort) -- performance benchmarks
- [GPU Memory Pooling and Sharing](https://introl.com/blog/gpu-memory-pooling-sharing-multi-tenant-kubernetes-2025) -- MIG/MPS/time-slicing comparison
- [NVIDIA Multi-Process Service Documentation](https://docs.nvidia.com/deploy/mps/index.html) -- MPS architecture reference
- [Running Multiple Ollama Instances](https://gist.github.com/jrknox1977/15eeb39fd71ae72cf2014a7cbeb9b2e1) -- multi-container patterns
- [Ollama VRAM Eviction Issue #9926](https://github.com/ollama/ollama/issues/9926) -- known VRAM management bug

## Related Documents

- [Local Model Survey](local-model-survey.md) -- model VRAM requirements and agent role mapping
- [Architecture](architecture.md) -- system design and agent hierarchy
- [ADR-007: Hybrid Local/Cloud Agents](decisions/ADR-007-hybrid-local-cloud.md)
- [ADR-019: Self-Hosted-First Development](decisions/ADR-019-self-hosted-first.md)
