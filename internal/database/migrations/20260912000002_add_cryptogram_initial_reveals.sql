-- +goose Up

-- Apply one starting hint to both newly seeded and already imported game catalogs.
UPDATE game_cryptograms
SET initial_revealed_letters = ARRAY['E']::VARCHAR(1)[]
WHERE initial_revealed_letters = '{}';

-- +goose Down

UPDATE game_cryptograms
SET initial_revealed_letters = '{}'
WHERE initial_revealed_letters = ARRAY['E']::VARCHAR(1)[];
