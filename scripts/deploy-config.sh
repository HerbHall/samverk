#!/bin/bash
# deploy-config.sh -- Sync samverk config files to target without rebuilding the binary.
#
# Copies .samverk/providers.yaml and .samverk/project.yaml to the target host
# and restarts the dispatcher if either file changed.
#
# Usage: bash scripts/deploy-config.sh [host]
#   host: deploy target (default: 192.168.1.162)

set -euo pipefail

HOST="${1:-192.168.1.162}"
CONFIG_DIR="/var/lib/samverk/.samverk"

echo "=== Config Deploy to ${HOST} ==="

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

sync_config ".samverk/providers.yaml"
sync_config ".samverk/project.yaml"

if [ "$CONFIG_CHANGED" -eq 1 ]; then
    echo "--- Config changed: restarting dispatcher..."
    ssh "root@${HOST}" 'systemctl restart samverk-dispatch'
    echo "    Dispatcher restarted."
    echo "=== Config deploy complete. ==="
else
    echo "=== Config deploy complete. No changes. ==="
fi
