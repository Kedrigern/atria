package links

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gofrs/uuid/v5"
)

// AddLink creates a directed relationship between two entities (e.g., Note A mentions Note B).
func AddLink(ctx context.Context, db *sql.DB, sourceID, targetID uuid.UUID, linkContext string) error {
	if sourceID == targetID {
		return fmt.Errorf("an entity cannot link to itself")
	}

	query := `INSERT INTO rel_entity_links (source_id, target_id, context) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, query, sourceID, targetID, linkContext)
	return err
}

// RemoveLink deletes a relationship between two entities.
func RemoveLink(ctx context.Context, db *sql.DB, sourceID, targetID uuid.UUID) error {
	query := `DELETE FROM rel_entity_links WHERE source_id = $1 AND target_id = $2`
	_, err := db.ExecContext(ctx, query, sourceID, targetID)
	return err
}
