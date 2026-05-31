package core

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gofrs/uuid/v5"
)

// SoftDeleteEntity marks an entity as deleted without removing it from the database.
func SoftDeleteEntity(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID) error {
	query := `UPDATE entities SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1 AND owner_id = $2`
	_, err := db.ExecContext(ctx, query, entityID, ownerID)
	return err
}

// UpdateVisibility sets the visibility of any entity owned by ownerID.
func UpdateVisibility(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID, visibility Visibility) error {
	query := `UPDATE entities SET visibility = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND owner_id = $3`
	_, err := db.ExecContext(ctx, query, visibility, entityID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to update visibility: %w", err)
	}
	return nil
}

// VerifyOwner returns the owner UUID for the given entity, or an error if not found.
func VerifyOwner(ctx context.Context, db *sql.DB, entityID uuid.UUID) (uuid.UUID, error) {
	var ownerID uuid.UUID
	err := db.QueryRowContext(ctx,
		`SELECT owner_id FROM entities WHERE id = $1 AND deleted_at IS NULL`,
		entityID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return uuid.Nil, fmt.Errorf("entity not found")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to verify owner: %w", err)
	}
	return ownerID, nil
}

// RenameEntity updates the title (and recalculates slug) for any entity.
func RenameEntity(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID, newTitle string) error {
	// Simple slug generation
	slug := strings.ToLower(strings.ReplaceAll(newTitle, " ", "-"))
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, "\\", "-")
	if len(slug) > 100 {
		slug = slug[:100]
	}

	query := `
		UPDATE entities
		SET title = $1, slug = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND owner_id = $4
	`
	_, err := db.ExecContext(ctx, query, newTitle, slug, entityID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to rename entity: %w", err)
	}
	return nil
}
