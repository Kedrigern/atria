package users

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"atria/internal/core"

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
	query := `SELECT id, email, display_name, role, created_at, last_login_at FROM users ORDER BY created_at ASC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var usersList []*core.User
	for rows.Next() {
		var user core.User
		var lastLogin sql.NullTime

		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt, &lastLogin); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}

		if lastLogin.Valid {
			user.LastLoginAt = &lastLogin.Time
		}
		usersList = append(usersList, &user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return usersList, nil
}

// GetUser retrieves a single user by either UUID or exact email match.
func GetUser(ctx context.Context, db *sql.DB, identifier string) (*core.User, error) {
	var query string
	var arg interface{}

	// Determine if the identifier is a valid UUID or an email string
	if id, err := core.ParseUUID(identifier); err == nil {
		query = `SELECT id, email, display_name, role, created_at, last_login_at FROM users WHERE id = $1`
		arg = id
	} else {
		query = `SELECT id, email, display_name, role, created_at, last_login_at FROM users WHERE email = $1`
		arg = identifier
	}

	var user core.User
	var lastLogin sql.NullTime

	err := db.QueryRowContext(ctx, query, arg).Scan(
		&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.CreatedAt, &lastLogin,
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

	return &user, nil
}

// UpdateUserRole changes the permission role of an existing user.
func UpdateUserRole(ctx context.Context, db *sql.DB, identifier string, newRole core.Role) error {
	// First, fetch the user to ensure they exist and resolve their exact ID
	user, err := GetUser(ctx, db, identifier)
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
