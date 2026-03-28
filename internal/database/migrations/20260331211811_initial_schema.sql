-- +goose Up

-- ==========================================
-- 1. USERS
-- ==========================================
CREATE TABLE users (
    id UUID PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(10) DEFAULT 'user' CHECK (role IN ('user', 'admin')), -- PŘIDÁNO
    last_login_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 2. ENTITIES (Central polymorphic table)
-- ==========================================
CREATE TABLE entities (
    id UUID PRIMARY KEY,
    parent_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder')),
    visibility VARCHAR(10) DEFAULT 'private' CHECK (visibility IN ('private', 'users', 'public')),
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(150) NOT NULL,
    locked_by UUID REFERENCES users(id) ON DELETE SET NULL,
    locked_at TIMESTAMP,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_parent ON entities(parent_id);
CREATE INDEX idx_entities_owner ON entities(owner_id);

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

CREATE TABLE notes (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    icon VARCHAR(50),
    markdown_content TEXT NOT NULL
);

CREATE TABLE rss_feeds (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    feed_url TEXT NOT NULL,
    site_url TEXT,
    description TEXT,
    etag_header VARCHAR(255),
    last_modified_header VARCHAR(255),
    error_count INTEGER DEFAULT 0,
    last_fetched_at TIMESTAMP,
    next_fetch_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

-- +goose Down
DROP TABLE IF EXISTS rel_entity_attachments;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS rel_entity_links;
DROP TABLE IF EXISTS rel_entity_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS rss_feeds;
DROP TABLE IF EXISTS notes;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS entities;
DROP TABLE IF EXISTS users;
