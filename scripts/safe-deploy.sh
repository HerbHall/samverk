#!/bin/bash
# safe-deploy.sh -- Deploy Samverk with drain-mode safety gate.
#
# Uses the dispatcher's drain API (port 9090) to atomically prevent new
# work from being claimed, then waits for in-flight tasks to complete
# before stopping services and deploying the new binary.
#
# This eliminates three compounding failures in the old approach:
#   1. Stale metrics: drain API reads live atomic counters, not SQLite snapshots
#   2. Between-task gap: drain mode prevents new claims, so active count only decreases
#   3. TOCTOU race: drain is atomic -- once set, no new work is claimed
#
# Usage: bash scripts/safe-deploy.sh [host] [timeout_seconds]
#   host:            deploy target (default: 192.168.1.162)
#   timeout_seconds: max seconds to wait for drain (default: 900 = 15 min)

set -euo pipefail

HOST="${1:-192.168.1.162}"
TIMEOUT="${2:-900}"
HEALTHZ="http://localhost:8080/healthz"
DRAIN_API="http://localhost:9090/api/v1/dispatcher/drain"

echo "=== Safe Deploy to ${HOST} ==="

# Step 1: Enter drain mode atomically -- no new claims after this returns.
echo "--- Entering drain mode..."
drain_response=$(ssh "root@${HOST}" "curl -sf -X POST '${DRAIN_API}'" 2>/dev/null || echo "")

if [ -z "$drain_response" ]; then
    echo "    WARNING: Could not reach drain API (port 9090). Dispatcher may not be running."
    echo "    Falling back to direct stop."
    ssh "root@${HOST}" 'systemctl stop samverk-dispatch samverk-serve 2>/dev/null || true'
else
    active=$(echo "$drain_response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('active_workers',0))" 2>/dev/null || echo "0")
    claimed=$(echo "$drain_response" | python3 -c "import sys,json; print(','.join(str(i) for i in json.load(sys.stdin).get('claimed_issues',[])))" 2>/dev/null || echo "")

    if [ "$active" -eq 0 ]; then
        echo "    Dispatcher idle (0 active). Drain mode set. Proceeding immediately."
    else
        echo "    Drain mode activated. ${active} active worker(s), issues: [${claimed}]"
        echo "--- Waiting for in-flight tasks to complete..."

        # Step 2: Poll live drain status until active workers reach 0.
        elapsed=0
        while [ "$elapsed" -lt "$TIMEOUT" ]; do
            sleep 10
            elapsed=$((elapsed + 10))
            state=$(ssh "root@${HOST}" "curl -sf '${DRAIN_API}'" 2>/dev/null || echo "{}")
            active=$(echo "$state" | python3 -c "import sys,json; print(json.load(sys.stdin).get('active_workers',0))" 2>/dev/null || echo "0")
            claimed=$(echo "$state" | python3 -c "import sys,json; print(','.join(str(i) for i in json.load(sys.stdin).get('claimed_issues',[])))" 2>/dev/null || echo "")
            if [ "$active" -eq 0 ]; then
                echo "    All workers idle after ${elapsed}s. Proceeding."
                break
            fi
            echo "    Waiting: ${active} active (issues: [${claimed}]) (${elapsed}s / ${TIMEOUT}s)"
        done

        if [ "$active" -ne 0 ]; then
            echo ""
            echo "WARNING: Timed out after ${TIMEOUT}s with ${active} active worker(s)."
            echo "Active issues: [${claimed}]"
            echo ""
            read -r -p "Force deploy? Active tasks will be orphaned and re-queued on restart. (y/n) " confirm
            if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
                echo "Deploy aborted. Cancelling drain mode..."
                ssh "root@${HOST}" "curl -sf -X DELETE '${DRAIN_API}'" > /dev/null 2>&1 || true
                echo "Drain mode cancelled. Dispatcher is accepting work again."
                exit 1
            fi
            echo "    Proceeding with force deploy (orphan recovery will re-queue on restart)."
        fi
    fi

    # Step 3: Stop services. Drain mode prevents TOCTOU race -- no new
    # work can be claimed between the idle check and the stop command.
    echo "--- Stopping services..."
    ssh "root@${HOST}" 'systemctl stop samverk-dispatch samverk-serve 2>/dev/null || true'
fi

# Step 4: Deploy binary.
echo "--- Deploying binary..."
# Kill any orphaned processes holding the binary open (survive systemctl stop).
ssh "root@${HOST}" 'fuser -k /usr/local/bin/samverk 2>/dev/null || true'
sleep 1
scp "bin/samverk-linux-amd64" "root@${HOST}:/usr/local/bin/samverk"

# Step 5: Deploy configs and run installer.
echo "--- Deploying configs..."
scp deploy/samverk-serve.service deploy/samverk-dispatch.service "root@${HOST}:/tmp/"
scp deploy/install.sh "root@${HOST}:/tmp/install.sh"
ssh "root@${HOST}" 'sed -i "s/\r$//" /tmp/install.sh && bash /tmp/install.sh'

# Step 5b: Sync samverk config files and restart dispatcher if changed.
CONFIG_DIR="/var/lib/samverk/.samverk"
CONFIG_CHANGED=0

sync_config() {
    local src="$1"
    local filename
    filename=$(basename "$src")
    local dest="${CONFIG_DIR}/${filename}"

    if [ ! -f "$src" ]; then
        echo "    Skipping ${filename} (not found in repo)"
        return
    fi

    local remote_hash
    remote_hash=$(ssh "root@${HOST}" "md5sum '${dest}' 2>/dev/null | cut -d' ' -f1" || echo "")
    local local_hash
    local_hash=$(md5sum "$src" | cut -d' ' -f1)

    if [ "$local_hash" = "$remote_hash" ]; then
        echo "    ${filename}: unchanged"
    else
        ssh "root@${HOST}" "mkdir -p '${CONFIG_DIR}'"
        scp "$src" "root@${HOST}:${dest}"
        echo "    ${filename}: updated"
        CONFIG_CHANGED=1
    fi
}

echo "--- Syncing config files..."
sync_config ".samverk/providers.yaml"
sync_config ".samverk/project.yaml"

# Step 6: Start services. Drain mode is cleared on startup (clean state).
echo "--- Starting services..."
ssh "root@${HOST}" 'systemctl start samverk-serve samverk-dispatch'

# Step 7: Verify health.
echo "--- Verifying health..."
sleep 3
if ssh "root@${HOST}" "curl -sf '${HEALTHZ}'" > /dev/null 2>&1; then
    echo "=== Deploy successful. Health check OK. ==="
else
    echo "=== WARNING: Health check failed! ==="
    echo "Check logs: ssh root@${HOST} journalctl -u samverk-serve -n 20"
    exit 1
fi

if [ "$CONFIG_CHANGED" -eq 1 ]; then
    echo "--- Config changed: restarting dispatcher..."
    ssh "root@${HOST}" 'systemctl restart samverk-dispatch'
    echo "    Dispatcher restarted."
fi
