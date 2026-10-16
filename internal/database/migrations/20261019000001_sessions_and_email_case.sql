-- +goose Up

-- Server-side sessions: the cookie carries a random token, the database
-- stores only its SHA-256, so logout (row deleted) really ends the session.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Emails are compared case-insensitively (Authelia may send "Alice@x.cz"
-- for a user created as "alice@x.cz"). Store them normalized and enforce it.
-- +goose StatementBegin
DO $$
DECLARE
    dup TEXT;
BEGIN
    SELECT lower(trim(email)) INTO dup
    FROM users
    GROUP BY lower(trim(email))
    HAVING COUNT(*) > 1
    LIMIT 1;
    IF dup IS NOT NULL THEN
        RAISE EXCEPTION 'cannot normalize user emails: several users differ only in letter case (%). Merge or delete them first.', dup;
    END IF;
END
$$;
-- +goose StatementEnd

UPDATE users SET email = lower(trim(email)) WHERE email <> lower(trim(email));
ALTER TABLE users ADD CONSTRAINT users_email_normalized CHECK (email = lower(trim(email)));

-- +goose Down

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_normalized;
DROP TABLE IF EXISTS sessions;
