-- +goose Up

CREATE TABLE game_cryptograms (
    id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    category VARCHAR(150),
    order_index INTEGER,
    language VARCHAR(10) NOT NULL DEFAULT 'en' CHECK (language = 'en'),
    plain_text TEXT NOT NULL,
    intro_text TEXT,
    completion_text TEXT,
    initial_revealed_letters VARCHAR(1)[] NOT NULL DEFAULT '{}',
    difficulty_score SMALLINT NOT NULL DEFAULT 3 CHECK (difficulty_score BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (char_length(btrim(plain_text)) > 0),
    CHECK (order_index IS NULL OR order_index >= 0)
);

CREATE INDEX idx_game_cryptograms_category_order ON game_cryptograms(category, order_index);
CREATE INDEX idx_game_cryptograms_difficulty ON game_cryptograms(difficulty_score);

CREATE TABLE game_cryptogram_progress (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cryptogram_id UUID NOT NULL REFERENCES game_cryptograms(id) ON DELETE CASCADE,
    cipher_mapping JSONB NOT NULL,
    guesses JSONB NOT NULL DEFAULT '{}'::jsonb,
    hints_used SMALLINT NOT NULL DEFAULT 0 CHECK (hints_used >= 0),
    moves_count INTEGER NOT NULL DEFAULT 0 CHECK (moves_count >= 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, cryptogram_id),
    CHECK (jsonb_typeof(cipher_mapping) = 'object'),
    CHECK (jsonb_typeof(guesses) = 'object'),
    CHECK (completed_at IS NULL OR completed_at >= started_at)
);

CREATE INDEX idx_game_cryptogram_progress_user_updated ON game_cryptogram_progress(user_id, updated_at DESC);

INSERT INTO game_cryptograms (id, title, category, order_index, plain_text, intro_text, completion_text, difficulty_score) VALUES
('01900000-0000-7000-8000-000000000001', 'Action and intention', 'Benjamin Franklin', 1, 'Well done is better than well said.', 'A short proverb about action and intention.', 'A quote from Benjamin Franklin, American writer, inventor, and statesman.', 2),
('01900000-0000-7000-8000-000000000002', 'Learning and influence', 'Francis Bacon', 2, 'Knowledge is power.', 'A concise statement about learning.', 'A quote from Francis Bacon, English philosopher and statesman.', 1),
('01900000-0000-7000-8000-000000000003', 'A divided community', 'Abraham Lincoln', 3, 'A house divided against itself cannot stand.', 'A well-known metaphor for a divided community.', 'From Abraham Lincoln''s 1858 House Divided speech.', 3),
('01900000-0000-7000-8000-000000000004', 'Enthusiasm', 'Ralph Waldo Emerson', 4, 'Nothing great was ever achieved without enthusiasm.', 'A reminder that commitment matters.', 'A quote from Ralph Waldo Emerson, American essayist and poet.', 3),
('01900000-0000-7000-8000-000000000005', 'Life and roles', 'William Shakespeare', 5, 'All the world''s a stage, and all the men and women merely players.', 'A metaphor about the roles people play in life.', 'From William Shakespeare''s As You Like It, Act II, Scene VII.', 3),
('01900000-0000-7000-8000-000000000006', 'Beginning work', 'Mark Twain', 6, 'The secret of getting ahead is getting started.', 'A practical prompt to begin.', 'Commonly attributed to Mark Twain. Attribution is uncertain.', 3),
('01900000-0000-7000-8000-000000000007', 'Intellectual humility', 'Socrates', 7, 'The only true wisdom is in knowing you know nothing.', 'A paradox about intellectual humility.', 'Commonly attributed to Socrates through Plato''s account of his philosophy.', 3),
('01900000-0000-7000-8000-000000000008', 'Available resources', 'Theodore Roosevelt', 8, 'Do what you can, with what you have, where you are.', 'A compact encouragement to act with the resources available.', 'A quote from Theodore Roosevelt, 26th president of the United States.', 3),
('01900000-0000-7000-8000-000000000009', 'Education and value', 'Benjamin Franklin', 9, 'An investment in knowledge pays the best interest.', 'A quote connecting education and long-term value.', 'A quote from Benjamin Franklin, in Poor Richard''s Almanack.', 3),
('01900000-0000-7000-8000-000000000010', 'The first step', 'Lao Tzu', 10, 'The journey of a thousand miles begins with one step.', 'A proverb about starting difficult work.', 'A saying attributed to Lao Tzu, from the Tao Te Ching. Translation wording varies.', 3);

-- +goose Down

DROP TABLE IF EXISTS game_cryptogram_progress;
DROP TABLE IF EXISTS game_cryptograms;
