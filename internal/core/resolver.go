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

// FindEntities resolves an entity by full UUID, short UUID suffix (last 8 chars), or exact title.
func FindEntities(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityType EntityType, identifier string, includeDeleted bool) ([]EntitySummary, error) {
	var query string
	var args []interface{}

	if parsedID, err := ParseUUID(identifier); err == nil {
		if entityType != "" {
			if includeDeleted {
				query = `SELECT id, title, type FROM entities WHERE id = $1 AND owner_id = $2 AND type = $3`
			} else {
				query = `SELECT id, title, type FROM entities WHERE id = $1 AND owner_id = $2 AND type = $3 AND deleted_at IS NULL`
			}
			args = []interface{}{parsedID, ownerID, entityType}
		} else {
			if includeDeleted {
				query = `SELECT id, title, type FROM entities WHERE id = $1 AND owner_id = $2`
			} else {
				query = `SELECT id, title, type FROM entities WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`
			}
			args = []interface{}{parsedID, ownerID}
		}
	} else {
		if entityType != "" {
			if includeDeleted {
				query = `SELECT id, title, type FROM entities WHERE owner_id = $1 AND type = $2 AND (short_id = $3 OR title = $4)`
			} else {
				query = `SELECT id, title, type FROM entities WHERE owner_id = $1 AND type = $2 AND (short_id = $3 OR title = $4) AND deleted_at IS NULL`
			}
			args = []interface{}{ownerID, entityType, identifier, identifier}
		} else {
			if includeDeleted {
				query = `SELECT id, title, type FROM entities WHERE owner_id = $1 AND (short_id = $2 OR title = $3)`
			} else {
				query = `SELECT id, title, type FROM entities WHERE owner_id = $1 AND (short_id = $2 OR title = $3) AND deleted_at IS NULL`
			}
			args = []interface{}{ownerID, identifier, identifier}
		}
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
