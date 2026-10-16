-- +goose Up

ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard', 'ebook'));

CREATE TABLE ebooks (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    description TEXT,
    author VARCHAR(255),
    language VARCHAR(35),
    cover_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    epub_attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    generated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ebooks_cover_attachment ON ebooks(cover_attachment_id);
CREATE INDEX idx_ebooks_epub_attachment ON ebooks(epub_attachment_id);

-- +goose Down

DROP TABLE IF EXISTS ebooks;

-- The restored type check cannot hold while ebook entities exist.
DELETE FROM entities WHERE type = 'ebook';

ALTER TABLE entities DROP CONSTRAINT IF EXISTS entities_type_check;
ALTER TABLE entities ADD CONSTRAINT entities_type_check
    CHECK (type IN ('note', 'article', 'rss', 'spreadsheet', 'folder', 'dashboard'));
