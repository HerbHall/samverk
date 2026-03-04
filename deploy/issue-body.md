## Overview

Deploy Samverk as a systemd service on a Proxmox LXC container (Debian 12). All deployment files are prepared in `deploy/`. The deployment creates two systemd services: `samverk-serve` (HTTP server + MCP + dashboard) and `samverk-dispatch` (issue watcher + agent router).

## Prerequisites

**On your Windows machine:**

- Go 1.25+ installed
- Node.js available (VS 2022 bundled Node works)
- SSH access to Proxmox host (192.168.1.203)

**API keys ready:**

- GitHub Personal Access Token (repo scope) -- from github.com/settings/tokens
- Anthropic API key -- from console.anthropic.com
- OpenAI API key (optional) -- from platform.openai.com

**Infrastructure:**

- Proxmox host at 192.168.1.203 (Tailscale: 100.124.44.112)
- Gitea LXC 200 at 192.168.1.160:3000 (already running)
- Ollama VM 300 at 192.168.1.207:11434 (already running)

## Deployment Files (already prepared)

| File | Purpose |
|------|---------|
| `deploy/setup-lxc.sh` | Creates LXC container 201 on Proxmox host |
| `deploy/install.sh` | Installs binary + systemd services inside container |
| `deploy/samverk-serve.service` | systemd unit for HTTP server |
| `deploy/samverk-dispatch.service` | systemd unit for dispatcher |
| `deploy/samverk.env.example` | Environment variables template |
| `deploy/config/providers.yaml` | AI provider configuration |
| `deploy/config/server.yaml` | Multi-project configuration |
| `deploy/config/autonomy.yaml` | Autonomy policy defaults |
| `Makefile` targets | `cross-build`, `deploy-binary`, `deploy-config`, `deploy` |

## Step-by-Step Procedure

### Phase 1: Create the LXC Container (~5 min)

SSH into the Proxmox host and create the container:

```bash
# From your local machine:
ssh root@192.168.1.203

# On Proxmox host -- download Debian template if needed:
pveam download local debian-12-standard_12.7-1_amd64.tar.zst

# Create container (ID 201, IP 192.168.1.161):
pct create 201 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst \
  --hostname samverk \
  --memory 512 \
  --swap 256 \
  --cores 2 \
  --rootfs local-lvm:4 \
  --net0 name=eth0,bridge=vmbr0,ip=192.168.1.161/24,gw=192.168.1.1 \
  --nameserver 192.168.1.1 \
  --unprivileged 1 \
  --onboot 1 \
  --features nesting=1

pct start 201

# Wait for boot, then set up inside container:
pct exec 201 -- apt-get update -qq
pct exec 201 -- apt-get upgrade -y -qq
pct exec 201 -- apt-get install -y -qq curl
pct exec 201 -- useradd -r -s /usr/sbin/nologin -d /var/lib/samverk -m samverk
pct exec 201 -- mkdir -p /var/lib/samverk/.samverk
pct exec 201 -- chown -R samverk:samverk /var/lib/samverk
```

Verify: `pct exec 201 -- ping -c 1 192.168.1.160` (should reach Gitea).

Exit back to your local machine: `exit`

### Phase 2: Build the Binary (~2 min)

On your Windows machine in the Samverk repo:

```bash
cd d:/DevSpace/Samverk

# Cross-compile for Linux:
make cross-build

# Verify:
ls -lh bin/samverk-linux-amd64
# Should be ~14MB ELF binary
```

### Phase 3: Prepare Environment File (~3 min)

```bash
# Copy the template:
cp deploy/samverk.env.example deploy/samverk.env

# Edit with your actual keys:
# (use your editor -- VS Code, nano, vim, etc.)
```

Fill in these values in `deploy/samverk.env`:

- `GITHUB_TOKEN` -- your GitHub PAT with repo scope
- `SAMVERK_GITHUB_OWNER` -- `herbhall`
- `SAMVERK_GITHUB_REPO` -- `samverk`
- `ANTHROPIC_API_KEY` -- your Anthropic key
- `OPENAI_API_KEY` -- your OpenAI key (optional)
- `SAMVERK_AUTH_TOKEN` -- generate with `openssl rand -hex 32`

**Important:** `deploy/samverk.env` is gitignored. Never commit API keys.

### Phase 4: Deploy to Container (~2 min)

Copy everything to the LXC container:

