# Atria - Core Libraries & Dependencies

## 1. Web Framework & Routing
* **`github.com/gin-gonic/gin`**
  High-performance HTTP web framework. Used for handling API requests and serving HTML/HTMX templates.

## 2. CLI (Command Line Interface)
* **`github.com/spf13/cobra`**
  Industry standard for building CLI applications. Used for subcommands like `atria db migrate` or `atria user add`.

## 3. Database & Migrations
* **`github.com/lib/pq`**
  Pure Go PostgreSQL driver for the standard `database/sql` package.
* **`github.com/pressly/goose/v3`**
  Database migration tool. Allows writing migrations in plain SQL and embedding them directly into the Go binary.

## 4. Configuration
* **`github.com/joho/godotenv`**
  Loads environment variables from a `.env` file for local development (follows 12-Factor App methodology).

## 5. Security & Identifiers
* **`golang.org/x/crypto/bcrypt`**
  Standard cryptographic library for secure user password hashing.
* **`github.com/gofrs/uuid/v5`**
  Used to generate UUIDv7. Ensures chronological sorting and prevents B-Tree index fragmentation in PostgreSQL.

## 6. RSS & Feed Parsing
* **`github.com/mmcdole/gofeed`**
  Robust RSS and Atom feed parser. Reliably handles various feed formats, broken XMLs, and edge cases.

## 7. Read-it-Later & Article Extraction
* **`github.com/go-shiori/go-readability`**
  Go port of Mozilla's Readability.js. Strips ads, menus, and popups to extract clean article text and metadata.

## 8. E-Ink Export
* **`github.com/bmaupin/go-epub`**
  Generates valid EPUB files. Used to bundle saved articles for native offline reading on e-readers (Kindle, Kobo).
