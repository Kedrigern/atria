# Atria - Product & Technical Specification

## 1. Overview
**Atria** is a personal mind palace and collaborative knowledge base. It unifies a Read-it-Later service, an RSS reader, Markdown-based notes, dashboards, and basic spreadsheets into a single, blazing-fast, distraction-free environment. 

While designed for absolute personal focus, Atria supports multi-user instances, allowing small teams or families to share an instance and share specific items. The core philosophy of Atria is **focus and permanence**: removing cognitive overload, avoiding data lock-in, and preventing storage bloat while maintaining offline reading availability.

## 2. Core Modules

### 2.1. RSS Reader
* **Purpose:** Fast triage of external information.
* **Features:**
  * Automated background fetching of RSS/Atom feeds.
  * Skimmable list view (titles and excerpts only) for quick "keep or discard" decisions.
  * **Action:** One-click transfer of an interesting feed item to the "Read-it-Later" inbox, which triggers the backend to download and parse the full article.
* Inspired by https://github.com/miniflux/v2

### 2.2. Read-it-Later (Bookmarks & Articles)
* **Purpose:** Distraction-free reading and long-term archiving.
* **Workflow:** `Inbox` (to be read) ➔ `Archive` (read and saved).
* **Features:**
  * **Readability Extraction:** Automatically strips ads, menus, and popups, saving only the clean text, author/domain, date, and main images.
  * **Full Web Archiving (WARC):** Planned for a later version and explicitly out of scope for v1.
  * **E-ink Export:** Select multiple articles and generate a `.epub` file for reading on Kindle/Kobo devices.
  * **Public Sharing:** Generate public, read-only links for specific articles.
* Inspired by: https://github.com/go-shiori/shiori

### 2.3. Knowledge Base (Markdown Notes)
* **Purpose:** Personal and public note-taking and knowledge linking.
* **Features:**
  * Full Markdown support (including YAML frontmatter for metadata).
  * **Advanced Plugins:** Support for extended Markdown features, notably **Mermaid.js** for rendering complex diagrams.
  * **Organization:** Standard folder hierarchy combined with flexible `#tags`.
  * **Inline Attachments:** Support for uploading and embedding images and PDF documents.
  * **User Mentions:** Planned for a later version and not a priority for v1.
  * **Directory Sync (Hybrid Storage):** For v1, this is reduced to one-way export of Markdown files rather than bi-directional sync.
* inspired by: 
  * https://github.com/usememos/memos
  * https://github.com/hackmdio/CodiMD


### 2.4. Dashboards (New)
* **Purpose:** High-level overview and real-time data visualization.
* **Features:**
  * Customizable grid layout.
  * Widget support to display internal data (e.g., recent notes, unread RSS count) or external API integrations (e.g., bank account balances, weather).

### 2.5. Spreadsheets
* **Purpose:** Lightweight tabular data management.
* **Features:**
  * Basic grid interface with rows, columns, and simple formulas (SUM, AVERAGE).
  * Can be embedded or linked within Markdown notes.

## 3. Cross-Cutting Features

### 3.1. The Knowledge Graph (Backlinks)
Everything in Atria is connected. A dedicated linking engine parses internal links (e.g., `[[Note Name]]`) and maintains a graph. Every article, note, or spreadsheet displays a "Mentioned in" section at the bottom, creating an organic web of knowledge.

### 3.2. Users & Collaboration
* **Multi-User Instance:** A single Atria server can host multiple isolated accounts.
* **Mentions & Notifications:** Deferred beyond v1.
* **Authentication (SSO-Only):** Web access is secured exclusively via Forward Authentication (Authelia/Authentik). Atria handles zero passwords or login screens.
* **Access Control:** Users can set item visibility to strictly private, shareable across the instance (`users`), or fully public (`public`).

### 3.3. Data Ingestion
* **Web Extension:** Captures the *rendered* HTML of the current page, allowing Atria to save articles behind paywalls or SSO.
* **Mobile Share Target:** Fully integrated into the native Android/iOS share sheet via PWA Web Share Target API.

