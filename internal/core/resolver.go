package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofrs/uuid/v5"
)

// EntitySummary is a lightweight struct for listings and disambiguation.
type EntitySummary struct {
	ID    uuid.UUID
	Title string
	Type  EntityType
}

// FindEntities resolves an entity by full UUID, short UUID suffix, or exact title.
func FindEntities(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityType EntityType, identifier string, includeDeleted bool) ([]EntitySummary, error) {
	query := `SELECT id, title, type FROM entities WHERE owner_id = $1`
	args := []interface{}{ownerID}

	if parsedID, err := ParseUUID(identifier); err == nil {
		args = append(args, parsedID)
		query += fmt.Sprintf(` AND id = $%d`, len(args))
	} else {
		args = append(args, identifier, identifier)
		query += fmt.Sprintf(` AND (short_id = $%d OR title = $%d)`, len(args)-1, len(args))
	}

	if entityType != "" {
		args = append(args, entityType)
		query += fmt.Sprintf(` AND type = $%d`, len(args))
	}

	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []EntitySummary
	for rows.Next() {
		var e EntitySummary
		if err := rows.Scan(&e.ID, &e.Title, &e.Type); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, nil
}

// FindUser resolves a user by full UUID or exact email match.
func FindUser(ctx context.Context, db *sql.DB, identifier string) (*User, error) {
	var query string
	var arg interface{}

	// Determine if the identifier is a valid UUID or an email string
	if id, err := ParseUUID(identifier); err == nil {
		query = `SELECT id, email, display_name, role, preferences, password_hash, created_at, last_login_at FROM users WHERE id = $1`
		arg = id
	} else {
		query = `SELECT id, email, display_name, role, preferences, password_hash, created_at, last_login_at FROM users WHERE email = $1`
		arg = identifier
	}

	var user User
	var lastLogin sql.NullTime
	var prefsBytes []byte

	err := db.QueryRowContext(ctx, query, arg).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.Role,
		&prefsBytes, &user.PasswordHash, &user.CreatedAt, &lastLogin,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}

	user.Preferences = DefaultPreferences()
	if len(prefsBytes) > 0 && string(prefsBytes) != "{}" {
		if err := json.Unmarshal(prefsBytes, &user.Preferences); err != nil {
			log.Printf("Warning: failed to unmarshal preferences for user %s: %v", user.ID, err)
		}
	}

	return &user, nil
}
