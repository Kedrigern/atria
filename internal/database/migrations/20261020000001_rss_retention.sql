-- +goose Up

-- When the retention job cleared an item's content/description (title, link
-- and GUID stay so the item is not re-imported as unread).
ALTER TABLE rss_items ADD COLUMN content_pruned_at TIMESTAMPTZ;

-- Supports the daily retention query: read items that still have content.
CREATE INDEX idx_rss_items_prunable ON rss_items(read_at)
    WHERE read_at IS NOT NULL AND content_pruned_at IS NULL;

-- Never used; it described deleting rows, which would make items that are
-- still in the feed come back as unread. Replaced by rss.PruneReadItems.
DROP VIEW IF EXISTS rss_cleanup_candidates_view;

-- +goose Down

CREATE VIEW rss_cleanup_candidates_view AS
SELECT id FROM (
    SELECT id, read_at, ROW_NUMBER() OVER (PARTITION BY feed_id ORDER BY published_at DESC) as item_rank
    FROM rss_items WHERE read_at IS NOT NULL
) ranked WHERE item_rank > 10 AND read_at < NOW() - INTERVAL '60 days';

DROP INDEX IF EXISTS idx_rss_items_prunable;
ALTER TABLE rss_items DROP COLUMN IF EXISTS content_pruned_at;
