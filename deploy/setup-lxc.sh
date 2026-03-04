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
TEMPLATE="local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"

echo "=== Samverk LXC Setup ==="
echo "Container ID: $CTID"
echo "IP Address:   $IP"
echo ""

# Check if template exists; list available if not.
if ! pveam list local | grep -q "debian-12-standard"; then
    echo "Debian 12 template not found. Downloading..."
    pveam download local debian-12-standard_12.7-1_amd64.tar.zst
fi

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
