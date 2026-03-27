# Gitea Actions Runner Fleet

CI/CD runner infrastructure for Gitea Actions on `gitea.herbhall.net` (CT 200).

## Fleet Overview

| Runner | Machine | IP | CPU | RAM | Docker | Capacity | Auto-Start |
|--------|---------|-----|-----|-----|--------|----------|------------|
| samverk-runner | CT 200 (LXC) | 192.168.1.160 | 4 cores | 8 GB | Yes (docker.io) | 3 | systemd (gitea-runner.service) |
| hdh-nzxt-win | HDH-NZXT (Windows) | 192.168.1.202 | 20 cores | 62 GB | Yes (Docker Desktop) | 4 | Task Scheduler (at logon) |
| unraid-runner | HDH-UNRAID | 192.168.1.215 | 8 cores | 31 GB | Yes (native) | 4 | Docker container (always restart) |
| hdh-d10u-runner | HDH-D10U (Ubuntu) | 192.168.1.165 | 4 cores | 15 GB | **No** (needs install) | 2 | Manual (nohup) |

**Total capacity:** 13 concurrent CI jobs (11 with Docker, 2 host-only).

All runners use `act_runner v0.3.0` and connect to `http://192.168.1.160:3000` (Gitea internal).

## Runner Configuration

### CT 200 (samverk-runner)

- **Config:** `/var/lib/gitea-runner/config.yaml`
- **Registration:** `/var/lib/gitea-runner/.runner`
- **Service:** `systemctl status gitea-runner`
- **Labels:** `ubuntu-latest`, `ubuntu-22.04` (Docker mode: `node:20-bullseye`)
- **Docker:** `docker.io` from Debian apt (installed 2026-03-27)
- **Notes:** Shares host with Gitea. Capacity limited to 3 to avoid resource contention.

### HDH-NZXT (hdh-nzxt-win)

- **Binary:** `C:\Temp\act_runner\act_runner.exe`
- **Config:** `C:\Temp\act_runner\config.yaml`
- **Registration:** `C:\Temp\act_runner\.runner`
- **Auto-start:** Windows Task Scheduler, task "Gitea Actions Runner" (at logon)
- **Startup script:** `C:\Temp\act_runner\start-runner.bat` (checks for duplicate)
- **Log:** `C:\Temp\act_runner\runner.log`
- **Labels:** `ubuntu-latest`, `ubuntu-22.04` (Docker mode: `node:20-bullseye`)
- **Docker:** Docker Desktop v29.2.1
- **Notes:** Also runs native Ollama (port 11434) and Docker Ollama (port 11435) for AI inference. CI uses CPU/RAM only; no GPU contention.

### HDH-UNRAID (unraid-runner)

- **Container:** `act_runner` (image: `gitea/act_runner:latest`)
- **Data volume:** `/mnt/user/appdata/act_runner:/data`
- **Docker socket:** `/var/run/docker.sock:/var/run/docker.sock`
- **Config:** `/data/config.yaml` (inside container)
- **Labels:** `ubuntu-latest`, `ubuntu-22.04` (Docker mode: `node:20-bullseye`)
- **Notes:** Docker-in-Docker via socket mount. Auto-restarts with container policy.

### HDH-D10U (hdh-d10u-runner)

- **Binary:** `/home/herbh/.local/bin/act_runner`
- **Config:** `/home/herbh/gitea-runner/config.yaml`
- **Registration:** `/home/herbh/gitea-runner/.runner` (ID 6)
- **Labels:** `ubuntu-latest` (host mode -- `docker_host: "-"`)
- **Docker:** **Not installed.** JS-based GitHub Actions (checkout, setup-go, cache) fail with OCI runtime errors.
- **Action required:** Install Docker to fix CI failures:

```bash
ssh herbh@192.168.1.165
sudo apt install -y docker.io
sudo usermod -aG docker herbh
# Log out and back in for group change
sed -i 's/docker_host: "-"/docker_host: ""/' ~/gitea-runner/config.yaml
kill $(pgrep act_runner)
nohup ~/.local/bin/act_runner daemon --config ~/gitea-runner/config.yaml &
```

## Monitoring

### Runner Health Endpoint

A lightweight HTTP health server runs on CT 200 (port 9090) and exposes runner status:

```bash
curl http://192.168.1.160:9090/runners
```

Returns JSON array with each runner's `id`, `name`, `version`, `status` (online/offline), and `seconds_since_online`.

**Service:** `runner-health.service` (systemd, auto-start)

- Script: `/usr/local/bin/runner-health.sh` (queries Gitea SQLite DB)
- Server: `/usr/local/bin/runner-health-server.py` (Python HTTP on port 9090)
- Stale threshold: 300 seconds (5 minutes)

### Infra Probe Integration

The Samverk infra probe (nightly at 3 AM) includes a `gitea-runners` endpoint that:

- Queries `http://192.168.1.160:9090/runners`
- Reports total/online/offline counts
- Logs `WARN` for each offline runner with structured fields
- Syncs to Synapset machine memories for cross-session awareness

### Manual Fleet Check

```bash
# Quick status from any machine with access to CT 200
curl -s http://192.168.1.160:9090/runners | python3 -m json.tool

# From Gitea DB directly (on CT 200)
sqlite3 /var/lib/gitea/data/gitea.db \
  "SELECT id, name, version FROM action_runner WHERE deleted IS NULL OR deleted = ''"
```

## Troubleshooting

### Runner shows offline

1. Check if the process is running on the host machine
2. Check network connectivity to Gitea (`curl http://192.168.1.160:3000/api/v1/version`)
3. Check runner logs for registration errors
4. Restart the runner service/process

### CI jobs fail with OCI runtime errors

**Cause:** Runner in host mode (`docker_host: "-"`) cannot execute JS-based GitHub Actions (checkout, setup-go, cache). These actions require a container runtime.

**Fix:** Install Docker and switch to Docker mode (`docker_host: ""`).

### Stale CI status shows false failures

**Cause:** Gitea's combined commit status API returns ALL statuses ever posted for a SHA, including old "failure" entries from previous CI runs.

**Fix:** PR #369 added deduplication by context name in `GetPRChecks()`, keeping only the most recent status per check.

### Runner doesn't restart after reboot

| Machine | Fix |
|---------|-----|
| CT 200 | `systemctl enable gitea-runner` (already enabled) |
| HDH-NZXT | Check Task Scheduler: "Gitea Actions Runner" task exists and is enabled |
| Unraid | Docker container restart policy handles this |
| HDH-D10U | No auto-start configured. Add systemd service or cron `@reboot` entry. |

## Capacity Planning

Current fleet: **13 concurrent job slots** across 4 machines.

Each CI run for samverk uses 4 parallel jobs (Lint, Build, Markdown Lint in `gitea-ci.yml` + govulncheck, Trivy in `security.yml`). Test runs sequentially after Build.

- **Single PR:** ~5 job slots for ~3 minutes
- **3 concurrent PRs:** ~15 job slots needed (exceeds capacity by 2 -- jobs queue)
- **Pipeline burst (dispatcher re-queues 10 issues):** Jobs queue; ~3 rounds of 13 to clear

To add more capacity: register a new runner on any Docker-capable machine with `act_runner register` pointing at `http://192.168.1.160:3000`.
