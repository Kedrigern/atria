package core

import (
	"context"
	"database/sql"

	"github.com/gofrs/uuid/v5"
)

// SoftDeleteEntity
func SoftDeleteEntity(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID) error {
	query := `UPDATE entities SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1 AND owner_id = $2`
	_, err := db.ExecContext(ctx, query, entityID, ownerID)
	return err
}
