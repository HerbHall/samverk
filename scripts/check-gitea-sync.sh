#!/bin/bash
# check-gitea-sync.sh -- Refuse to deploy if local tree diverges from Gitea main.
#
# Prevents the class of bug where local working tree has stale code that
# doesn't match what Gitea considers the source of truth (main branch).
#
# Checks:
#   1. Working tree must be clean (no uncommitted changes)
#   2. Local HEAD must match the remote main branch tip
#
# Usage: bash scripts/check-gitea-sync.sh [remote_name]
#   remote_name: git remote pointing at Gitea (default: auto-detect)
#
# Override remote: GITEA_REMOTE=origin bash scripts/check-gitea-sync.sh
# Skip check:     DEPLOY_SKIP_SYNC_CHECK=1 make redeploy

set -euo pipefail

# Allow skipping for emergency hotfixes (must be explicit).
if [ "${DEPLOY_SKIP_SYNC_CHECK:-0}" = "1" ]; then
    echo "WARNING: DEPLOY_SKIP_SYNC_CHECK=1 -- skipping Gitea sync check."
    echo "         You are deploying code that may not match Gitea main."
    exit 0
fi

# Auto-detect the Gitea remote. Prefer "gitea", fall back to "origin".
detect_remote() {
    if [ -n "${GITEA_REMOTE:-}" ]; then
        echo "$GITEA_REMOTE"
        return
    fi
    if [ -n "${1:-}" ]; then
        echo "$1"
        return
    fi
    if git remote | grep -qx "gitea"; then
        echo "gitea"
    else
        echo "origin"
    fi
}

REMOTE=$(detect_remote "${1:-}")
BRANCH="main"

echo "=== Pre-deploy: checking sync with ${REMOTE}/${BRANCH} ==="

# Check 1: Working tree must be clean.
if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
    echo ""
    echo "ERROR: Working tree has uncommitted changes."
    echo ""
    echo "  Deploying a dirty tree means the binary won't match any commit."
    echo "  Commit or stash your changes first:"
    echo ""
    echo "    git stash        # stash changes"
    echo "    make redeploy    # deploy clean tree"
    echo "    git stash pop    # restore changes"
    echo ""
    exit 1
fi

# Also check for untracked .go or web/ files that would be compiled.
untracked=$(git ls-files --others --exclude-standard -- '*.go' 'web/' 2>/dev/null || true)
if [ -n "$untracked" ]; then
    echo ""
    echo "ERROR: Untracked files would be included in the build:"
    echo "$untracked" | head -10
    echo ""
    echo "  These files exist locally but not in Gitea. The deployed binary"
    echo "  would include code that isn't in the source of truth."
    echo ""
    echo "  Either commit them, delete them, or add them to .gitignore."
    echo ""
    exit 1
fi

# Check 2: Fetch latest from Gitea.
echo "--- Fetching ${REMOTE}/${BRANCH}..."
if ! git fetch "$REMOTE" "$BRANCH" 2>/dev/null; then
    echo ""
    echo "ERROR: Could not fetch ${REMOTE}/${BRANCH}."
    echo ""
    echo "  Check your network connection and remote configuration:"
    echo "    git remote -v"
    echo ""
    exit 1
fi

# Check 3: Local HEAD must match remote main.
LOCAL_HEAD=$(git rev-parse HEAD)
REMOTE_HEAD=$(git rev-parse "${REMOTE}/${BRANCH}")

if [ "$LOCAL_HEAD" != "$REMOTE_HEAD" ]; then
    echo ""
    echo "ERROR: Local HEAD does not match ${REMOTE}/${BRANCH}."
    echo ""
    echo "  Local:  ${LOCAL_HEAD}"
    echo "  Remote: ${REMOTE_HEAD}"
    echo ""
    # Show which direction the divergence goes.
    ahead=$(git rev-list --count "${REMOTE}/${BRANCH}..HEAD" 2>/dev/null || echo "?")
    behind=$(git rev-list --count "HEAD..${REMOTE}/${BRANCH}" 2>/dev/null || echo "?")
    echo "  Local is ${ahead} commit(s) ahead, ${behind} commit(s) behind ${REMOTE}/${BRANCH}."
    echo ""
    echo "  To sync with Gitea and deploy:"
    echo ""
    echo "    git pull ${REMOTE} ${BRANCH}    # or: git reset --hard ${REMOTE}/${BRANCH}"
    echo "    make redeploy"
    echo ""
    exit 1
fi

echo "    Local HEAD matches ${REMOTE}/${BRANCH} (${LOCAL_HEAD:0:8}). OK to deploy."
echo ""
