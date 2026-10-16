package notes

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"

	"atria/internal/core"
)

// NoteSummary is a lightweight struct for listing and tree views.
type NoteSummary struct {
	ID        uuid.UUID
	Title     string
	Path      string
	CreatedAt time.Time
	CharCount int
	Tags      []string
}

// GetPublicNote loads an active note that is available through a public link.
func GetPublicNote(ctx context.Context, db *sql.DB, id uuid.UUID) (*core.Note, error) {
	query := `
		SELECT id, short_id, parent_id, owner_id, type, visibility, title, slug, path, created_at, updated_at, deleted_at,
		       icon, markdown_content
		FROM notes_full_view
		WHERE id = $1 AND visibility = $2
	`
	var n core.Note
	err := db.QueryRowContext(ctx, query, id, core.VisibilityPublic).Scan(
		&n.ID, &n.ShortID, &n.ParentID, &n.OwnerID, &n.Type, &n.Visibility, &n.Title, &n.Slug, &n.Path, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		&n.Icon, &n.MarkdownContent,
	)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("public note not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public note: %w", err)
	}
	return &n, nil
}

// GetNote loads the full note entity including content and metadata.
func GetNote(ctx context.Context, db *sql.DB, id, ownerID uuid.UUID) (*core.Note, error) {
	query := `
		SELECT id, short_id, parent_id, owner_id, type, visibility, title, slug, path, created_at, updated_at, deleted_at,
		       icon, markdown_content
		FROM notes_full_view
		WHERE id = $1 AND owner_id = $2
	`
	var n core.Note
	err := db.QueryRowContext(ctx, query, id, ownerID).Scan(
		&n.ID, &n.ShortID, &n.ParentID, &n.OwnerID, &n.Type, &n.Visibility, &n.Title, &n.Slug, &n.Path, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		&n.Icon, &n.MarkdownContent,
	)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("note not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch note: %w", err)
	}
	return &n, nil
}

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
		slug := core.LimitRunes(strings.ToLower(strings.ReplaceAll(part, " ", "-")), core.MaxSlugLength)
		folderUUID := core.NewUUID()

		queryInsert := `
        INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
        ON CONFLICT (owner_id, type, title, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000')) WHERE deleted_at IS NULL
        DO NOTHING
    `
		_, err := tx.ExecContext(ctx, queryInsert,
			folderUUID, currentParentID, ownerID, core.TypeFolder, core.VisibilityPrivate, part, slug, time.Now().UTC(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert folder '%s': %w", part, err)
		}

		queryFind := `SELECT id FROM entities WHERE owner_id = $1 AND type = $2 AND title = $3 AND parent_id IS NOT DISTINCT FROM $4 AND deleted_at IS NULL`
		err = tx.QueryRowContext(ctx, queryFind, ownerID, core.TypeFolder, part, currentParentID).Scan(&folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve folder id for '%s': %w", part, err)
		}

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
	defer func() { _ = tx.Rollback() }() // Safe to call even if committed

	// 1. Resolve or create the folder structure
	parentID, err := EnsurePath(ctx, tx, ownerID, path)
	if err != nil {
		return nil, err
	}

	// 2. Create the Entity record
	entityID := core.NewUUID()
	slug := noteSlug(title)
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
	    INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, path, created_at, updated_at)
	    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = tx.ExecContext(ctx, queryEntity, entityID, parentID, ownerID, core.TypeNote, core.VisibilityPrivate, title, slug, path, now, now)
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

