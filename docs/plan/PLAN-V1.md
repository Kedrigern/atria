# Atria - V1 Plan

## 1. Goal

Ship a calm, reliable first version of Atria focused on **reading, saving, and linking knowledge**.

Atria v1 should be useful as a daily personal system for:

- collecting information
- triaging incoming content
- reading in a clean interface
- turning saved reading into notes
- organizing knowledge with tags and links
- exporting reading bundles to `.epub`

## 2. Product Shape

Atria v1 is:

- an **online-first** application
- a **read-it-later** system
- an **RSS triage** tool
- a **Markdown knowledge base**
- a web app with **offline reading** support

Atria v1 is **not**:

- an offline-first editing platform
- a real-time collaborative editor
- a full web archiving product
- a heavy SPA centered on complex client-side state

## 3. Technical Direction

The agreed technical direction for v1 is:

- **Backend:** Go + Gin
- **Database:** PostgreSQL
- **Frontend:** server-rendered HTML with HTMX and small JavaScript enhancements
- **App model:** online-first with offline reading
- **Storage:** text and metadata in PostgreSQL, binary files on the local filesystem

## 4. Core User Workflow

The main v1 workflow is:

1. Subscribe to RSS feeds or save a URL directly
2. Skim and triage incoming content
3. Save interesting items into the reading inbox
4. Read articles in a clean distraction-free view
5. Archive or organize content with tags
6. Create notes and link ideas together
7. Export selected reading to `.epub`
8. Re-open saved content offline for reading when needed

## 5. Core Platform Capabilities

These are foundational systems required early because multiple features depend on them.

### 5.1. Accounts and Access
- user accounts and authentication
- item ownership
- visibility levels: `private`, `users`, `public`

### 5.2. Shared Content Model
- central `entities` model
- folders / hierarchy
- consistent metadata across notes, articles, and related content

### 5.3. Tags
Tags are part of v1 core scope.

They should work across major content types and support:
- organization
- filtering
- search
- lightweight categorization without forcing deep folder trees

### 5.4. Backlinks and Internal Linking
Atria v1 should support:
- internal links such as `[[Note Name]]`
- backlink generation
- connected navigation between related notes and articles

### 5.5. Attachments
Attachments are a **core cross-cutting platform capability**, not just a notes feature.

They should be designed early because they may be used by:
- notes
- saved articles
- other content types as needed

V1 attachment support should include:
- local filesystem storage
- database references
- deduplication by file hash
- clean reuse across the application

### 5.6. Search
Search is required for v1.

It should support:
- notes
- saved articles
- titles
- body text
- tags

### 5.7. Background Jobs
Background processing is required for:
- RSS fetching
- article fetching and parsing
- EPUB generation
- garbage collection / pruning

## 6. User-Facing V1 Features

### 6.1. RSS Reader
V1 includes:
- feed subscription
- background fetching
- title/excerpt triage view
- quick action to save an item into the reading pipeline

### 6.2. Read-it-Later
V1 includes:
- saving a URL
- backend fetching
- readability-style extraction
- clean article view
- inbox / archive workflow
- public read-only sharing

### 6.3. Markdown Notes
V1 includes:
- Markdown note creation and editing
- folder organization
- tags
- internal links
- backlinks
- Mermaid rendering if practical in the initial release

### 6.4. Offline Reading
V1 includes:
- installable web app behavior
- service worker caching
- offline access to previously viewed or explicitly cached reading content

This is for **reading only**, not editing.

### 6.5. EPUB Export
V1 includes:
- selecting saved articles
- generating a `.epub`
- downloading the result for e-reader use

## 7. Basic Activity Tracking

Atria v1 should include small but useful activity metadata.

### 7.1. Entity Activity
Each entity should track basic last-change information:
- when it was last updated
- who updated it last

This is intended to answer simple questions like:
- who changed this last?
- when was this updated?

### 7.2. User Activity
User records should track basic login/activity information:
- last successful login time
- optionally last seen time if implementation remains simple

### 7.3. Scope Limit
This is **not** a full audit log in v1.

V1 does not require:
- full revision history
- per-field change history
- immutable event logs
- complete activity timelines

## 8. Explicitly Out of Scope for V1

The following are not part of Atria v1:

- WARC archiving
- offline editing
- IndexedDB as the main application database
- bi-directional Markdown directory sync
- real-time collaboration
- CRDT/Yjs-based synchronization
- mentions and notifications as core collaboration features
- advanced dashboards
- spreadsheet-grade editing
- full browser extension capture of rendered authenticated pages

Future collaboration technology may remain documented, but it is not part of the current v1 delivery target.

## 9. V1 Success Criteria

Atria v1 is successful if a user can:

- subscribe to feeds
- skim incoming RSS items quickly
- save articles from RSS or direct URLs
- read saved articles in a clean interface
- organize items with tags
- create notes and connect them through links/backlinks
- attach supporting files where needed
- search across saved knowledge
- export selected articles to EPUB
- read saved content offline
- understand basic last-update and last-login information

## 10. Guiding Principle

Atria v1 should prioritize **clarity, calmness, and durability** over feature breadth.

If a feature adds significant complexity but does not strengthen the core workflow of:

**capture -> triage -> read -> note -> link -> retrieve -> export**

then it should be postponed.