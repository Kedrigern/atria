package core

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

// EnsurePath traverses the virtual path (e.g., "/home/solar") and creates missing folders.
// It returns the UUID of the final folder, or nil if the path is empty (root).
// Notes and tables share the same folders.
func EnsurePath(ctx context.Context, tx *sql.Tx, ownerID uuid.UUID, path string) (*uuid.UUID, error) {
	cleanPath := strings.Trim(path, "/")
	if cleanPath == "" {
		return nil, nil // Root level
	}

	parts := strings.Split(cleanPath, "/")
	var currentParentID *uuid.UUID

	for _, part := range parts {
		var folderID uuid.UUID
		slug := LimitRunes(strings.ToLower(strings.ReplaceAll(part, " ", "-")), MaxSlugLength)
		folderUUID := NewUUID()

		queryInsert := `
        INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
        ON CONFLICT (owner_id, type, title, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000')) WHERE deleted_at IS NULL
        DO NOTHING
    `
		_, err := tx.ExecContext(ctx, queryInsert,
			folderUUID, currentParentID, ownerID, TypeFolder, VisibilityPrivate, part, slug, time.Now().UTC(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert folder '%s': %w", part, err)
		}

		queryFind := `SELECT id FROM entities WHERE owner_id = $1 AND type = $2 AND title = $3 AND parent_id IS NOT DISTINCT FROM $4 AND deleted_at IS NULL`
		err = tx.QueryRowContext(ctx, queryFind, ownerID, TypeFolder, part, currentParentID).Scan(&folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve folder id for '%s': %w", part, err)
		}

		currentParentID = &folderID
	}

	return currentParentID, nil
}
