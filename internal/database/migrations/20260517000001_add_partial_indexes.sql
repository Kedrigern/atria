-- +goose Up

-- 1. Index for RSS (mainly for rss_to_read_view)
CREATE INDEX idx_rss_items_unread ON rss_items(read_at) WHERE read_at IS NULL;

-- 2. Index for active entity
CREATE INDEX idx_entities_active ON entities(deleted_at) WHERE deleted_at IS NULL;

-- 3. Index for rel_entity_tags (for tag-based entity filtering)
CREATE INDEX idx_rel_entity_tags_tag_id ON rel_entity_tags(tag_id);

-- +goose Down

DROP INDEX IF EXISTS idx_rss_items_unread;
DROP INDEX IF EXISTS idx_entities_active;
DROP INDEX IF EXISTS idx_rel_entity_tags_tag_id;
