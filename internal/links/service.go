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
		return core.NewUserError("an entity cannot link to itself")
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

// GetEntityLinks returns the outgoing and incoming links for an entity owned
// by ownerID. Only entities of the same owner are listed on either side.
func GetEntityLinks(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID) (outgoing []core.EntitySummary, incoming []core.EntitySummary, err error) {
	queryOut := `
		SELECT e.id, e.title, e.type
		FROM rel_entity_links l
		JOIN entities s ON s.id = l.source_id
		JOIN entities e ON e.id = l.target_id
		WHERE l.source_id = $1 AND s.owner_id = $2 AND e.owner_id = $2 AND e.deleted_at IS NULL`
	if outgoing, err = queryLinkedEntities(ctx, db, queryOut, entityID, ownerID); err != nil {
		return nil, nil, fmt.Errorf("load outgoing links: %w", err)
	}

	queryIn := `
		SELECT e.id, e.title, e.type
		FROM rel_entity_links l
		JOIN entities t ON t.id = l.target_id
		JOIN entities e ON e.id = l.source_id
		WHERE l.target_id = $1 AND t.owner_id = $2 AND e.owner_id = $2 AND e.deleted_at IS NULL`
	if incoming, err = queryLinkedEntities(ctx, db, queryIn, entityID, ownerID); err != nil {
		return nil, nil, fmt.Errorf("load incoming links: %w", err)
	}

	return outgoing, incoming, nil
}

func queryLinkedEntities(ctx context.Context, db *sql.DB, query string, args ...any) ([]core.EntitySummary, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []core.EntitySummary
	for rows.Next() {
		var e core.EntitySummary
		if err := rows.Scan(&e.ID, &e.Title, &e.Type); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
