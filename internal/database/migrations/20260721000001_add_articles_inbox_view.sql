-- +goose Up

-- The inbox list is read from this view so the entity, article, and tag joins
-- are evaluated once in PostgreSQL rather than for every application caller.
CREATE VIEW articles_inbox_view AS
SELECT
    e.id,
    e.owner_id,
    e.title,
    a.domain,
    e.created_at,
    COALESCE(LENGTH(a.text_content), 0) AS char_count,
    COALESCE(
        array_agg(t.name ORDER BY t.name) FILTER (WHERE t.id IS NOT NULL),
        ARRAY[]::VARCHAR(20)[]
    ) AS tags
FROM entities e
JOIN articles a ON a.id = e.id
LEFT JOIN rel_entity_tags ret ON ret.entity_id = e.id
LEFT JOIN tags t ON t.id = ret.tag_id AND t.owner_id = e.owner_id
WHERE e.deleted_at IS NULL
  AND a.is_archived = FALSE
GROUP BY e.id, e.owner_id, e.title, a.domain, e.created_at, a.text_content;

-- +goose Down

DROP VIEW IF EXISTS articles_inbox_view;
