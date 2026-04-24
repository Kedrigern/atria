
## Atria - Phased Development Roadmap

This document outlines the sequential, phased implementation plan for Atria. Based on the "Online-First, Offline-Read" philosophy, Atria relies on the server as the absolute source of truth for all editing, eliminating offline merge conflicts and allowing for a much lighter frontend architecture.

### Phase 1: Foundation & Identity (The Core Engine)
**Goal:** Establish the database, frontend paradigm, and user scaffolding.
* **Database Schema:** Implement the core PostgreSQL schema using UUIDv7 for chronologically sorted, highly performant primary keys.
* **Polymorphic Base:** Create the central `entities` table (Class Table Inheritance) to hold shared metadata for all content types, supporting infinite folder hierarchies and native access control (`private`, `users`, `public`).
* **Full-Text Search Foundation:** Add a `search_vector` column to the `entities` table with PostgreSQL triggers to enable fast global search across all future content types without heavy `JOIN` operations.
* **Frontend Architecture:** Solidify the lightweight HTMX approach, dropping the heavy IndexedDB requirement in favor of standard server-rendered HTML with small JavaScript enhancements.
* **Authentication:** Build the multi-user account system and session management.

### Phase 2: Connective Tissue (Storage, Tags, & API)
**Goal:** Build the systems that link, store, and deduplicate data.
* **API Layer:** Scaffold the high-performance REST API using Go + Gin.
* **Knowledge Graph & Tags:** Implement the `#tags` system and the `item_links` junction table to support universal backlinks.
* **Attachment Engine:** Build the filesystem storage handler for binary files, paired with the `attachments` tracking table.
* **Anti-Bloat GC:** Implement SHA-256 file hashing on upload to prevent duplicates, and build the background Garbage Collection worker to prune orphaned files.

### Phase 3: Triage (RSS Reader)
**Goal:** Fast ingestion of external information.
* **Data Model:** Create the `rss` subtype table linked to `entities`.
* **Background Workers:** Implement automated fetching and parsing of RSS/Atom feeds.
* **Skimmable UI:** Build the fast triage list view displaying only titles and excerpts.
* **Action Hooks:** Implement the one-click transfer function to push feed items into the Read-it-Later pipeline.
* **Offline Reading:** Implement a basic Service Worker to cache the rendered HTMX lists and article views for offline reading on mobile/e-ink browsers.

### Phase 4: Archival (Read-it-Later & E-Ink Export)
**Goal:** Clean, save, and permanently store web content for deep reading.
* **Data Model:** Create the `articles` subtype table to store cleaned HTML.
* **Readability Extraction:** Integrate a parser to strip ads, popups, and menus, saving clean text.
* **The E-ink Pipeline:** Implement the `.epub` bulk export feature. This allows users to bundle their unread inbox into a single file for native, highly optimized offline reading on Kindle/Kobo/Android e-readers.
* **Web Archiving:** Defer WARC generation to a later version; it is not part of v1.

### Phase 5: Creation (Knowledge Base & Notes)
**Goal:** Distraction-free writing and structured knowledge.
* **Data Model:** Create the `notes` subtype table to store raw `markdown_content`.
* **Markdown Engine:** Implement Markdown rendering (including Mermaid.js support).
* **Concurrency Security:** Implement Phase 1 Pessimistic Locking (`locked_by`, `locked_at`). Because the app is online-only for editing, this lock will work flawlessly to prevent simultaneous overwrites.
* **Export/Publish Sync:** Implement a one-way CLI export of Markdown files to support static-site generators (like Zensical) without the messy conflicts of a bi-directional sync.

### Phase 6: Data & Dashboards
**Goal:** High-level overviews and lightweight tabular data.
* **Dashboards:** Build customizable grid layouts with widgets for unread counts and recent notes.
* **Simple Spreadsheets:** Implement basic CSV or SQL-backed tables that can be embedded directly within Markdown notes.
