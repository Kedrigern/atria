-- +goose Up

-- ==========================================
-- NOTES search vector
-- (joins entities.title [A] + notes.markdown_content [B])
-- ==========================================

ALTER TABLE notes ADD COLUMN search_vector tsvector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notes_search_vector_update()
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

CREATE TRIGGER notes_search_vector_trig
BEFORE INSERT OR UPDATE ON notes
FOR EACH ROW EXECUTE FUNCTION notes_search_vector_update();

CREATE INDEX idx_notes_search_vector ON notes USING GIN(search_vector);

-- Populate existing rows
UPDATE notes SET markdown_content = markdown_content;


-- ==========================================
-- ARTICLES search vector
-- (joins entities.title [A] + articles.text_content [B] + articles.user_note [C])
-- ==========================================

ALTER TABLE articles ADD COLUMN search_vector tsvector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION articles_search_vector_update()
RETURNS TRIGGER AS $$
DECLARE
    v_title TEXT;
BEGIN
    SELECT title INTO v_title FROM entities WHERE id = NEW.id;

    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(v_title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.text_content, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.user_note, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER articles_search_vector_trig
BEFORE INSERT OR UPDATE ON articles
FOR EACH ROW EXECUTE FUNCTION articles_search_vector_update();

CREATE INDEX idx_articles_search_vector ON articles USING GIN(search_vector);

-- Populate existing rows
UPDATE articles SET html_content = html_content;


-- ==========================================
-- RSS_ITEMS search vector
-- (title [A] + description [B] + content [C], no entity join needed)
-- ==========================================

ALTER TABLE rss_items ADD COLUMN search_vector tsvector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rss_items_search_vector_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.description, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.content, '')), 'C');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER rss_items_search_vector_trig
BEFORE INSERT OR UPDATE ON rss_items
FOR EACH ROW EXECUTE FUNCTION rss_items_search_vector_update();

CREATE INDEX idx_rss_items_search_vector ON rss_items USING GIN(search_vector);

-- Populate existing rows
UPDATE rss_items SET title = title;


-- +goose Down

-- ==========================================
-- RSS_ITEMS — reverse
-- ==========================================
DROP TRIGGER IF EXISTS rss_items_search_vector_trig ON rss_items;
DROP FUNCTION IF EXISTS rss_items_search_vector_update();
DROP INDEX IF EXISTS idx_rss_items_search_vector;
ALTER TABLE rss_items DROP COLUMN IF EXISTS search_vector;

-- ==========================================
-- ARTICLES — reverse
-- ==========================================
DROP TRIGGER IF EXISTS articles_search_vector_trig ON articles;
DROP FUNCTION IF EXISTS articles_search_vector_update();
DROP INDEX IF EXISTS idx_articles_search_vector;
ALTER TABLE articles DROP COLUMN IF EXISTS search_vector;

-- ==========================================
-- NOTES — reverse
-- ==========================================
DROP TRIGGER IF EXISTS notes_search_vector_trig ON notes;
DROP FUNCTION IF EXISTS notes_search_vector_update();
DROP INDEX IF EXISTS idx_notes_search_vector;
ALTER TABLE notes DROP COLUMN IF EXISTS search_vector;
