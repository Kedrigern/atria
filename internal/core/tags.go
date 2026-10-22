package core

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/gofrs/uuid/v5"
)

var validTagRegex = regexp.MustCompile(`^[a-z0-9\-]+$`)

// CreateTag creates a new tag for a user.
func CreateTag(ctx context.Context, db *sql.DB, ownerID uuid.UUID, name string, isSystem bool) (*Tag, error) {
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	if len(name) > 20 {
		return nil, NewUserError("tag name '%s' exceeds the 20 characters limit", name)
	}
	if !validTagRegex.MatchString(name) {
		return nil, NewUserError("tag name '%s' can only contain lowercase letters, numbers, and hyphens", name)
	}

	tag := &Tag{
		ID:       NewUUID(),
		OwnerID:  ownerID,
		Name:     name,
		IsSystem: isSystem,
	}

	query := `
		INSERT INTO tags (id, owner_id, name, is_system)
		VALUES ($1, $2, $3, $4)
	`
	_, err := db.ExecContext(ctx, query, tag.ID, tag.OwnerID, tag.Name, tag.IsSystem)
	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}
	return tag, nil
}

// FindTagByName finds a tag for a specific user.
func FindTagByName(ctx context.Context, db *sql.DB, ownerID uuid.UUID, name string) (*Tag, error) {
	var t Tag
	query := `SELECT id, owner_id, name, description, color, icon, is_system FROM tags WHERE owner_id = $1 AND name = $2`
	err := db.QueryRowContext(ctx, query, ownerID, name).Scan(
		&t.ID, &t.OwnerID, &t.Name, &t.Description, &t.Color, &t.Icon, &t.IsSystem,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetEntityTags retrieves all tags associated with a specific entity.
func GetEntityTags(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID) ([]Tag, error) {
	query := `
		SELECT t.id, t.owner_id, t.name, t.description, t.color, t.icon, t.is_system
		FROM tags t
		JOIN rel_entity_tags rel ON t.id = rel.tag_id
		WHERE rel.entity_id = $1 AND t.owner_id = $2
	`
	rows, err := db.QueryContext(ctx, query, entityID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Description, &t.Color, &t.Icon, &t.IsSystem)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// AttachTag connects an existing tag to any entity (Note, Article, RSS).
func AttachTag(ctx context.Context, db *sql.DB, entityID, tagID uuid.UUID) error {
	query := `INSERT INTO rel_entity_tags (entity_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, query, entityID, tagID)
	return err
}

// AttachTagByTitle is a helper that finds a tag by name and attaches it to an entity.
func AttachTagByTitle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityID uuid.UUID, tagName string) error {
	tagName = strings.ToLower(strings.ReplaceAll(tagName, " ", "-"))

	tag, err := FindTagByName(ctx, db, ownerID, tagName)
	if err != nil {
		if err == sql.ErrNoRows {
			// Option: Auto-create tag if it doesn't exist
			tag, err = CreateTag(ctx, db, ownerID, tagName, false)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	query := `INSERT INTO rel_entity_tags (entity_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err = db.ExecContext(ctx, query, entityID, tag.ID)
	return err
}

// ListTags retrieves all tags owned by a specific user.
func ListTags(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]Tag, error) {
	query := `
		SELECT t.id, t.owner_id, t.name, t.description, t.color, t.icon, t.is_system,
		       COUNT(e.id) AS usage_count
		FROM tags t
		LEFT JOIN rel_entity_tags rel ON rel.tag_id = t.id
		LEFT JOIN entities e ON e.id = rel.entity_id AND e.owner_id = t.owner_id AND e.deleted_at IS NULL
		WHERE t.owner_id = $1
		GROUP BY t.id, t.owner_id, t.name, t.description, t.color, t.icon, t.is_system
		ORDER BY t.is_system DESC, t.name ASC
	`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		err := rows.Scan(&t.ID, &t.OwnerID, &t.Name, &t.Description, &t.Color, &t.Icon, &t.IsSystem, &t.UsageCount)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tags: %w", err)
	}
	return tags, nil
}

// GetTagEntities returns a list of all entities (articles, notes...) that have the given tag.
func GetTagEntities(ctx context.Context, db *sql.DB, ownerID uuid.UUID, tagName string) ([]EntitySummary, error) {
	query := `
		SELECT entity_id, entity_title, entity_type
		FROM entities_by_tag_view
		WHERE owner_id = $1 AND tag_name = $2
		ORDER BY created_at DESC
	`
	rows, err := db.QueryContext(ctx, query, ownerID, tagName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EntitySummary
	for rows.Next() {
		var e EntitySummary
		if err := rows.Scan(&e.ID, &e.Title, &e.Type); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// TagMerge reports what MergeTags did.
type TagMerge struct {
	// Moved is the number of items that got the kept tag.
	Moved int
	// AlreadyTagged is the number of items that had both tags.
	AlreadyTagged int
}

// MergeTags moves all items of the tag "from" to the tag "into" and deletes
// "from". Both tags belong to the owner. A system tag cannot be merged away.
func MergeTags(ctx context.Context, db *sql.DB, ownerID uuid.UUID, from, into string) (*TagMerge, error) {
	from = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(from), "#"))
	into = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(into), "#"))
	if from == "" || into == "" {
		return nil, NewUserError("select the tag to merge and the tag to keep")
	}
	if from == into {
		return nil, NewUserError("select two different tags")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tag merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	find := func(name string) (uuid.UUID, bool, error) {
		var id uuid.UUID
		var isSystem sql.NullBool
		err := tx.QueryRowContext(ctx, `SELECT id, is_system FROM tags WHERE owner_id = $1 AND name = $2 FOR UPDATE`, ownerID, name).
			Scan(&id, &isSystem)
		if err == sql.ErrNoRows {
			return id, false, NewUserError("tag #%s does not exist", name)
		}
		if err != nil {
			return id, false, fmt.Errorf("find tag %s: %w", name, err)
		}
		return id, isSystem.Bool, nil
	}
	fromID, fromSystem, err := find(from)
	if err != nil {
		return nil, err
	}
	intoID, _, err := find(into)
	if err != nil {
		return nil, err
	}
	if fromSystem {
		return nil, NewUserError("#%s is a system tag; merge another tag into it instead", from)
	}

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rel_entity_tags WHERE tag_id = $1`, fromID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count tagged items: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO rel_entity_tags (entity_id, tag_id)
		SELECT entity_id, $2 FROM rel_entity_tags WHERE tag_id = $1
		ON CONFLICT DO NOTHING
	`, fromID, intoID)
	if err != nil {
		return nil, fmt.Errorf("move tagged items: %w", err)
	}
	moved, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("count moved items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = $1`, fromID); err != nil {
		return nil, fmt.Errorf("delete merged tag: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tag merge: %w", err)
	}
	return &TagMerge{Moved: int(moved), AlreadyTagged: total - int(moved)}, nil
}
