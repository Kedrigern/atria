-- +goose Up

-- entity_paths_view built the path of EVERY entity of EVERY user with a
-- recursive CTE; PostgreSQL cannot push id/owner filters into it, so loading
-- one note walked the whole forest. entity_path() walks up from a single
-- entity instead (cost = folder depth) and stops at depth 100, so a corrupt
-- parent cycle cannot loop forever.

-- +goose StatementBegin
CREATE FUNCTION entity_path(start_id UUID) RETURNS TEXT
LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up AS (
        SELECT id, parent_id, slug, 1 AS depth
        FROM entities
        WHERE id = start_id
        UNION ALL
        SELECT e.id, e.parent_id, e.slug, up.depth + 1
        FROM entities e
        JOIN up ON e.id = up.parent_id
        WHERE up.depth < 100
    )
    SELECT '/' || string_agg(slug::text, '/' ORDER BY depth DESC) FROM up
$$;
-- +goose StatementEnd

CREATE OR REPLACE VIEW notes_full_view AS
SELECT
    e.id, e.short_id, e.parent_id, e.owner_id, e.type, e.visibility, e.title, e.slug,
    e.created_at, e.updated_at, e.deleted_at,
    COALESCE(entity_path(e.parent_id), '/') AS path,
    n.icon,
    n.markdown_content
FROM entities e
JOIN notes n ON e.id = n.id
WHERE e.deleted_at IS NULL;

-- The note tree (notes.ListNotes) now builds paths with an owner-scoped
-- recursive query; nothing else used this view.
DROP VIEW notes_tree_view;

DROP VIEW entity_paths_view;

-- +goose Down

CREATE VIEW entity_paths_view AS
WITH RECURSIVE tree AS (
    SELECT id, owner_id, type, deleted_at, '/' || slug::text AS full_path
    FROM entities
    WHERE parent_id IS NULL
    UNION ALL
    SELECT e.id, e.owner_id, e.type, e.deleted_at, t.full_path || '/' || e.slug
    FROM entities e
    INNER JOIN tree t ON e.parent_id = t.id
)
SELECT * FROM tree;

CREATE OR REPLACE VIEW notes_full_view AS
SELECT
    e.id, e.short_id, e.parent_id, e.owner_id, e.type, e.visibility, e.title, e.slug,
    e.created_at, e.updated_at, e.deleted_at,
    COALESCE(p.full_path, '/') AS path,
    n.icon,
    n.markdown_content
FROM entities e
JOIN notes n ON e.id = n.id
LEFT JOIN entity_paths_view p ON e.parent_id = p.id
WHERE e.deleted_at IS NULL;

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

DROP FUNCTION IF EXISTS entity_path(UUID);
