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

echo "[START] Starting Mock Server..."
# port 0 = dynamic port
./bin/mockserver -port 0 > mock_info.json &
MOCK_PID=$!

trap "echo '[CLEAN] Shutting down Mock Server (PID $MOCK_PID)...'; kill $MOCK_PID; rm -f mock_info.json" EXIT

sleep 1

if ! command -v jq &> /dev/null; then
    echo "[ERROR] 'jq' package is not found."
    exit 1
fi

RSS_URL=$(jq -r '.rss' mock_info.json)
RSS_AUTH_URL=$(jq -r '.rss_auth' mock_info.json)
HOST=$(jq -r '.host' mock_info.json)
PORT=$(jq -r '.port' mock_info.json)
BASE_URL="http://$HOST:$PORT"

echo "[OK] Mock Server running at port $PORT"
echo "================================================="

echo "[USER] Create default user..."
./bin/atria user add --email $USER_EMAIL --password admin --name Admin || true

echo "[ADD] Add local RSS feeds..."
./bin/atria rss add "local rss" "$RSS_URL"

echo "[FETCH] Fetch RSS feeds..."
./bin/atria rss fetch

echo "[DATA] Adding articles..."
./bin/atria article add "$BASE_URL/article/98"
./bin/atria article add "$BASE_URL/article/99"

echo "[DATA] Adding notes..."

# 1. Solar Setup (Mermaid, Math, Table, HTML, Naughty Headings)
cat << 'EOF' | ./bin/atria note add "Solar Panels" --path="/home/solar"
# Solar Panel Efficiency Analysis

This document serves to analyze the efficiency of our home solar power plant. We need to calculate it properly before purchasing an additional battery.

Here is a small test of raw HTML in Markdown: <span style="color: #f59e0b; font-weight: bold;">This should be orange and bold!</span>

## Mathematical Model
The efficiency of the system can be expressed using a simple physical model. For our inverter, the following applies:

$$ E = \frac{P_{out}}{P_{in}} \times 100\% $$

And here is a small inline formula: $\Delta T = T_{max} - T_{min}$, which should be rendered directly within the text flow.

# Another H1 (Naughty heading!)
The user wrote this heading as level 1, even though it's in the middle of the text. Our AST parser should shift all these headings to H2 and H3 levels so they don't clash with the main database title.

### Wiring Diagram (Mermaid)
Let's try to draw the architecture here. Our frontend should catch this and render it.

```mermaid
graph TD;
    Panels[Solar Panels 10kWp]-->Inverter[GoodWe Inverter];
    Inverter-->Battery[(Battery 15kWh)];
    Inverter-->Grid((Power Grid));
    Inverter-->House{Appliances};
```

## Production Table
A brief comparison of generation and consumption during summer months.

| Month | Generation (kWh) | Consumption (kWh) | Surplus (kWh) |
|-------|------------------|-------------------|---------------|
| May   | 850              | 320               | 530           |
| June  | 920              | 310               | 610           |
EOF

# 2. Programming Snippets (Intentionally missing H1, Code blocks, Task Lists)
cat << 'EOF' | ./bin/atria note add "Dev Snippets" --path="/work/programming"
## Programming Snippets

This note simulates text that I copied from somewhere else. It intentionally **lacks a level 1 heading (H1)** throughout the text. Our two-pass parser should analyze this and decide not to shift the headings at all.

### Go (Golang)
Golang is fantastic for backend services like Atria. Here is a small example:

```go
package main
import "fmt"
func main() {
    fmt.Println("Hello from Atria Backend!")
}
```

### Things to learn
This will test GFM (GitHub Flavored Markdown) Checkboxes:
- [x] Basics of Go templates
- [x] SQLite via Go
- [ ] Goroutines and Channels
- [ ] Advanced AST transformations in Goldmark
EOF

# 3. Atria Architecture (Čtení z reálného souboru)
if [ -f "docs/architecture/overview.md" ]; then
    ./bin/atria note add "Atria Architecture" --path="/work/atria" < docs/architecture/overview.md
else
    echo "[WARN] docs/architecture/overview.md not found, skipping detailed fixture."
fi

echo "[TAGS] Creating tags..."
./bin/atria tag add "home"
./bin/atria tag add "work"
./bin/atria tag add "tech"

echo "[TAGS] Attaching tags to entities..."
# Attach to RSS
./bin/atria tag attach "local rss" "tech"

# Attach to Articles
./bin/atria tag attach "Mock Article #98: The Future of Testing" "work"
./bin/atria tag attach "Mock Article #98: The Future of Testing" "tech"
./bin/atria tag attach "Mock Article #99: The Future of Testing" "work"

# Attach to Notes
./bin/atria tag attach "Solar Panels" "home"
./bin/atria tag attach "Dev Snippets" "work"
./bin/atria tag attach "Atria Architecture" "tech"

echo "[ATTACH] Creating dummy files and attaching them..."
echo "Manual for solar inverter model X-100" > inverter_manual.txt
echo "Meeting notes from architectural review" > arch_review.pdf

./bin/atria attachment add inverter_manual.txt --link "Solar Panels"
./bin/atria attachment add arch_review.pdf --link "Atria Architecture"

rm inverter_manual.txt arch_review.pdf

echo "================================================="
echo "[SUCCESS] Fixtures have been generated!"
```

echo "[TAGS] Creating tags..."
./bin/atria tag add "home"
./bin/atria tag add "work"
./bin/atria tag add "tech"

echo "[TAGS] Attaching tags to entities..."
# Attach to RSS
./bin/atria tag attach "local rss" "tech"

# Attach to Articles (pomocí přesných názvů z mock serveru)
./bin/atria tag attach "Mock Article #98: The Future of Testing" "work"
./bin/atria tag attach "Mock Article #98: The Future of Testing" "tech"
./bin/atria tag attach "Mock Article #99: The Future of Testing" "work"

# Attach to Notes
./bin/atria tag attach "Solar Setup" "home"
./bin/atria tag attach "Standup Notes" "work"

echo "[ATTACH] Creating dummy files and attaching them..."
echo "Manual for solar inverter model X-100" > inverter_manual.txt
echo "Meeting notes from architectural review" > arch_review.pdf

./bin/atria attachment add inverter_manual.txt --link "Solar Setup"
./bin/atria attachment add arch_review.pdf --link "Standup Notes"

rm inverter_manual.txt arch_review.pdf

echo "================================================="
echo "[SUCCESS] Fixtures have been generated!"
