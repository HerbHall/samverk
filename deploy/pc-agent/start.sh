#!/bin/bash
# Samverk PC Agent - Docker entrypoint
# Starts the PowerShell agent loop in continuous mode.
# Forge config comes from environment variables (SAMVERK_FORGE, SAMVERK_FORGE_URL,
# SAMVERK_FORGE_PROJECT) -- see forge.psm1 Get-ForgeConfig.

set -euo pipefail

echo "[pc-agent] Starting Samverk PC Agent on $(hostname)"
echo "[pc-agent] Server: ${SAMVERK_SERVER_URL:-not set}"
echo "[pc-agent] Forge:  ${SAMVERK_FORGE:-not set} / ${SAMVERK_FORGE_PROJECT:-not set}"

# Claude CLI uses OAuth (Max plan), not API keys.
# The claude-auth volume persists /root/.claude/ across restarts.
# First-time setup: docker exec -it samverk-pc-agent claude login
# This writes OAuth credentials to /root/.claude/ (persisted in volume).
if [ ! -f /root/.claude/.credentials.json ] && [ ! -d /root/.claude/auth ]; then
    echo "[pc-agent] WARNING: No Claude OAuth credentials found."
    echo "[pc-agent] Run: docker exec -it samverk-pc-agent claude login"
    echo "[pc-agent] Continuing without Claude -- agent will register but cannot execute tasks."
fi

# Ensure TOS flags exist (non-destructive -- preserves OAuth creds).
if [ ! -f /root/.claude/settings.json ]; then
    echo '{"hasCompletedOnboarding":true,"hasAcknowledgedDisclaimer":true}' > /root/.claude/settings.json
fi

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
