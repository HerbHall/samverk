#!/bin/bash
# Samverk PC Agent - Docker entrypoint
# Starts the PowerShell agent loop in continuous mode.
# Forge config comes from environment variables (SAMVERK_FORGE, SAMVERK_FORGE_URL,
# SAMVERK_FORGE_PROJECT) -- see forge.psm1 Get-ForgeConfig.

set -euo pipefail

echo "[pc-agent] Starting Samverk PC Agent on $(hostname)"
echo "[pc-agent] Server: ${SAMVERK_SERVER_URL:-not set}"
echo "[pc-agent] Forge:  ${SAMVERK_FORGE:-not set} / ${SAMVERK_FORGE_PROJECT:-not set}"

# Accept Claude TOS non-interactively
mkdir -p /root/.claude
echo '{"hasCompletedOnboarding":true,"hasAcknowledgedDisclaimer":true}' > /root/.claude/settings.json

# Set up bare repo for worktree-based workflow
if [ ! -d "/workspace/samverk.git" ]; then
    echo "[pc-agent] Creating bare repo for worktree workflow..."
    git clone --bare "${GITEA_REPO_URL:-http://192.168.1.160:3000/samverk/samverk.git}" /workspace/samverk.git
fi

# Patch Windows-specific paths in agent scripts for Linux container.
# The modules default to D:\bots which doesn't exist in Docker.
sed -i "s|Root         = 'D:\\\\bots'|Root         = '/workspace'|g" /agent/scripts/pc-agent/workspace.psm1
sed -i "s|'D:\\\\bots'|'/workspace'|g" /agent/scripts/pc-agent/registration.psm1

# Run the agent loop
exec pwsh -NoProfile -File /agent/scripts/pc-agent/agent-loop.ps1 \
    -Continuous \
    -InitWorkspace
