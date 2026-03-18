# Scripts

Operational scripts for Samverk infrastructure.

## Cloudflared Watchdog

Monitors tunnel health every 60s. Restarts cloudflared after 2 consecutive failures.

### Install on CT 201 (Caddy/tunnel host)

````bash
sudo cp scripts/cloudflared-watchdog.sh /usr/local/bin/
sudo chmod +x /usr/local/bin/cloudflared-watchdog.sh
sudo cp scripts/cloudflared-watchdog.service /etc/systemd/system/
sudo cp scripts/cloudflared-watchdog.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cloudflared-watchdog.timer
````

### Configuration

Environment variables (set in a systemd override or `/etc/default/cloudflared-watchdog`):

| Variable | Default | Description |
|----------|---------|-------------|
| `CLOUDFLARED_HEALTH_URL` | `http://localhost:8080/healthz` | Health endpoint to check |
| `CLOUDFLARED_SERVICE` | `cloudflared` | Systemd service name to restart |
| `CLOUDFLARED_MAX_FAILURES` | `2` | Consecutive failures before restart |

### How it works

1. Timer fires every 60s (first run 120s after boot)
2. Script checks if cloudflared systemd service is active
3. Script hits the health endpoint through the tunnel
4. If both pass, resets the failure counter
5. If either fails, increments counter
6. After 2 consecutive failures, restarts the cloudflared service