// FindNotes resolves an identifier and returns matching notes.
// If includeDeleted is true, it also searches the "trash" (soft-deleted items).
func FindNotes(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string, includeDeleted bool) ([]NoteSummary, error) {
	// Added JOIN with notes to calculate the character count of markdown_content
	query := `
		SELECT n.id, n.title, COALESCE(entity_path(n.parent_id), '/') AS path, n.created_at, COALESCE(LENGTH(nt.markdown_content), 0) AS char_count
		FROM entities n
		JOIN notes nt ON n.id = nt.id
		WHERE n.owner_id = $1 AND n.type = 'note'
	`
	var args []interface{}

	if !includeDeleted {
		query += ` AND n.deleted_at IS NULL`
	}

	if parsedID, err := uuid.FromString(identifier); err == nil {
		query += ` AND n.id = $2`
		args = []interface{}{ownerID, parsedID}
	} else {
		// short_id is the indexed last-8-characters column shown by the CLI;
		// a leading-wildcard LIKE on id::text could not use any index.
		query += ` AND (n.short_id = $2 OR n.title = $2)`
		args = []interface{}{ownerID, identifier}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []NoteSummary
	for rows.Next() {
		var n NoteSummary
		// Added &n.CharCount to match the updated query
		if err := rows.Scan(&n.ID, &n.Title, &n.Path, &n.CreatedAt, &n.CharCount); err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	return results, rows.Err()
}

// ListNotes retrieves notes for the authenticated user, optimized for tree view display.
func ListNotes(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]NoteSummary, error) {
	// Paths are built in one recursive pass over this owner's entities only
	// (calling entity_path per row would walk the tree once per note).
	query := `
		WITH RECURSIVE tree AS (
			SELECT id, '/' || slug::text AS full_path, 1 AS depth
			FROM entities
			WHERE owner_id = $1 AND parent_id IS NULL
			UNION ALL
			SELECT e.id, t.full_path || '/' || e.slug, t.depth + 1
			FROM entities e
			JOIN tree t ON e.parent_id = t.id
			WHERE e.owner_id = $1 AND t.depth < 100
		)
		SELECT e.id, e.title, COALESCE(p.full_path, '/') AS path, e.created_at,
		       COALESCE(LENGTH(n.markdown_content), 0) AS char_count,
		       COALESCE(
		           array_agg(t.name ORDER BY t.name) FILTER (WHERE t.id IS NOT NULL),
		           ARRAY[]::VARCHAR(20)[]
		       ) AS tags
		FROM entities e
		JOIN notes n ON n.id = e.id
		LEFT JOIN tree p ON p.id = e.parent_id
		LEFT JOIN rel_entity_tags ret ON ret.entity_id = e.id
		LEFT JOIN tags t ON t.id = ret.tag_id AND t.owner_id = e.owner_id
		WHERE e.owner_id = $1 AND e.deleted_at IS NULL AND e.type = 'note'
		GROUP BY e.id, e.title, p.full_path, e.created_at, n.markdown_content
		ORDER BY path ASC, e.title ASC
	`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes for tree: %w", err)
	}
	defer rows.Close()

	var list []NoteSummary
	for rows.Next() {
		var n NoteSummary
		if err := rows.Scan(&n.ID, &n.Title, &n.Path, &n.CreatedAt, &n.CharCount, pq.Array(&n.Tags)); err != nil {
			return nil, fmt.Errorf("failed to scan note row for tree: %w", err)
		}
		list = append(list, n)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error in note tree: %w", err)
	}

	return list, nil
}

// DeleteEntity safely deletes a note or folder. Defaults to soft-delete unless 'hard' is true.
func DeleteEntity(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityID uuid.UUID, recursive bool, hard bool) error {
	var entityType string
	var deletedAt sql.NullTime

	// No deleted_at IS NULL filter — we need to see trashed entities too.
	queryType := `SELECT type, deleted_at FROM entities WHERE id = $1 AND owner_id = $2`
	err := db.QueryRowContext(ctx, queryType, entityID, ownerID).Scan(&entityType, &deletedAt)
	if err == sql.ErrNoRows {
		return core.NewUserError("entity not found or you don't have permission")
	}
	if err != nil {
		return fmt.Errorf("failed to verify entity: %w", err)
	}

	if entityType == string(core.TypeFolder) && !recursive {
		return core.NewUserError("target is a folder. You must use the --recursive flag to delete it and all its contents")
	}

	if !hard && deletedAt.Valid {
		return core.NewUserError("entity is already in the trash. Use --hard to permanently delete it")
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
func GetNoteContent(ctx context.Context, db *sql.DB, ownerID, noteID uuid.UUID) (string, error) {
	var content string
	query := `SELECT n.markdown_content FROM notes n JOIN entities e ON e.id = n.id WHERE n.id = $1 AND e.owner_id = $2`
	err := db.QueryRowContext(ctx, query, noteID, ownerID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", core.NewUserError("note content not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch note content: %w", err)
	}
	return content, nil
}

// UpdateNote updates a note's title, folder path, and Markdown content atomically.
func UpdateNote(ctx context.Context, db *sql.DB, ownerID, noteID uuid.UUID, title, path, content string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin note update transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	parentID, err := EnsurePath(ctx, tx, ownerID, path)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE entities
		SET title = $1, slug = $2, path = $3, parent_id = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5 AND owner_id = $6 AND type = $7 AND deleted_at IS NULL
	`, title, noteSlug(title), path, parentID, noteID, ownerID, core.TypeNote)
	if err != nil {
		return fmt.Errorf("failed to update note metadata: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify note update: %w", err)
	}
	if rowsAffected == 0 {
		return core.NewUserError("note not found")
	}

	if _, err := tx.ExecContext(ctx, `UPDATE notes SET markdown_content = $1 WHERE id = $2`, content, noteID); err != nil {
		return fmt.Errorf("failed to update note content: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit note update: %w", err)
	}
	return nil
}

func noteSlug(title string) string {
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, "\\", "-")
	return core.LimitRunes(slug, core.MaxSlugLength)
}
