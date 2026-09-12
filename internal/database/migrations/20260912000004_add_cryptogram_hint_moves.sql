-- +goose Up

ALTER TABLE game_cryptogram_moves
    ADD COLUMN is_hint BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down

ALTER TABLE game_cryptogram_moves
    DROP COLUMN IF EXISTS is_hint;
