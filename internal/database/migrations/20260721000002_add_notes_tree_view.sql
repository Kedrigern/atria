-- +goose Up

-- The note tree reads its metadata and tag associations from this view.
CREATE VIEW notes_tree_view AS
SELECT
    e.id,
    e.owner_id,
    e.title,
    COALESCE(p.full_path, '/') AS path,
    e.created_at,
    COALESCE(LENGTH(n.markdown_content), 0) AS char_count,
    COALESCE(
        array_agg(t.name ORDER BY t.name) FILTER (WHERE t.id IS NOT NULL),
        ARRAY[]::VARCHAR(20)[]
    ) AS tags
FROM entities e
JOIN notes n ON n.id = e.id
LEFT JOIN entity_paths_view p ON p.id = e.parent_id
LEFT JOIN rel_entity_tags ret ON ret.entity_id = e.id
LEFT JOIN tags t ON t.id = ret.tag_id AND t.owner_id = e.owner_id
WHERE e.deleted_at IS NULL
  AND e.type = 'note'
GROUP BY e.id, e.owner_id, e.title, p.full_path, e.created_at, n.markdown_content;

-- +goose Down

DROP VIEW IF EXISTS notes_tree_view;
