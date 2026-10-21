-- +goose Up

-- A campaign is one story of a book, told in 3 to 10 cryptograms that open
-- one by one (docs/architecture/cryptogram-campaigns.md).
CREATE TABLE game_campaigns (
    id UUID PRIMARY KEY,
    slug VARCHAR(200) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    book_title VARCHAR(255) NOT NULL,
    book_author VARCHAR(255),
    source_url TEXT,
    license TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    topics TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE game_cryptograms
    ADD COLUMN campaign_id UUID REFERENCES game_campaigns(id) ON DELETE CASCADE,
    ADD COLUMN display_text TEXT,
    ADD COLUMN topics TEXT[] NOT NULL DEFAULT '{}';

CREATE UNIQUE INDEX idx_game_cryptograms_campaign_order
    ON game_cryptograms(campaign_id, order_index) WHERE campaign_id IS NOT NULL;

-- All single puzzles before this migration are quotes.
UPDATE game_cryptograms SET topics = '{quotes}' WHERE campaign_id IS NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_game_cryptograms_campaign_order;
DELETE FROM game_cryptograms WHERE campaign_id IS NOT NULL;
ALTER TABLE game_cryptograms
    DROP COLUMN IF EXISTS topics,
    DROP COLUMN IF EXISTS display_text,
    DROP COLUMN IF EXISTS campaign_id;
DROP TABLE IF EXISTS game_campaigns;
