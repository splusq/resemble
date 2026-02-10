#!/bin/bash
set -euo pipefail

echo "=== resemble smoke test ==="

# 1. Build binary
echo "Building..."
go build -o resemble ./cmd/resemble/

# 2. Init with a temp git repo
TMPDIR=$(mktemp -d)
CACHE_REPO="$TMPDIR/smoke-cache.git"
git init --bare "$CACHE_REPO" >/dev/null 2>&1
./resemble init --cache-repo "$CACHE_REPO"

# 3. Start proxy against httpbin
./resemble start --upstream https://httpbin.org --port 9999 &
PROXY_PID=$!
sleep 2

cleanup() {
    kill $PROXY_PID 2>/dev/null || true
    rm -rf "$TMPDIR"
    rm -f resemble
    rm -f .testproxy.yml
    rm -rf .resemble-cache
}
trap cleanup EXIT

# 4. First request (should record)
echo ""
echo "--- First request (record) ---"
RESP=$(curl -s http://localhost:9999/get)
echo "$RESP" | head -5
echo "..."

# 5. Second request (should replay from cache)
echo ""
echo "--- Second request (replay) ---"
RESP2=$(curl -s http://localhost:9999/get)
echo "$RESP2" | head -5
echo "..."

# 6. Status
echo ""
echo "--- Status ---"
./resemble status

# 7. Stop
echo ""
echo "--- Stop ---"
kill $PROXY_PID
wait $PROXY_PID 2>/dev/null || true

echo ""
echo "=== Smoke test passed ==="
