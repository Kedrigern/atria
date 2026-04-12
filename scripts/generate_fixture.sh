#!/bin/bash
# =====================================================================
# Atria - Fixture Generator
# This script populates a clean database with sample data.
# It starts a temporary local HTTP server to avoid external dependencies.
# =====================================================================

set -e # Exit immediately if a command exits with a non-zero status

# Allow overriding the command (e.g., ATRIA_CMD="./atria" for compiled binary)
ATRIA_CMD=${ATRIA_CMD:-"go run cmd/atria/*.go"}
PORT=9999
MOCK_DIR=$(mktemp -d)
USER_EMAIL=${ATRIA_USER:-"admin@atria.local"}

echo "[BUILD] Compile Atria and Mock Server..."
go build -o bin/atria cmd/atria/*.go
go build -o bin/mockserver cmd/mockserver/main.go

echo "[START] Starting  Mock Server..."
# port 0 = dynamic port
./bin/mockserver -port 0 > mock_info.json &
MOCK_PID=$!

trap "echo '[CLEAN] Cleaning: Shut down Mock Server (PID $MOCK_PID)...'; kill $MOCK_PID; rm -f mock_info.json" EXIT

sleep 1

if ! command -v jq &> /dev/null; then
    echo "❌ Error: 'jq' package is not found."
    exit 1
fi

RSS_URL=$(jq -r '.rss' mock_info.json)
RSS_AUTH_URL=$(jq -r '.rss_auth' mock_info.json)
HOST=$(jq -r '.host' mock_info.json)
PORT=$(jq -r '.port' mock_info.json)
BASE_URL="http://$HOST:$PORT"

echo "[OK] Mock Server runing at port $PORT"
echo "================================================="

echo "[USER] Create default user..."
./bin/atria user add --email $USER_EMAIL --password admin --name Admin || true

echo "[DATA] Add locals RSS feeds..."
./bin/atria rss add "local rss" "$RSS_URL"

# (Volitelné) Až přidáme podporu do CLI, můžeme přidat i ten chráněný
# ./bin/atria rss add "$RSS_AUTH_URL" --auth-type basic --username admin --password secret

echo "[DATA] Fetch rss feeds..."
./bin/atria rss fetch

echo "[DATA] Adding articles..."
./bin/atria article add "$BASE_URL/article/98"
./bin/atria article add "$BASE_URL/article/99"

echo "================================================="
echo "[SUCCESS] Fixtures has been generated!"
