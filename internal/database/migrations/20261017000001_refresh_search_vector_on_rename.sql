-- +goose Up

-- The notes/articles search vectors include entities.title, but their triggers
-- only fire on the subtype tables. Re-fire them when an entity is renamed so
-- the new title is searchable immediately.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entities_title_search_refresh()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.type = 'note' THEN
        UPDATE notes SET markdown_content = markdown_content WHERE id = NEW.id;
    ELSIF NEW.type = 'article' THEN
        UPDATE articles SET text_content = text_content WHERE id = NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER entities_title_search_refresh_trig
AFTER UPDATE OF title ON entities
FOR EACH ROW
WHEN (OLD.title IS DISTINCT FROM NEW.title)
EXECUTE FUNCTION entities_title_search_refresh();

-- Repair vectors that went stale before this trigger existed.
UPDATE notes SET markdown_content = markdown_content;
UPDATE articles SET text_content = text_content;

-- +goose Down

DROP TRIGGER IF EXISTS entities_title_search_refresh_trig ON entities;
DROP FUNCTION IF EXISTS entities_title_search_refresh();
