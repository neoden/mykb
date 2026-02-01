#!/bin/bash
set -e

# Load environment variables from .env if it exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

HOST="${DEPLOY_HOST:?DEPLOY_HOST not set. Add it to .env or export it.}"

echo "Building..."
GOOS=linux GOARCH=amd64 go build -o mykb.bin .

echo "Stopping service..."
ssh ${HOST} "systemctl stop mykb 2>/dev/null || true"

echo "Copying binary..."
scp mykb.bin ${HOST}:/usr/local/bin/mykb
rm mykb.bin

echo "Copying systemd unit..."
scp mykb.service ${HOST}:/etc/systemd/system/mykb.service

echo "Setting up on remote..."
ssh ${HOST} << 'EOF'
    # Create user if not exists
    id mykb &>/dev/null || useradd -r -s /sbin/nologin mykb

    # Create directories
    mkdir -p /var/lib/mykb
    mkdir -p /etc/mykb
    chown mykb:mykb /var/lib/mykb

    # Check config exists
    if [ ! -f /etc/mykb/config.toml ]; then
        echo "WARNING: /etc/mykb/config.toml not found. Create it before starting."
    fi

    # Reload and restart
    systemctl daemon-reload
    systemctl enable mykb
    systemctl restart mykb
EOF

echo "Status:"
ssh ${HOST} "systemctl status mykb --no-pager"

echo "Done!"
