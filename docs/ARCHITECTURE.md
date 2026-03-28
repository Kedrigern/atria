# Atria - Architecture & Database Design

This document outlines the core architectural decisions and database schema design for Atria. The system relies on **PostgreSQL** as the primary datastore, utilizing the local filesystem for binary payloads to keep the database lean. Atria is designed as an online-first application with offline reading support rather than offline-first editing.

## 1. Core Architectural Principles
1. **Relational Integrity over Document Stores:** Because Atria is fundamentally a graph of connected thoughts (backlinks, tags, folders), a strictly relational database (PostgreSQL) is far superior to a NoSQL document store.
2. **Lean Database (Zero Binary Bloat):** The PostgreSQL database must remain incredibly small and fast. It stores *only* text and metadata. All binary files (Images, PDFs, generated EPUB files, and future archive formats) are stored on the local filesystem, with the database acting only as a reference index.
3. **Universal Identifiers:** Every object in the system is globally addressable via a UUID.
4. **Framework Agnostic:** The database schema enforces its own integrity (cascades, check constraints) and does not rely on application-layer ORM logic to maintain state.

## 2. Primary Keys: Why UUIDv7?
Atria uses **UUIDv7** for all primary keys instead of standard auto-incrementing integers or UUIDv4.

* **Why UUIDs?** It prevents users from guessing the total number of notes/articles in the system, makes merging databases easier, and leaves room for future distributed clients if needed.
* **Why v7 over v4?** UUIDv4 is completely random, which causes severe index fragmentation in B-Tree databases like PostgreSQL at scale. UUIDv7 embeds a UNIX timestamp in the first 48 bits. This means IDs are naturally sorted chronologically, keeping database inserts blazing fast and indices perfectly organized.

## 3. The Polymorphic Data Model (Class Table Inheritance)
To allow any item to link to any other item without complex schema hacks, Atria uses the **Class Table Inheritance** pattern. Instead of having completely independent tables for Notes and Articles, we use a central `entities` table that holds shared metadata, and specific subtype tables that hold the unique content.

### 3.1. Central `entities` Table
This table acts as the universal registry for every piece of content in Atria. It supports infinite folder hierarchies, soft-deletes, and basic access control natively.

```sql
CREATE TABLE entities (
    id UUID PRIMARY KEY,           -- UUIDv7
    type VARCHAR(15) NOT NULL CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder')),
    visibility VARCHAR(10) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'users', 'public')),
    parent_id UUID REFERENCES entities(id) ON DELETE CASCADE, -- Enables tree hierarchy
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(150) NOT NULL,
    owner_id UUID NOT NULL,        -- Refers to the Users table
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ         -- Soft delete (NULL = active, Timestamp = in Trash)
);

-- Indices for fast filtering, routing, access control, and hierarchy lookup
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_parent ON entities(parent_id);
CREATE INDEX idx_entities_owner ON entities(owner_id);
CREATE INDEX idx_entities_visibility ON entities(visibility);
```
* **`private`**: Only the `owner_id` can access.
* **`users`**: Any authenticated user on the instance can access.
* **`public`**: Accessible to the outside world (e.g., public sharing link).

### 3.2. Subtype Tables
These tables do not generate their own IDs. Their Primary Key is also a Foreign Key pointing exactly to `entities.id`.

```sql
CREATE TABLE articles (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    original_url VARCHAR(2000) NOT NULL,
    domain VARCHAR(100) NOT NULL,  -- e.g., 'github.com' (for easy filtering)
    html_content TEXT,             -- The cleaned Readability HTML
    text_content TEXT              -- Plain text for full-text search
);
-- Note: In v1, generated EPUB files and other binary artifacts are stored as Attachments. WARC archiving is deferred to a later version.

CREATE TABLE notes (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    markdown_content TEXT NOT NULL
);
```

### 3.3. Tagging System
Tags are global and can be applied to any entity via a junction table.

```sql
CREATE TABLE tags (
    id UUID PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    color VARCHAR(7)               -- Hex code for UI, e.g., '#FF5733'
);

CREATE TABLE item_tags (
    item_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    tag_id UUID REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, tag_id)
);
```

## 4. The Knowledge Graph (Backlinks)
Because every object shares the central `entities` table, creating universal backlinks (e.g., a Note linking to a Read-it-Later Article) requires only one simple junction table.

```sql
CREATE TABLE item_links (
    source_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    target_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    context VARCHAR(255),          -- Optional: The text snippet around the link
    PRIMARY KEY (source_id, target_id)
);
```
To find everything that mentions a specific article, the backend simply queries: 
`SELECT source_id FROM item_links WHERE target_id = 'article-uuid';`

## 5. Attachment Management & Garbage Collection
To prevent the system from bloating, binary files (Images, PDFs, generated EPUB files, and future archive artifacts) are stored on disk. The database tracks them using a deduplicated reference system.

### 5.1. Attachments Schema
```sql
CREATE TABLE attachments (
    id UUID PRIMARY KEY,           -- UUIDv7
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(50) NOT NULL,
    size_bytes INTEGER NOT NULL,
    file_hash VARCHAR(64) UNIQUE NOT NULL, -- SHA-256 for deduplication
    disk_path VARCHAR(500) NOT NULL,       -- e.g., '2023/10/hash.epub'
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Junction table linking entities to their attachments
CREATE TABLE item_attachments (
    item_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    attachment_id UUID REFERENCES attachments(id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, attachment_id)
);
```

### 5.2. Garbage Collection (GC) Workflow
When a user empties the trash (hard-deleting a soft-deleted entity), the database cascades and deletes the `item_attachments` relations. A background worker (Garbage Collector) periodically runs the following query to find "orphaned" files on disk:

```sql
-- Find attachments that have ZERO references in the system
SELECT a.id, a.disk_path 
FROM attachments a 
LEFT JOIN item_attachments ia ON a.id = ia.attachment_id 
WHERE ia.item_id IS NULL;
```
The GC process loops through the results, removes the physical file from the disk, and then removes the row from the `attachments` table.

## 6. Smart URL Routing ("The Notion Pattern")
To ensure permalinks never break—even if a user renames a note—Atria uses a hybrid URL structure combining the `slug` and the `UUID`.

**Format:** `<domain>/<type>/<slug>-<uuid>`
**Example:** `atria.local/note/docker-setup-guide-01H7X...`

**Backend Resolution Logic:**
1. The application router captures the full string.
2. It strips everything up to the last hyphen `-` to extract the `UUID`.
3. It queries the `entities` table purely by the `UUID`.
4. *Result:* The link works indefinitely. If the user changes the title, the old URL will still resolve perfectly, effectively ignoring the mismatched slug.
