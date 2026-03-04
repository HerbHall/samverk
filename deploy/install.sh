#!/bin/bash
# install.sh -- Installs Samverk binary and systemd services.
# Run this inside the LXC container as root.
#
# Prerequisites:
#   - Binary at /usr/local/bin/samverk (copied via SCP)
#   - Config files in /var/lib/samverk/.samverk/ (copied via SCP)
#   - Environment file at /var/lib/samverk/.samverk/samverk.env (copied via SCP)
#
# Usage: bash install.sh

set -euo pipefail

INSTALL_DIR="/var/lib/samverk"
CONFIG_DIR="${INSTALL_DIR}/.samverk"
BIN="/usr/local/bin/samverk"
SERVICE_USER="samverk"

echo "=== Samverk Installation ==="

# Verify binary exists.
if [ ! -f "$BIN" ]; then
    echo "ERROR: Binary not found at $BIN"
    echo "Copy it first: scp bin/samverk-linux-amd64 root@<host>:${BIN}"
    exit 1
fi

# Make binary executable.
chmod 755 "$BIN"

# Verify the binary works.
echo "Binary version: $($BIN version)"

# Verify config directory exists.
if [ ! -d "$CONFIG_DIR" ]; then
    echo "Creating config directory..."
    mkdir -p "$CONFIG_DIR"
fi

# Verify environment file exists.
if [ ! -f "${CONFIG_DIR}/samverk.env" ]; then
    echo "WARNING: Environment file not found at ${CONFIG_DIR}/samverk.env"
    echo "Services will fail to start without API keys."
    echo "Copy samverk.env.example, fill in values, and place at ${CONFIG_DIR}/samverk.env"
fi

# Lock down permissions on secrets.
if [ -f "${CONFIG_DIR}/samverk.env" ]; then
    chmod 600 "${CONFIG_DIR}/samverk.env"
fi
if [ -f "${CONFIG_DIR}/auth.yaml" ]; then
    chmod 600 "${CONFIG_DIR}/auth.yaml"
fi

# Set ownership.
chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR"

# Install systemd unit files.
echo "Installing systemd services..."

# Check if service files were copied alongside install.sh.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "${SCRIPT_DIR}/samverk-serve.service" ]; then
    cp "${SCRIPT_DIR}/samverk-serve.service" /etc/systemd/system/
    cp "${SCRIPT_DIR}/samverk-dispatch.service" /etc/systemd/system/
elif [ -f "/tmp/samverk-serve.service" ]; then
    cp /tmp/samverk-serve.service /etc/systemd/system/
    cp /tmp/samverk-dispatch.service /etc/systemd/system/
else
    echo "ERROR: systemd service files not found."
    echo "Copy deploy/samverk-serve.service and deploy/samverk-dispatch.service"
    echo "to /tmp/ or the same directory as this script."
    exit 1
fi

# Reload systemd and enable services.
systemctl daemon-reload
systemctl enable samverk-serve.service
systemctl enable samverk-dispatch.service

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Start services:"
echo "  systemctl start samverk-serve"
echo "  systemctl start samverk-dispatch"
echo ""
echo "Check status:"
echo "  systemctl status samverk-serve"
echo "  systemctl status samverk-dispatch"
echo "  journalctl -u samverk-serve -f"
echo "  journalctl -u samverk-dispatch -f"
echo ""
echo "Health check:"
echo "  curl http://localhost:8080/healthz"
echo ""
echo "Create an API key for Claude Desktop:"
echo "  sudo -u samverk $BIN key create --name claude-desktop \\"
echo "    --auth-keys ${CONFIG_DIR}/auth.yaml"
