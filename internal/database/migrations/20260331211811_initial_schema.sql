-- +goose Up

-- ==========================================
-- 1. USERS
-- ==========================================
CREATE TABLE users (
    id UUID PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(10) DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    preferences JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_login_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 2. ENTITIES (Central polymorphic table)
-- ==========================================
CREATE TABLE entities (
    id UUID PRIMARY KEY,
    short_id VARCHAR(8) GENERATED ALWAYS AS (RIGHT(id::text, 8)) STORED,
    parent_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard')),
    visibility VARCHAR(10) DEFAULT 'private' CHECK (visibility IN ('private', 'users', 'public')),
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(150) NOT NULL,
    path TEXT NOT NULL DEFAULT '/',
    locked_by UUID REFERENCES users(id) ON DELETE SET NULL,
    locked_at TIMESTAMP,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_entities_short_id ON entities(short_id);
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_parent ON entities(parent_id);
CREATE INDEX idx_entities_owner ON entities(owner_id);
-- Prevent duplicit folder creation
CREATE UNIQUE INDEX idx_unique_active_entity
ON entities(owner_id, type, title, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'))
WHERE deleted_at IS NULL;

-- ==========================================
-- 3. ENTITY SUBTYPES
-- ==========================================
CREATE TABLE articles (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    original_url TEXT NOT NULL,
    domain VARCHAR(100) NOT NULL,
    is_archived BOOLEAN DEFAULT FALSE,
    user_note TEXT,
    html_content TEXT,
    text_content TEXT
);


-- MARKDOWN NOTEs
CREATE TABLE notes (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    icon VARCHAR(50),
    markdown_content TEXT NOT NULL
);

-- RSS
CREATE TABLE rss_feeds (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    feed_url TEXT NOT NULL,
    site_url TEXT,
    etag_header VARCHAR(255),
    last_modified_header VARCHAR(255),
    next_fetch_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    -- Diagnostic fields
    last_fetch_at TIMESTAMP,
    last_fetch_status INTEGER,
    last_fetch_error TEXT,
    -- Autentization
    http_auth_type VARCHAR(20) CHECK (http_auth_type IN ('basic', 'bearer')),
    http_auth_username VARCHAR(255),
    http_auth_token TEXT,
    -- Per feed filtering
    filter_rules JSONB DEFAULT '{}'::jsonb
);

CREATE TABLE rss_items (
    id UUID PRIMARY KEY,
    feed_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    description TEXT,
    content TEXT,
    guid TEXT,
    published_at TIMESTAMP NOT NULL,
    read_at TIMESTAMP, -- NULL = unread
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(feed_id, guid)
);

-- ==========================================
-- 4. TAGS
-- ==========================================
CREATE TABLE tags (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(20) NOT NULL CHECK (name ~ '^[a-z0-9\-]+$'),
    description TEXT,
    color VARCHAR(7),
    icon VARCHAR(50),
    is_system BOOLEAN DEFAULT FALSE,
    UNIQUE (owner_id, name)
);

CREATE TABLE rel_entity_tags (
    entity_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    tag_id UUID REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (entity_id, tag_id)
);


-- ==========================================
-- 5. Dashboard
-- ==========================================
CREATE TABLE dashboards (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    layout_data JSONB NOT NULL DEFAULT '{}'::jsonb
);

-- ==========================================
-- 5. KNOWLEDGE GRAPH (Backlinks)
-- ==========================================
CREATE TABLE rel_entity_links (
    source_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    target_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    context VARCHAR(255),
    PRIMARY KEY (source_id, target_id)
);

-- ==========================================
-- 6. ATTACHMENTS
-- ==========================================
CREATE TABLE attachments (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(50) NOT NULL,
    size_bytes INTEGER NOT NULL,
    file_hash VARCHAR(64) NOT NULL,
    disk_path TEXT NOT NULL,
    visibility VARCHAR(10) DEFAULT 'private' CHECK (visibility IN ('private', 'users', 'public')),
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    UNIQUE (owner_id, file_hash)
);

CREATE TABLE rel_entity_attachments (
    entity_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    attachment_id UUID REFERENCES attachments(id) ON DELETE CASCADE,
    PRIMARY KEY (entity_id, attachment_id)
);

-------------------------------------
-- VIEWS
-------------------------------------

CREATE VIEW entity_paths_view AS
WITH RECURSIVE tree AS (
    -- Root entities (without parrent)
    SELECT id, owner_id, type, deleted_at, '/' || slug::text AS full_path
    FROM entities
    WHERE parent_id IS NULL

    UNION ALL

    -- Child entities
    SELECT e.id, e.owner_id, e.type, e.deleted_at, t.full_path || '/' || e.slug
    FROM entities e
    INNER JOIN tree t ON e.parent_id = t.id
)
SELECT * FROM tree;

-- RSS item to view
CREATE VIEW rss_to_read_view AS
SELECT
    i.id,
    e.short_id,
    i.feed_id,
    e.owner_id,
    COALESCE(NULLIF(f.site_url, ''), e.title) AS source_name,
    i.title,
    i.link,
    COALESCE(i.description, '') AS description,
    COALESCE(i.content, '') AS content,
    i.published_at,
    i.created_at
FROM rss_items i
JOIN rss_feeds f ON i.feed_id = f.id
JOIN entities e ON i.feed_id = e.id
WHERE i.read_at IS NULL
  AND e.deleted_at IS NULL
ORDER BY i.published_at DESC;

-- RSS feeds (sources) view
CREATE VIEW rss_feeds_full_view AS
SELECT
    e.id, e.short_id, e.owner_id, e.title, e.slug, e.visibility, e.created_at, e.updated_at, e.deleted_at,
    f.feed_url, f.site_url, f.etag_header, f.last_modified_header,
    f.next_fetch_at, f.last_fetch_at, f.last_fetch_status, f.last_fetch_error,
    f.http_auth_type, f.http_auth_username, f.http_auth_token, f.filter_rules
FROM entities e
JOIN rss_feeds f ON e.id = f.id
WHERE e.deleted_at IS NULL;

-- Clean candidates for automated pruning
CREATE VIEW rss_cleanup_candidates_view AS
SELECT id FROM (
    SELECT id, read_at, ROW_NUMBER() OVER (PARTITION BY feed_id ORDER BY published_at DESC) as item_rank
    FROM rss_items WHERE read_at IS NOT NULL
) ranked WHERE item_rank > 10 AND read_at < NOW() - INTERVAL '60 days';

-- Full Note with metadata
CREATE VIEW notes_full_view AS
SELECT
    e.id, e.short_id, e.parent_id, e.owner_id, e.type, e.visibility, e.title, e.slug,
    e.created_at, e.updated_at, e.deleted_at,
    COALESCE(p.full_path, '/') AS path,  -- Dynamicaly calculate path
    n.icon,
    n.markdown_content
FROM entities e
JOIN notes n ON e.id = n.id
LEFT JOIN entity_paths_view p ON e.parent_id = p.id
WHERE e.deleted_at IS NULL;

-- Full Article with metadata
CREATE VIEW articles_full_view AS
SELECT e.*, a.original_url, a.domain, a.user_note, a.html_content, a.text_content, a.is_archived
FROM entities e JOIN articles a ON e.id = a.id
WHERE e.deleted_at IS NULL;

CREATE VIEW entities_by_tag_view AS
SELECT
    t.name AS tag_name,
    t.owner_id,
    e.id AS entity_id,
    e.type AS entity_type,
    e.title AS entity_title,
    e.created_at
FROM tags t
JOIN rel_entity_tags rel ON t.id = rel.tag_id
JOIN entities e ON rel.entity_id = e.id
WHERE e.deleted_at IS NULL;

-- +goose Down
DROP VIEW IF EXISTS articles_full_view;
DROP VIEW IF EXISTS notes_full_view;
DROP VIEW IF EXISTS rss_cleanup_candidates_view;
DROP VIEW IF EXISTS rss_to_read_view;
DROP VIEW IF EXISTS entity_paths_view;
DROP VIEW IF EXISTS entities_by_tag_view;
DROP TABLE IF EXISTS rel_entity_attachments;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS notes;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS rss_items;
DROP TABLE IF EXISTS rss_feeds;
DROP TABLE IF EXISTS entities;
DROP TABLE IF EXISTS users;
