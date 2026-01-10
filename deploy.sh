#!/bin/bash
set -e

# Load environment variables from .env if it exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

HOST="${DEPLOY_HOST:?DEPLOY_HOST not set. Add it to .env or export it.}"
REMOTE_DIR="/opt/mykb"

echo "Syncing files..."
rsync -av --exclude='.venv' --exclude='__pycache__' --exclude='*.db' --exclude='.git' --exclude='.env' \
    ./ ${HOST}:${REMOTE_DIR}/

echo "Rebuilding and restarting app..."
ssh ${HOST} "cd ${REMOTE_DIR} && docker compose up -d --build app"

echo "Checking logs..."
ssh ${HOST} "cd ${REMOTE_DIR} && docker compose logs --tail=10 app"

echo "Done!"
