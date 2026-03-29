#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-paperclip}"
SERVER_CONTAINER_NAME="${SERVER_CONTAINER_NAME:-paperclip-server}"
DB_CONTAINER_NAME="${DB_CONTAINER_NAME:-paperclip-db}"
HOST_PORT="${HOST_PORT:-3100}"
DATA_DIR="${DATA_DIR:-$REPO_ROOT/data/apple-container-paperclip}"
DB_DATA_DIR="${DB_DATA_DIR:-$REPO_ROOT/data/apple-container-pgdata}"

mkdir -p "$DATA_DIR"
mkdir -p "$DB_DATA_DIR"

if ! command -v container &> /dev/null; then
  echo "Error: The 'container' command was not found."
  echo "Please install Apple's container tool from https://github.com/apple/container"
  exit 1
fi

echo "==> Setting up network"
if ! container network list | grep -q "paperclip-net"; then
  container network create paperclip-net || true
fi

echo "==> Starting database container ($DB_CONTAINER_NAME)"
# Remove existing container if it exists
if container ls --all | grep -q "$DB_CONTAINER_NAME"; then
  container rm -f "$DB_CONTAINER_NAME" >/dev/null 2>&1 || true
fi

container run -d \
  --name "$DB_CONTAINER_NAME" \
  --network paperclip-net \
  -p 5432:5432 \
  --env POSTGRES_USER=paperclip \
  --env POSTGRES_PASSWORD=paperclip \
  --env POSTGRES_DB=paperclip \
  --volume "$DB_DATA_DIR:/var/lib/postgresql/data" \
  postgres:17-alpine

echo "==> Waiting for database to be ready..."
sleep 5 # Simple wait for PostgreSQL to boot

echo "==> Building server image using Apple's container tool"
container build --tag "$IMAGE_NAME" --file "$REPO_ROOT/Dockerfile" "$REPO_ROOT"

echo "==> Starting server container ($SERVER_CONTAINER_NAME)"
# Remove existing container if it exists
if container ls --all | grep -q "$SERVER_CONTAINER_NAME"; then
  container rm -f "$SERVER_CONTAINER_NAME" >/dev/null 2>&1 || true
fi

BETTER_AUTH_SECRET="${BETTER_AUTH_SECRET:-$(openssl rand -hex 32)}"

container run -d \
  --name "$SERVER_CONTAINER_NAME" \
  --network paperclip-net \
  -p "$HOST_PORT:3100" \
  --env HOST=0.0.0.0 \
  --env PORT=3100 \
  --env DATABASE_URL="postgres://paperclip:paperclip@${DB_CONTAINER_NAME}.test:5432/paperclip" \
  --env PAPERCLIP_HOME=/paperclip \
  --env BETTER_AUTH_SECRET="$BETTER_AUTH_SECRET" \
  --volume "$DATA_DIR:/paperclip" \
  "$IMAGE_NAME"

echo "==> Paperclip is running on http://localhost:$HOST_PORT"
echo "    Database is running on localhost:5432"