### 3.4. Online-First PWA with Offline Reading
* Installable on mobile and desktop devices.
* Uses **Service Workers** to cache UI assets and previously viewed content for offline reading.
* The server remains the source of truth for editing; offline editing is out of scope for v1.

### 3.5. Command Line Interface (CLI)
* **Purpose:** Frictionless quick-capture and management for power users directly from the terminal.
* **Architecture:** A standalone Go CLI client that communicates with the Atria Gin backend.
* **Core Commands (Domain-Driven):**
  * `atria link add <url>`: Saves and parses an article.
  * `atria note add "content" --tags`: Quick-captures a Markdown note.
  * `atria search run <query>`: Performs a full-text search across all items.
  * `atria export epub`: Generates an `.epub` export from selected saved articles.
  * `atria system prune`: Forces the garbage collection process to free up disk space.
  *(Note: Short aliases can be configured later).*

## 4. Technical Architecture

### 4.1. Technology Stack
* **Backend API:** Go + Gin.
* **CLI Client:** Go.
* **Database:** PostgreSQL. *(For detailed schema, indexing strategies, and polymorphic design, refer to `ARCHITECTURE.md`).*
* **Frontend:** Server-rendered HTML with HTMX and small JavaScript enhancements.

### 4.2. Storage & Anti-Bloat Strategy
To prevent the application from bloating, Atria employs strict storage management:
* **Separation of Concerns:** PostgreSQL stores only text and references. Binary files (images, PDFs, generated EPUB files) are saved directly to the local file system.
* **Deduplication:** Every incoming file is hashed (SHA-256) to prevent saving duplicates.
* **Workflow Pruning (Garbage Collection):** Background workers clean up orphaned attachments when items are archived or hard-deleted.
* **X-Accel-Redirect:** The backend handles authentication for private files and generated downloads, but delegates the actual file serving to a reverse proxy (Nginx/Caddy).

### 4.3. Identifiers & Discovery
* **UUIDv7 Suffixes:** While the system uses full UUIDv7 for storage, the CLI and UI prioritize the **last 8 characters** (suffix) for display and resolution. 
* **Rationale:** UUIDv7 starts with a timestamp. Items created in quick succession share identical prefixes. Using the random suffix ensures visual uniqueness in lists.
* **Resolution:** The system supports resolving entities by full UUID, title, or the 8-character suffix.

## 5. Concurrency & Collaborative Editing

Handling multiple users accessing and editing the same document (Note, Spreadsheet) requires a structured approach to prevent data loss or conflicting states. Atria implements this in two phases:

### Phase 1: Pessimistic Locking (Current Implementation)
To keep the initial architecture robust and straightforward, Atria uses a locking mechanism (similar to Atlassian Confluence or Microsoft SharePoint).
* **Database Level:** The `entities` table includes two transient columns: `locked_by` (UUID referencing the User) and `locked_at` (Timestamp).
* **Workflow:**
  1. User A opens a note for editing. The backend sets the lock.
  2. User B opens the same note. The backend detects the lock and serves the document in **Read-Only mode**, displaying a banner: *"This item is currently being edited by User A."*
  3. When User A saves and closes the document, or after a specific timeout (e.g., 15 minutes of inactivity), the lock is released.

### Phase 2: Real-time Collaboration (Future Roadmap)
For future iterations, Atria is designed to support real-time, Google Docs-style collaboration. This will be achieved without locking, using **CRDT (Conflict-free Replicated Data Types)**.

* **Frontend:** Migration to a CRDT-aware rich text editor, such as **Tiptap** or **BlockNote** (which provides a Notion-like block interface).
* **Core Technology:** The **Yjs** ecosystem. Yjs natively supports offline-first PWA capabilities, merging changes seamlessly once the device reconnects.
* **Backend Integration:** * FastAPI's native asynchronous WebSockets will be used to broadcast changes.
  * The Python `y-py` and `ypy-websocket` libraries will act as the synchronization server.
  * **Storage considerations:** The `notes` table will be extended with a `crdt_state` (BYTEA/Binary) column to store the Yjs history state, which the backend will periodically compile down into plain `markdown_content` for full-text search and Zensical directory sync.
  * **Scope note:** This technology direction remains documented as a possible future roadmap, but it is explicitly out of the current v1 scope.
