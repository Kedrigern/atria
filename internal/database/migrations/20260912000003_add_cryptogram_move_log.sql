-- +goose Up

ALTER TABLE game_cryptogram_progress
    ADD COLUMN errors_made INTEGER NOT NULL DEFAULT 0 CHECK (errors_made >= 0);

CREATE TABLE game_cryptogram_moves (
    id UUID PRIMARY KEY,
    progress_id UUID NOT NULL REFERENCES game_cryptogram_progress(id) ON DELETE CASCADE,
    code INTEGER NOT NULL CHECK (code > 0),
    letter VARCHAR(1) NOT NULL CHECK (letter ~ '^[A-Z]$'),
    is_correct BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_game_cryptogram_moves_progress_created
    ON game_cryptogram_moves(progress_id, created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS game_cryptogram_moves;
ALTER TABLE game_cryptogram_progress DROP COLUMN IF EXISTS errors_made;
