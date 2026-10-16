package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/uuid/v5"
)

// ErrSessionNotFound means the token is unknown, expired or revoked.
var ErrSessionNotFound = errors.New("session not found")

func hashSessionToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CreateSession starts a session for userID and returns the random token to
// put in the cookie. Only the token's hash is stored, so a database leak does
// not hand out valid sessions. It also records the login time and removes
// expired sessions.
func CreateSession(ctx context.Context, db *sql.DB, userID uuid.UUID, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	_, err := db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, NOW() + make_interval(secs => $3))`,
		hashSessionToken(token), userID, ttl.Seconds())
	if err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}

	// Best effort housekeeping; a failure here must not block the login.
	_, _ = db.ExecContext(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	_, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)

	return token, nil
}

// SessionUserID returns the user of an active session.
func SessionUserID(ctx context.Context, db *sql.DB, token string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := db.QueryRowContext(ctx,
		`SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > NOW()`,
		hashSessionToken(token)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrSessionNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("look up session: %w", err)
	}
	return userID, nil
}

// DeleteSession revokes one session (logout).
func DeleteSession(ctx context.Context, db *sql.DB, token string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashSessionToken(token))
	return err
}

// DeleteUserSessions revokes every session of a user (e.g. "log out everywhere").
func DeleteUserSessions(ctx context.Context, db *sql.DB, userID uuid.UUID) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}
