# Ollama VM Configuration

GPU-accelerated Ollama instance on Proxmox for Samverk local AI agent inference.

## Infrastructure

| Component | Value |
|-----------|-------|
| Proxmox Host | 192.168.1.203 (Tailscale: 100.124.44.112) |
| VM ID | 300 (QEMU/KVM) |
| VM IP | 192.168.1.207 |
| OS | Ubuntu 24.04.4 LTS |
| CPU | 8 cores (host passthrough) |
| RAM | 32 GB |
| Disk | 64 GB (scsi0, virtio-scsi-single) |
| GPU | NVIDIA RTX 3090 Ti (VFIO passthrough) |
| NVIDIA Driver | 590.48.01 |
| CUDA | 13.1 |
| Ollama Version | 0.17.4 |
| Ollama URL | `http://192.168.1.207:11434` |
| Auto-start | Yes (onboot=1) |

## Access

| Setting | Value |
|---------|-------|
| SSH User | herb |
| SSH Auth | Key-based (same key as Proxmox host) |
| Ollama API | `http://192.168.1.207:11434` (LAN-accessible, bound to 0.0.0.0) |

## GPU Passthrough Setup

The RTX 3090 Ti is passed through from the Proxmox host to VM 300 via VFIO.

### Host Kernel Parameters

```text
# /etc/default/grub (GRUB_CMDLINE_LINUX_DEFAULT)
intel_iommu=on iommu=pt
```

**Note:** The Proxmox installer may inject `noapic` in `/etc/default/grub.d/installer.cfg` which blocks IOMMU. Remove it if present. VT-d must also be enabled in BIOS.

### Host VFIO Configuration

```text
# /etc/modprobe.d/vfio.conf
options vfio-pci ids=10de:2203,10de:1aef

# /etc/modprobe.d/blacklist-nvidia.conf
blacklist nvidia
blacklist nouveau
blacklist snd_hda_intel

# /etc/modules
vfio
vfio_iommu_type1
vfio_pci
```

Device IDs:

- `10de:2203` -- RTX 3090 Ti GPU
- `10de:1aef` -- RTX 3090 Ti Audio

### VM Hardware Configuration

```bash
qm set 300 --hostpci0 0000:01:00,pcie=1,x-vga=1
```

The GPU occupies IOMMU group containing both the GPU and its audio device.

## Installed Models

| Model | Size | Quantization | Use Case |
|-------|------|-------------|----------|
| qwen2.5-coder:7b | ~4.7 GB | Q4_K_M | Code generation, tests |
| deepseek-r1:7b | ~4.7 GB | Q4_K_M | QC, reasoning |
| llama3.2:3b | ~2 GB | Q4_K_M | Dispatch, classification |

Total: ~11.4 GB, leaving ~12 GB headroom on 24 GB VRAM.

### Future Models

| Model | Size | Use Case |
|-------|------|----------|
| qwen2.5-coder:14b | ~9 GB | Higher-quality code generation (replaces 7b when solo) |

## Performance

Measured with `llama3.2:3b` on RTX 3090 Ti:

| Metric | Value |
|--------|-------|
| Eval rate | 295.86 tokens/s |
| First model load | ~37 seconds (cold start from disk) |
| Subsequent loads | ~5-15 seconds (cached) |

## Ollama Configuration

Ollama runs as a systemd service with a drop-in override for LAN access and tuning:

```text
# /etc/systemd/system/ollama.service.d/override.conf
[Service]
Environment="OLLAMA_HOST=0.0.0.0"
Environment="OLLAMA_MAX_LOADED_MODELS=3"
Environment="OLLAMA_NUM_PARALLEL=2"
Environment="OLLAMA_KEEP_ALIVE=10m"
Environment="OLLAMA_KV_CACHE_TYPE=q8_0"
```

## API Usage

```bash
# List available models
curl http://192.168.1.207:11434/api/tags

# List running models and VRAM usage
curl http://192.168.1.207:11434/api/ps

# Chat completion
curl http://192.168.1.207:11434/api/chat -d '{
  "model": "llama3.2:3b",
  "messages": [{"role": "user", "content": "Hello"}]
}'

# Pull a new model
curl http://192.168.1.207:11434/api/pull -d '{"name": "qwen2.5-coder:7b"}'

# Health check
curl http://192.168.1.207:11434/
```

## VM Management

```bash
# Start/stop from Proxmox host
qm start 300
qm stop 300

# SSH into VM
ssh herb@192.168.1.207

# Check Ollama service
ssh herb@192.168.1.207 systemctl status ollama

# View Ollama logs
ssh herb@192.168.1.207 journalctl -u ollama -f

# Check GPU status inside VM
ssh herb@192.168.1.207 nvidia-smi
```

## Troubleshooting

### No IOMMU Groups After Enabling intel_iommu

Check for `noapic` in Proxmox drop-in configs:

```bash
cat /etc/default/grub.d/installer.cfg
```

Remove `noapic` if present, then `update-grub && reboot`.

### VFIO Probe Error -22

Ensure VT-d is enabled in BIOS and IOMMU groups are populated:

```bash
find /sys/kernel/iommu_groups/ -type l | head -20
```

### Ollama Not Accessible on LAN

Verify the override is active:

```bash
systemctl cat ollama | grep OLLAMA_HOST
ss -tlnp | grep 11434
```

Should show `0.0.0.0:11434`, not `127.0.0.1:11434`.

## References

- [Ollama Container Orchestration](ollama-orchestration.md)
- [System Requirements](system-requirements.md)
- [Local Model Survey](local-model-survey.md)
- [ADR-007: Hybrid Local/Cloud Agents](decisions/ADR-007-hybrid-local-cloud.md)
- [ADR-019: Self-Hosted-First Development](decisions/ADR-019-self-hosted-first.md)
- [Gitea Setup](gitea-setup.md)
