#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-paperclip-macos}"
HOST_PORT="${HOST_PORT:-3100}"
DATA_DIR="${DATA_DIR:-$REPO_ROOT/data/macos-paperclip}"

mkdir -p "$DATA_DIR"

echo "==> Building macOS container image"
docker build -f Dockerfile.macos -t "$IMAGE_NAME" .

echo "==> Running macOS container"
docker run -d --rm \
  --name "$IMAGE_NAME" \
  -p "$HOST_PORT:3100" \
  -e HOST=0.0.0.0 \
  -e PORT=3100 \
  -e PAPERCLIP_HOME=/paperclip \
  -v "$DATA_DIR:/paperclip" \
  "$IMAGE_NAME"

echo "==> Paperclip is running on http://localhost:$HOST_PORT"
