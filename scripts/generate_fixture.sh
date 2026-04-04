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

echo "🚀 Starting Atria Fixture Generator..."

# ---------------------------------------------------------
# 0. Setup Local Mock HTTP Server
# ---------------------------------------------------------
echo "📦 Setting up local mock HTTP server at http://localhost:$PORT..."

# Create mock Article 1
cat <<EOF > "$MOCK_DIR/article1.html"
<html><head><title>The Joy of Go</title></head>
<body>
    <h1>The Joy of Go Programming</h1>
    <p>Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.</p>
    <p>This is a fake article served from the local fixture script to test Readability extraction without hitting the internet.</p>
</body></html>
EOF

# Create mock Article 2
cat <<EOF > "$MOCK_DIR/article2.html"
<html><head><title>Why SQLite is Awesome</title></head>
<body>
    <h1>Why SQLite is Awesome</h1>
    <p>SQLite is a C-language library that implements a small, fast, self-contained, high-reliability, full-featured, SQL database engine.</p>
</body></html>
EOF

# Create mock RSS Feed
cat <<EOF > "$MOCK_DIR/feed.xml"
<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
    <title>Mock Tech Blog</title>
    <link>http://localhost:$PORT</link>
    <description>A fake blog for testing</description>
    <item>
        <title>First Mock Post</title>
        <link>http://localhost:$PORT/article1.html</link>
        <description>Summary of the first post.</description>
        <guid>mock-post-1</guid>
    </item>
</channel>
</rss>
EOF

# Start python HTTP server in the background and suppress output
python3 -m http.server $PORT --directory "$MOCK_DIR" > /dev/null 2>&1 &
SERVER_PID=$!

# Ensure the server and temp dir are cleaned up when the script exits (even on failure)
trap 'echo "🧹 Cleaning up local server (PID: $SERVER_PID)..."; kill $SERVER_PID; rm -rf "$MOCK_DIR"' EXIT

sleep 1 # Give the server a moment to start

# Optional: Wipe database before starting (Uncomment if you want automatic reset)
# echo "⚠️ Wiping the database..."
# $ATRIA_CMD db drop --force
# $ATRIA_CMD db migrate

# ---------------------------------------------------------
# 1. User Creation & Role Management
# ---------------------------------------------------------
echo "👥 Creating users..."
$ATRIA_CMD user add --email="admin@atria.local" --name="Admin User" --password="password123" --role=admin
$ATRIA_CMD user add --email="user@atria.local" --name="Standard User" --password="password123" --role=admin

# Downgrade the second user
echo "🔄 Downgrading second user to 'user' role..."
$ATRIA_CMD user role user@atria.local user

# ---------------------------------------------------------
# 2. Admin Content Generation
# ---------------------------------------------------------
echo ""
echo "👑 Generating content for Admin (admin@atria.local)..."
export ATRIA_USER="admin@atria.local"

# Notes & Folders
echo "Hello World!" | $ATRIA_CMD note add "Hello Admin" --path="/"

echo "- Call the bank" | $ATRIA_CMD note add "TODO" --path="/work"
# We capture the UUID of this note to attach a tag later
NOTE_ID=$(echo "My secret recipes" | $ATRIA_CMD note add "Recipes" --path="/home" | awk '/ID:/ {print $2}')
echo "- Buy milk" | $ATRIA_CMD note add "TODO" --path="/home"

# Tags
$ATRIA_CMD tag add "personal"
$ATRIA_CMD tag attach "$NOTE_ID" "personal"

# Articles
$ATRIA_CMD article add "http://localhost:$PORT/article1.html"

# RSS
$ATRIA_CMD rss add "Mock Tech Blog" "http://localhost:$PORT/feed.xml"
$ATRIA_CMD rss fetch

# ---------------------------------------------------------
# 3. Standard User Content Generation
# ---------------------------------------------------------
echo ""
echo "👤 Generating content for Standard User (user@atria.local)..."
export ATRIA_USER="user@atria.local"

# Notes
echo "Hello from the standard user!" | $ATRIA_CMD note add "Hello User" --path="/"

# Articles
$ATRIA_CMD article add "http://localhost:$PORT/article2.html"

# ---------------------------------------------------------
# 4. Verification / Status Output
# ---------------------------------------------------------
echo ""
echo "=========================================================="
echo "✨ Fixtures successfully generated! Verifying current state:"
echo "=========================================================="
echo ""

unset ATRIA_USER

echo "--- USERS ---"
$ATRIA_CMD user list
echo ""

echo "--- ADMIN NOTES ---"
ATRIA_USER=admin@atria.local $ATRIA_CMD note list
echo ""

echo "--- ADMIN ARTICLES ---"
ATRIA_USER=admin@atria.local $ATRIA_CMD article list
echo ""

echo "--- ADMIN TAGS ---"
ATRIA_USER=admin@atria.local $ATRIA_CMD tag list
echo ""

echo "🎉 All done!"
