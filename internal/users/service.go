package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
	"golang.org/x/crypto/bcrypt"
)

// CreateUser hashes the password securely, generates a UUIDv7, and inserts the new user into the database.
func CreateUser(ctx context.Context, db *sql.DB, email, displayName, password string, role core.Role) (*core.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &core.User{
		ID:           core.NewUUID(),
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	query := `
		INSERT INTO users (id, email, display_name, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = db.ExecContext(ctx, query, user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role, user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user into database: %w", err)
	}

	return user, nil
}

// ListUsers retrieves all users from the database.
func ListUsers(ctx context.Context, db *sql.DB) ([]*core.User, error) {
	query := `SELECT id, email, display_name, role, preferences, created_at, last_login_at FROM users ORDER BY created_at ASC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var usersList []*core.User
	for rows.Next() {
		var user core.User
		var lastLogin sql.NullTime
		var prefsBytes []byte

		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &prefsBytes, &user.CreatedAt, &lastLogin); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}

		if lastLogin.Valid {
			user.LastLoginAt = &lastLogin.Time
		}

		user.Preferences = core.DefaultPreferences()
		if len(prefsBytes) > 0 && string(prefsBytes) != "{}" {
			_ = json.Unmarshal(prefsBytes, &user.Preferences) // Ignorujeme err pro stručnost u listu
		}

		usersList = append(usersList, &user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return usersList, nil
}

// UpdateUserRole changes the permission role of an existing user.
func UpdateUserRole(ctx context.Context, db *sql.DB, identifier string, newRole core.Role) error {
	// First, fetch the user to ensure they exist and resolve their exact ID
	user, err := core.FindUser(ctx, db, identifier)
	if err != nil {
		return err
	}

	query := `UPDATE users SET role = $1 WHERE id = $2`
	_, err = db.ExecContext(ctx, query, newRole, user.ID)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	return nil
}

func UpdatePreferences(ctx context.Context, db *sql.DB, userID uuid.UUID, prefs core.UserPreferences) error {
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to marshal user preferences: %w", err)
	}

	// Předpokládáme sloupec `preferences` v tabulce users
	query := `UPDATE users SET preferences = $1 WHERE id = $2`

	_, err = db.ExecContext(ctx, query, string(prefsJSON), userID)
	if err != nil {
		return fmt.Errorf("failed to update user preferences in db: %w", err)
	}

	return nil
}