```bash
DEPLOY_HOST=192.168.1.161

# Copy binary:
scp bin/samverk-linux-amd64 root@${DEPLOY_HOST}:/usr/local/bin/samverk

# Copy config files:
scp deploy/config/providers.yaml deploy/config/server.yaml \
    deploy/config/autonomy.yaml \
    root@${DEPLOY_HOST}:/var/lib/samverk/.samverk/

# Copy environment file (contains secrets):
scp deploy/samverk.env \
    root@${DEPLOY_HOST}:/var/lib/samverk/.samverk/samverk.env

# Copy systemd units and installer:
scp deploy/samverk-serve.service deploy/samverk-dispatch.service \
    deploy/install.sh root@${DEPLOY_HOST}:/tmp/

# Run the installer:
ssh root@${DEPLOY_HOST} bash /tmp/install.sh
```

Or use the Makefile shortcut (after env file and configs are already on the host):

```bash
make deploy DEPLOY_HOST=192.168.1.161
```

### Phase 5: Start Services and Verify (~3 min)

```bash
ssh root@192.168.1.161

# Start both services:
systemctl start samverk-serve
systemctl start samverk-dispatch

# Check status:
systemctl status samverk-serve
systemctl status samverk-dispatch

# Health check:
curl http://localhost:8080/healthz
# Expected: {"status":"ok"}

# Check logs:
journalctl -u samverk-serve --no-pager -n 20
journalctl -u samverk-dispatch --no-pager -n 20
```

### Phase 6: Create API Key for Claude Desktop (~1 min)

Still on the LXC container:

```bash
# Create an API key for Claude Desktop MCP connection:
sudo -u samverk /usr/local/bin/samverk key create \
  --name claude-desktop \
  --auth-keys /var/lib/samverk/.samverk/auth.yaml

# SAVE THE OUTPUT TOKEN -- it is only shown once.
# Format: sk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Phase 7: Verify from Your Machine (~2 min)

From your Windows machine:

```bash
# Dashboard should be accessible:
curl http://192.168.1.161:8080/healthz

# Open in browser:
# http://192.168.1.161:8080
```

Test MCP endpoint (replace TOKEN with the sk_ key from Phase 6):

```bash
curl -X POST http://192.168.1.161:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer TOKEN" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"capabilities":{}},"id":1}'
```

### Phase 8: Configure Claude Desktop (Optional, ~2 min)

Add to Claude Desktop MCP config:

```json
{
  "samverk": {
    "url": "http://192.168.1.161:8080/mcp",
    "headers": {
      "Authorization": "Bearer sk_your_key_here"
    }
  }
}
```

## Post-Deployment

### Updating

After code changes, redeploy with:

```bash
make deploy DEPLOY_HOST=192.168.1.161
ssh root@192.168.1.161 systemctl restart samverk-serve samverk-dispatch
```

### Monitoring

```bash
# Live logs:
ssh root@192.168.1.161 journalctl -u samverk-serve -f
ssh root@192.168.1.161 journalctl -u samverk-dispatch -f

# Service status:
ssh root@192.168.1.161 systemctl status samverk-serve samverk-dispatch
```

### Container Management (from Proxmox host)

```bash
pct status 201          # Check container state
pct stop 201            # Stop container
pct start 201           # Start container (services auto-start)
pct exec 201 -- bash    # Shell into container
```

## Architecture Summary

```text
Your Machine (Windows)                    Proxmox Host (192.168.1.203)
  |                                         |
  | make cross-build                        |  LXC 200 (Gitea, .160:3000)
  | scp binary + configs                    |  LXC 201 (Samverk, .161:8080)  <-- NEW
  |                                         |  VM  300 (Ollama, .207:11434)
  |                                         |
  | Browser -> http://192.168.1.161:8080    |
  | Claude Desktop -> MCP -> :8080/mcp      |
  |                                         |
  | Tailscale (100.124.44.112)              |
  | -> future: access from phone/tablet     |
```

## Rollback

If something goes wrong:

```bash
ssh root@192.168.1.161
systemctl stop samverk-serve samverk-dispatch
# Previous binary stays at /usr/local/bin/samverk until overwritten
# Database at /var/lib/samverk/.samverk/samverk.db is never deleted
# To destroy the container entirely (from Proxmox host):
# pct stop 201 && pct destroy 201
```

## Estimated Time

~15-20 minutes for a fresh deployment, including creating the LXC container.

## Checklist

- [ ] Proxmox template downloaded
- [ ] LXC container 201 created and started
- [ ] Binary cross-compiled and deployed
- [ ] Environment file filled with API keys
- [ ] Config files deployed
- [ ] systemd services installed and enabled
- [ ] samverk-serve running (healthz returns ok)
- [ ] samverk-dispatch running (logs show polling)
- [ ] Dashboard accessible at `http://192.168.1.161:8080`
- [ ] API key created for Claude Desktop
- [ ] MCP endpoint responds to initialize request
