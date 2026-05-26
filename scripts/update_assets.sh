#!/bin/bash
# =====================================================================
# Atria - Asset Updater
# Downloads external CSS/JS dependencies for local offline embedding.
# =====================================================================

set -e

# Define target directory
STATIC_DIR="internal/web/static"
mkdir -p "$STATIC_DIR"

HTMX_VERSION="latest" # 2.0.10
TABULATOR_VERSION="@6.4.0" #empty means latest

echo "[GET] Downloading water.css..."
curl -sL "https://cdn.jsdelivr.net/npm/water.css@2/out/water.css" -o "$STATIC_DIR/water.css"

echo "[GET] Downloading htmx.min.js..."
curl -sL "https://unpkg.com/htmx.org@$HTMX_VERSION/dist/htmx.min.js" -o "$STATIC_DIR/htmx.min.js"

echo "[GET] Downloading tabulator.min.css..."
curl -sL "https://unpkg.com/tabulator-tables$TABULATOR_VERSION/dist/css/tabulator.min.css" -o "$STATIC_DIR/tabulator.min.css"

echo "[GET] Downloading tabulator.min.js..."
curl -sL "https://unpkg.com/tabulator-tables$TABULATOR_VERSION/dist/js/tabulator.min.js" -o "$STATIC_DIR/tabulator.min.js"

echo "[OK] All static assets updated successfully!"
