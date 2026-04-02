package notes

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"

	"atria/internal/core"
)

// EnsurePath traverses the virtual path (e.g., "/home/solar") and creates missing folders.
// It returns the UUID of the final folder, or nil if the path is empty (root).
func EnsurePath(ctx context.Context, tx *sql.Tx, ownerID uuid.UUID, path string) (*uuid.UUID, error) {
	cleanPath := strings.Trim(path, "/")
	if cleanPath == "" {
		return nil, nil // Root level
	}

	parts := strings.Split(cleanPath, "/")
	var currentParentID *uuid.UUID

	for _, part := range parts {
		var folderID uuid.UUID

		// 1. Try to find the folder at the current level
		queryFind := `SELECT id FROM entities WHERE owner_id = $1 AND type = $2 AND title = $3 AND parent_id IS NOT DISTINCT FROM $4`
		err := tx.QueryRowContext(ctx, queryFind, ownerID, core.TypeFolder, part, currentParentID).Scan(&folderID)

		if err == sql.ErrNoRows {
			// 2. Folder does not exist, create it
			folderID = core.NewUUID()
			slug := strings.ToLower(strings.ReplaceAll(part, " ", "-"))

			queryInsert := `
				INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
			`
			_, err = tx.ExecContext(ctx, queryInsert,
				folderID, currentParentID, ownerID, core.TypeFolder, core.VisibilityPrivate, part, slug, time.Now().UTC(),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder '%s': %w", part, err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("failed to query folder '%s': %w", part, err)
		}

		// Move down the tree
		currentParentID = &folderID
	}

	return currentParentID, nil
}

// CreateNote creates a new markdown note, ensuring its virtual path exists.
func CreateNote(ctx context.Context, db *sql.DB, ownerID uuid.UUID, title, path, content string) (*core.Entity, error) {
	// Start a transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even if committed

	// 1. Resolve or create the folder structure
	parentID, err := EnsurePath(ctx, tx, ownerID, path)
	if err != nil {
		return nil, err
	}

	// 2. Create the Entity record
	entityID := core.NewUUID()
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	now := time.Now().UTC()

	entity := &core.Entity{
		ID:         entityID,
		ParentID:   parentID,
		OwnerID:    ownerID,
		Type:       core.TypeNote,
		Visibility: core.VisibilityPrivate,
		Title:      title,
		Slug:       slug,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	queryEntity := `
		INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = tx.ExecContext(ctx, queryEntity,
		entity.ID, entity.ParentID, entity.OwnerID, entity.Type, entity.Visibility, entity.Title, entity.Slug, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert entity: %w", err)
	}

	// 3. Create the Note content record
	queryNote := `INSERT INTO notes (id, markdown_content) VALUES ($1, $2)`
	_, err = tx.ExecContext(ctx, queryNote, entity.ID, content)
	if err != nil {
		return nil, fmt.Errorf("failed to insert note content: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return entity, nil
}

// NoteSummary is a lightweight struct for listing and disambiguation.
type NoteSummary struct {
	ID        uuid.UUID
	Title     string
	Path      string // NOVÉ: Předpočítaná virtuální cesta z databáze
	CreatedAt time.Time
}

// ListNotes retrieves all active (non-deleted) notes for a specific user.
func ListNotes(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]NoteSummary, error) {
	// Přidáno: Filtrování 'deleted_at IS NULL' ve všech úrovních stromu
	query := `
		WITH RECURSIVE folder_tree AS (
			SELECT id, slug::text AS path
			FROM entities
			WHERE owner_id = $1 AND type = 'folder' AND parent_id IS NULL AND deleted_at IS NULL

			UNION ALL

			SELECT e.id, ft.path || '/' || e.slug
			FROM entities e
			INNER JOIN folder_tree ft ON e.parent_id = ft.id
			WHERE e.type = 'folder' AND e.deleted_at IS NULL
		)
		SELECT n.id, n.title, COALESCE('/' || ft.path, '/') AS path, n.created_at
		FROM entities n
		LEFT JOIN folder_tree ft ON n.parent_id = ft.id
		WHERE n.owner_id = $1 AND n.type = 'note' AND n.deleted_at IS NULL
		ORDER BY n.created_at DESC
	`

	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	var notes []NoteSummary
	for rows.Next() {
		var n NoteSummary
		if err := rows.Scan(&n.ID, &n.Title, &n.Path, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// FindNotes resolves an identifier and returns matching notes.
// If includeDeleted is true, it also searches the "trash" (soft-deleted items).
func FindNotes(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string, includeDeleted bool) ([]NoteSummary, error) {
	// Dynamické sestavení podmínek pro smazané položky
	delCond, delCondE, delCondN := "AND deleted_at IS NULL", "AND e.deleted_at IS NULL", "AND n.deleted_at IS NULL"
	if includeDeleted {
		delCond, delCondE, delCondN = "", "", ""
	}

	baseCTE := fmt.Sprintf(`
		WITH RECURSIVE folder_tree AS (
			SELECT id, slug::text AS path
			FROM entities
			WHERE owner_id = $1 AND type = 'folder' AND parent_id IS NULL %s
			UNION ALL
			SELECT e.id, ft.path || '/' || e.slug
			FROM entities e
			INNER JOIN folder_tree ft ON e.parent_id = ft.id
			WHERE e.type = 'folder' %s
		)
	`, delCond, delCondE)

	var query string
	var args []interface{}

	if parsedID, err := uuid.FromString(identifier); err == nil {
		query = baseCTE + fmt.Sprintf(`
			SELECT n.id, n.title, COALESCE('/' || ft.path, '/') AS path, n.created_at
			FROM entities n
			LEFT JOIN folder_tree ft ON n.parent_id = ft.id
			WHERE n.id = $2 AND n.owner_id = $1 AND n.type = 'note' %s
		`, delCondN)
		args = []interface{}{ownerID, parsedID}
	} else {
		// Pozor na '%%' v Sprintf, které se přeloží na jedno '%' pro SQL LIKE operátor
		query = baseCTE + fmt.Sprintf(`
			SELECT n.id, n.title, COALESCE('/' || ft.path, '/') AS path, n.created_at
			FROM entities n
			LEFT JOIN folder_tree ft ON n.parent_id = ft.id
			WHERE n.owner_id = $1 AND n.type = 'note' %s
			AND (n.id::text LIKE $2 || '%%' OR n.title = $3)
		`, delCondN)
		args = []interface{}{ownerID, identifier, identifier}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []NoteSummary
	for rows.Next() {
		var n NoteSummary
		if err := rows.Scan(&n.ID, &n.Title, &n.Path, &n.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	return results, nil
}

// DeleteEntity safely deletes a note or folder. Defaults to soft-delete unless 'hard' is true.
func DeleteEntity(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityID uuid.UUID, recursive bool, hard bool) error {
	var entityType string
	var deletedAt sql.NullTime

	// Odstraněn filtr na 'deleted_at IS NULL', abychom viděli i do koše
	queryType := `SELECT type, deleted_at FROM entities WHERE id = $1 AND owner_id = $2`
	err := db.QueryRowContext(ctx, queryType, entityID, ownerID).Scan(&entityType, &deletedAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("entity not found or you don't have permission")
	}
	if err != nil {
		return fmt.Errorf("failed to verify entity: %w", err)
	}

	if entityType == string(core.TypeFolder) && !recursive {
		return fmt.Errorf("target is a folder. You must use the --recursive flag to delete it and all its contents")
	}

	// Pokud chceme jen soft delete, ale už to smazané je, upozorníme uživatele
	if !hard && deletedAt.Valid {
		return fmt.Errorf("entity is already in the trash. Use --hard to permanently delete it")
	}

	if hard {
		queryHard := `DELETE FROM entities WHERE id = $1 AND owner_id = $2`
		if _, err = db.ExecContext(ctx, queryHard, entityID, ownerID); err != nil {
			return fmt.Errorf("failed to hard delete entity: %w", err)
		}
	} else {
		querySoft := `
			WITH RECURSIVE targets AS (
				SELECT id FROM entities WHERE id = $1 AND owner_id = $2
				UNION ALL
				SELECT e.id FROM entities e INNER JOIN targets t ON e.parent_id = t.id
			)
			UPDATE entities
			SET deleted_at = CURRENT_TIMESTAMP
			WHERE id IN (SELECT id FROM targets) AND deleted_at IS NULL
		`
		if _, err = db.ExecContext(ctx, querySoft, entityID, ownerID); err != nil {
			return fmt.Errorf("failed to soft delete entity: %w", err)
		}
	}

	return nil
}

// GetNoteContent retrieves the raw markdown content of a specific note.
func GetNoteContent(ctx context.Context, db *sql.DB, noteID uuid.UUID) (string, error) {
	var content string
	query := `SELECT markdown_content FROM notes WHERE id = $1`
	err := db.QueryRowContext(ctx, query, noteID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("note content not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch note content: %w", err)
	}
	return content, nil
}
