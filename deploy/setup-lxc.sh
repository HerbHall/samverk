#!/bin/bash
# setup-lxc.sh -- Creates a Proxmox LXC container for Samverk.
# Run this on the Proxmox host (192.168.1.203).
#
# Usage: bash setup-lxc.sh [CONTAINER_ID] [IP_ADDRESS]
#   CONTAINER_ID defaults to 201
#   IP_ADDRESS defaults to 192.168.1.161

set -euo pipefail

CTID="${1:-201}"
IP="${2:-192.168.1.161}"
HOSTNAME="samverk"
echo "=== Samverk LXC Setup ==="
echo "Container ID: $CTID"
echo "IP Address:   $IP"
echo ""

# Find or download the latest Debian 12 template.
TEMPLATE_FILE=$(pveam list local | grep "debian-12-standard" | awk '{print $1}' | tail -1)
if [ -z "$TEMPLATE_FILE" ]; then
    echo "Debian 12 template not found locally. Downloading latest..."
    LATEST=$(pveam available --section system | grep "debian-12-standard" | awk '{print $2}' | tail -1)
    if [ -z "$LATEST" ]; then
        echo "ERROR: No Debian 12 standard template found in pveam repository."
        exit 1
    fi
    pveam download local "$LATEST"
    TEMPLATE_FILE="local:vztmpl/${LATEST}"
else
    echo "Using template: $TEMPLATE_FILE"
fi
TEMPLATE="$TEMPLATE_FILE"

# Create the container.
echo "Creating LXC container $CTID..."
pct create "$CTID" "$TEMPLATE" \
    --hostname "$HOSTNAME" \
    --memory 512 \
    --swap 256 \
    --cores 2 \
    --rootfs local-lvm:4 \
    --net0 name=eth0,bridge=vmbr0,ip="${IP}/24",gw=192.168.1.1 \
    --nameserver 192.168.1.1 \
    --unprivileged 1 \
    --onboot 1 \
    --features nesting=1 \
    --start 0

echo "Starting container..."
pct start "$CTID"

# Wait for container networking.
echo "Waiting for container to boot..."
sleep 5

# Update and install minimal dependencies.
echo "Updating packages..."
pct exec "$CTID" -- apt-get update -qq
pct exec "$CTID" -- apt-get upgrade -y -qq
pct exec "$CTID" -- apt-get install -y -qq curl

# Create service user and directories.
echo "Creating samverk user and directories..."
pct exec "$CTID" -- useradd -r -s /usr/sbin/nologin -d /var/lib/samverk -m samverk
pct exec "$CTID" -- mkdir -p /var/lib/samverk/.samverk
pct exec "$CTID" -- chown -R samverk:samverk /var/lib/samverk

echo ""
echo "=== LXC container $CTID created ==="
echo ""
echo "Next steps:"
echo "  1. Copy the binary:  scp bin/samverk-linux-amd64 root@${IP}:/usr/local/bin/samverk"
echo "  2. Copy install.sh:  scp deploy/install.sh root@${IP}:/tmp/install.sh"
echo "  3. Copy configs:     scp deploy/config/* root@${IP}:/var/lib/samverk/.samverk/"
echo "  4. Copy env file:    scp deploy/samverk.env root@${IP}:/var/lib/samverk/.samverk/samverk.env"
echo "  5. Run installer:    ssh root@${IP} 'bash /tmp/install.sh'"
echo ""
echo "Or run 'make deploy DEPLOY_HOST=${IP}' from the project root."
