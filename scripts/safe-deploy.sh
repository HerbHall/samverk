#!/bin/bash
# safe-deploy.sh -- Deploy Samverk with idle-wait safety gate.
#
# Waits for the dispatcher to become idle (no active workers, empty queue)
# before stopping services and deploying the new binary.
#
# Usage: bash scripts/safe-deploy.sh [host] [timeout_seconds]
#   host:            deploy target (default: 192.168.1.162)
#   timeout_seconds: max seconds to wait for idle (default: 600 = 10 min)

set -euo pipefail

HOST="${1:-192.168.1.162}"
TIMEOUT="${2:-600}"
HEALTHZ="http://localhost:8080/healthz"
METRICS="http://localhost:8080/api/v1/metrics"

echo "=== Safe Deploy to ${HOST} ==="

# Step 1: Check current state before deploying.
echo "--- Checking dispatcher state..."
AUTH_TOKEN=$(ssh "root@${HOST}" 'grep SAMVERK_AUTH_TOKEN /var/lib/samverk/.samverk/samverk.env 2>/dev/null | cut -d= -f2 | tr -d "\""' || echo "")

get_metrics() {
    if [ -n "$AUTH_TOKEN" ]; then
        ssh "root@${HOST}" "curl -sf -H 'Authorization: Bearer ${AUTH_TOKEN}' '${METRICS}'" 2>/dev/null || echo "{}"
    else
        ssh "root@${HOST}" "curl -sf '${METRICS}'" 2>/dev/null || echo "{}"
    fi
}

# Extract active workers and queue depth from metrics JSON.
check_idle() {
    local metrics
    metrics=$(get_metrics)
    local active queue
    active=$(echo "$metrics" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pool',{}).get('active_workers',0) if d.get('pool') else 0)" 2>/dev/null || echo "0")
    queue=$(echo "$metrics" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pool',{}).get('queue_depth',0) if d.get('pool') else 0)" 2>/dev/null || echo "0")
    echo "${active}:${queue}"
}

state=$(check_idle)
active="${state%%:*}"
queue="${state##*:}"

if [ "$active" -eq 0 ] && [ "$queue" -eq 0 ]; then
    echo "    Dispatcher idle (0 active, 0 queued). Proceeding."
else
    echo "    Dispatcher busy: ${active} active workers, ${queue} queued tasks."
    echo "--- Pausing dispatcher to drain (stopping new task pickup)..."

    # Stop the dispatch service first so no new tasks are claimed.
    ssh "root@${HOST}" 'systemctl stop samverk-dispatch 2>/dev/null || true'
    echo "    Dispatch service stopped. Waiting for in-flight tasks to complete..."

    # Wait for active workers to reach 0.
    elapsed=0
    while [ "$elapsed" -lt "$TIMEOUT" ]; do
        state=$(check_idle)
        active="${state%%:*}"
        if [ "$active" -eq 0 ]; then
            echo "    All workers idle after ${elapsed}s. Proceeding."
            break
        fi
        echo "    Still ${active} active worker(s)... (${elapsed}s / ${TIMEOUT}s)"
        sleep 10
        elapsed=$((elapsed + 10))
    done

    if [ "$active" -ne 0 ]; then
        echo "WARNING: Timed out after ${TIMEOUT}s with ${active} active worker(s)."
        echo "Proceeding anyway -- tasks will be interrupted."
    fi
fi

# Step 2: Stop remaining services.
echo "--- Stopping services..."
ssh "root@${HOST}" 'systemctl stop samverk-dispatch samverk-serve 2>/dev/null || true'

# Step 3: Deploy binary.
echo "--- Deploying binary..."
# Kill any orphaned processes holding the binary open (survive systemctl stop).
ssh "root@${HOST}" 'fuser -k /usr/local/bin/samverk 2>/dev/null || true'
sleep 1
scp "bin/samverk-linux-amd64" "root@${HOST}:/usr/local/bin/samverk"

# Step 4: Deploy configs and run installer.
echo "--- Deploying configs..."
scp deploy/samverk-serve.service deploy/samverk-dispatch.service "root@${HOST}:/tmp/"
scp deploy/install.sh "root@${HOST}:/tmp/install.sh"
ssh "root@${HOST}" 'sed -i "s/\r$//" /tmp/install.sh && bash /tmp/install.sh'

# Step 5: Start services.
echo "--- Starting services..."
ssh "root@${HOST}" 'systemctl start samverk-serve samverk-dispatch'

# Step 6: Verify health.
echo "--- Verifying health..."
sleep 3
if ssh "root@${HOST}" "curl -sf '${HEALTHZ}'" > /dev/null 2>&1; then
    echo "=== Deploy successful. Health check OK. ==="
else
    echo "=== WARNING: Health check failed! ==="
    echo "Check logs: ssh root@${HOST} journalctl -u samverk-serve -n 20"
    exit 1
fi
