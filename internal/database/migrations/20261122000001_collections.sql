-- +goose Up

-- Collections (entity type 'collection'): named lists of items, with a manual
-- or an automatic order. Design: docs/architecture/collections.md.
ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard', 'ebook', 'manuscript', 'collection'));

CREATE TABLE collections (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    description TEXT,
    cover_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    sort_order VARCHAR(10) NOT NULL DEFAULT 'manual'
        CHECK (sort_order IN ('manual', 'title', 'author', 'added'))
);

CREATE TABLE collection_items (
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    -- Manual order; gaps let a move change one row.
    position DOUBLE PRECISION NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, entity_id)
);

CREATE INDEX idx_collection_items_entity ON collection_items(entity_id);

-- +goose Down

DROP TABLE IF EXISTS collection_items;
DROP TABLE IF EXISTS collections;

-- The restored type check cannot hold while collection entities exist.
DELETE FROM entities WHERE type = 'collection';

ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard', 'ebook', 'manuscript'));
