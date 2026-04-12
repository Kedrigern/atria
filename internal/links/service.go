package links

import (
	"atria/internal/core"
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

	query := `
		INSERT INTO rel_entity_links (source_id, target_id, context)
		VALUES ($1, $2, $3)
		ON CONFLICT (source_id, target_id) DO NOTHING
	`
	_, err := db.ExecContext(ctx, query, sourceID, targetID, linkContext)
	return err
}

// RemoveLink deletes a relationship between two entities.
func RemoveLink(ctx context.Context, db *sql.DB, sourceID, targetID uuid.UUID) error {
	query := `DELETE FROM rel_entity_links WHERE source_id = $1 AND target_id = $2`
	_, err := db.ExecContext(ctx, query, sourceID, targetID)
	return err
}

// GetEntityLinks returns the outgoing and incoming links for a given entity.
func GetEntityLinks(ctx context.Context, db *sql.DB, entityID uuid.UUID) (outgoing []core.EntitySummary, incoming []core.EntitySummary, err error) {
	queryOut := `
		SELECT e.id, e.title, e.type
		FROM rel_entity_links l JOIN entities e ON l.target_id = e.id
		WHERE l.source_id = $1 AND e.deleted_at IS NULL`

	rowsOut, err := db.QueryContext(ctx, queryOut, entityID)
	if err == nil {
		defer rowsOut.Close()
		for rowsOut.Next() {
			var e core.EntitySummary
			if rowsOut.Scan(&e.ID, &e.Title, &e.Type) == nil {
				outgoing = append(outgoing, e)
			}
		}
	}

	queryIn := `
		SELECT e.id, e.title, e.type
		FROM rel_entity_links l JOIN entities e ON l.source_id = e.id
		WHERE l.target_id = $1 AND e.deleted_at IS NULL`

	rowsIn, err := db.QueryContext(ctx, queryIn, entityID)
	if err == nil {
		defer rowsIn.Close()
		for rowsIn.Next() {
			var e core.EntitySummary
			if rowsIn.Scan(&e.ID, &e.Title, &e.Type) == nil {
				incoming = append(incoming, e)
			}
		}
	}

	return outgoing, incoming, nil
}
