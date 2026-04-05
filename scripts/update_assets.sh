#!/bin/bash
# =====================================================================
# Atria - Asset Updater
# Downloads external CSS/JS dependencies for local offline embedding.
# =====================================================================

set -e

# Define target directory
STATIC_DIR="internal/web/static"
mkdir -p "$STATIC_DIR"

HTMX_VERSION="latest" # 2.0.8

echo "⬇️  Downloading water.css..."
curl -sL "https://cdn.jsdelivr.net/npm/water.css@2/out/water.css" -o "$STATIC_DIR/water.css"

echo "⬇️  Downloading htmx.min.js..."
curl -sL "https://unpkg.com/htmx.org@$HTMX_VERSION/dist/htmx.min.js" -o "$STATIC_DIR/htmx.min.js"

echo "✅ All static assets updated successfully!"
