-- +goose Up

-- Number of failed fetches in a row; drives exponential backoff of next_fetch_at.
ALTER TABLE rss_feeds ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE rss_feeds DROP COLUMN IF EXISTS consecutive_failures;
