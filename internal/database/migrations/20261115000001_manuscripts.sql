-- +goose Up

-- Manuscripts (entity type 'manuscript'): one Markdown file per book, kept
-- exactly as uploaded. Design: docs/architecture/manuscripts.md §4.
ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard', 'ebook', 'manuscript'));

CREATE TABLE manuscripts (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    author VARCHAR(255),
    language VARCHAR(35),
    description TEXT,
    cover_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    markdown_content TEXT NOT NULL,
    search_vector tsvector
);

CREATE INDEX idx_manuscripts_cover_attachment ON manuscripts(cover_attachment_id);
CREATE INDEX idx_manuscripts_search_vector ON manuscripts USING GIN(search_vector);

-- Same weights as notes: title [A], text [B].
-- +goose StatementBegin
CREATE FUNCTION manuscripts_search_vector_update()
RETURNS TRIGGER AS $$
DECLARE
    v_title TEXT;
BEGIN
    SELECT title INTO v_title FROM entities WHERE id = NEW.id;

    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(v_title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.markdown_content, '')), 'B');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER manuscripts_search_vector_trig
BEFORE INSERT OR UPDATE ON manuscripts
FOR EACH ROW EXECUTE FUNCTION manuscripts_search_vector_update();

-- A rename re-fires the vector trigger of the subtype table.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entities_title_search_refresh()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.type = 'note' THEN
        UPDATE notes SET markdown_content = markdown_content WHERE id = NEW.id;
    ELSIF NEW.type = 'article' THEN
        UPDATE articles SET text_content = text_content WHERE id = NEW.id;
    ELSIF NEW.type = 'manuscript' THEN
        UPDATE manuscripts SET markdown_content = markdown_content WHERE id = NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

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

DROP TABLE IF EXISTS manuscripts;
DROP FUNCTION IF EXISTS manuscripts_search_vector_update();

-- The restored type check cannot hold while manuscript entities exist.
DELETE FROM entities WHERE type = 'manuscript';

ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard', 'ebook'));
