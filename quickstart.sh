#!/bin/bash
set -e

echo "🚀 Starting Vashandi Quickstart..."

# 1. Check for .env
if [ ! -f .env ]; then
    echo "📝 Creating .env from .env.example..."
    cp .env.example .env
fi

# 2. Start Core Services
echo "🐳 Starting core services (database, redis, CA)..."
docker compose up -d ca db redis

# 3. Wait for CA to generate fingerprint
echo "⏳ Waiting for CA to initialize..."
MAX_RETRIES=10
COUNT=0
FINGERPRINT=""

while [ $COUNT -lt $MAX_RETRIES ]; do
    FINGERPRINT=$(docker logs vashandi-ca-1 2>&1 | grep "Root fingerprint" | head -n 1 | awk '{print $NF}')
    if [ ! -z "$FINGERPRINT" ]; then
        break
    fi
    echo "  ...still waiting..."
    sleep 2
    COUNT=$((COUNT + 1))
done

if [ -z "$FINGERPRINT" ]; then
    echo "❌ Failed to extract CA fingerprint. Check 'docker logs vashandi-ca-1'."
    exit 1
fi

echo "🔑 Extracted Fingerprint: $FINGERPRINT"

# 4. Update .env
# Use a temporary file to avoid sed compatibility issues between macOS/Linux
grep -v "STEP_CA_FINGERPRINT=" .env > .env.tmp || true
echo "STEP_CA_FINGERPRINT=$FINGERPRINT" >> .env.tmp
mv .env.tmp .env

# 5. Start everything
echo "🚀 Starting remaining services (building images)..."
# Export it for this session to ensure immediate pick-up
export STEP_CA_FINGERPRINT=$FINGERPRINT
docker compose up -d --build vashandi-ca-sidecar vashandi openbrain vashandi-ui

echo ""
echo "✅ Vashandi is coming up!"
echo "📍 API & UI: http://localhost:3100"
echo "🧠 OpenBrain: http://localhost:3101"
echo ""
echo "🛠️  Subsequent runs: Just use 'docker compose up -d'"
echo "💡 Note: If you change Go code, re-run this script to rebuild the binary."
