-- +goose Up

-- A book groups campaigns: the stories of a collection, or the chapters of a
-- novel (docs/architecture/cryptogram-campaigns.md, section 11).
CREATE TABLE game_books (
    id UUID PRIMARY KEY,
    slug VARCHAR(100) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    author VARCHAR(255),
    source_url TEXT,
    license TEXT,
    -- If true, a campaign opens after the campaign before it is solved.
    in_order BOOLEAN NOT NULL DEFAULT FALSE,
    topics TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- One book for each book slug of the existing campaigns ("book/story").
INSERT INTO game_books (id, slug, title, author, source_url, license)
SELECT gen_random_uuid(), split_part(slug, '/', 1), MIN(book_title), MIN(book_author), MIN(source_url), MIN(license)
FROM game_campaigns
GROUP BY split_part(slug, '/', 1);

ALTER TABLE game_campaigns ADD COLUMN book_id UUID REFERENCES game_books(id) ON DELETE CASCADE;
UPDATE game_campaigns c SET book_id = b.id FROM game_books b WHERE b.slug = split_part(c.slug, '/', 1);
ALTER TABLE game_campaigns ALTER COLUMN book_id SET NOT NULL;
CREATE INDEX idx_game_campaigns_book_position ON game_campaigns(book_id, position);

ALTER TABLE game_campaigns
    DROP COLUMN book_title,
    DROP COLUMN book_author,
    DROP COLUMN source_url,
    DROP COLUMN license;

-- +goose Down

ALTER TABLE game_campaigns
    ADD COLUMN book_title VARCHAR(255),
    ADD COLUMN book_author VARCHAR(255),
    ADD COLUMN source_url TEXT,
    ADD COLUMN license TEXT;
UPDATE game_campaigns c SET book_title = b.title, book_author = b.author, source_url = b.source_url, license = b.license
FROM game_books b WHERE b.id = c.book_id;
ALTER TABLE game_campaigns ALTER COLUMN book_title SET NOT NULL;
DROP INDEX IF EXISTS idx_game_campaigns_book_position;
ALTER TABLE game_campaigns DROP COLUMN book_id;
DROP TABLE IF EXISTS game_books;
