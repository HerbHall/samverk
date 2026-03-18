#!/bin/bash
# Cloudflared tunnel health watchdog.
# Checks if the tunnel is responsive and restarts cloudflared if not.
# Intended to run as a systemd timer every 60 seconds.

set -euo pipefail

HEALTH_URL="${CLOUDFLARED_HEALTH_URL:-http://localhost:8080/healthz}"
TUNNEL_SERVICE="${CLOUDFLARED_SERVICE:-cloudflared}"
MAX_FAILURES="${CLOUDFLARED_MAX_FAILURES:-2}"
FAILURE_FILE="/tmp/cloudflared-watchdog-failures"

# Initialize failure counter
if [[ ! -f "$FAILURE_FILE" ]]; then
    echo 0 > "$FAILURE_FILE"
fi

# Check tunnel health by hitting the backend through the tunnel's local metrics endpoint
# If cloudflared is running but the QUIC connection is dead, the tunnel won't proxy
# So we check both: is the process alive AND can it reach the backend
if systemctl is-active --quiet "$TUNNEL_SERVICE" && \
   curl -sf --max-time 10 "$HEALTH_URL" > /dev/null 2>&1; then
    # Healthy -- reset failure counter
    echo 0 > "$FAILURE_FILE"
    exit 0
fi

# Increment failure counter
FAILURES=$(cat "$FAILURE_FILE")
FAILURES=$((FAILURES + 1))
echo "$FAILURES" > "$FAILURE_FILE"

if [[ "$FAILURES" -ge "$MAX_FAILURES" ]]; then
    echo "cloudflared-watchdog: $FAILURES consecutive failures, restarting $TUNNEL_SERVICE"
    systemctl restart "$TUNNEL_SERVICE"
    echo 0 > "$FAILURE_FILE"
else
    echo "cloudflared-watchdog: failure $FAILURES/$MAX_FAILURES (will restart at $MAX_FAILURES)"
fi
