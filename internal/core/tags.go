package core

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gofrs/uuid/v5"
)

// CreateTag creates a new tag for a user.
func CreateTag(ctx context.Context, db *sql.DB, ownerID uuid.UUID, name string, isSystem bool) (*Tag, error) {
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
func GetEntityTags(ctx context.Context, db *sql.DB, entityID uuid.UUID) ([]Tag, error) {
	query := `
		SELECT t.id, t.owner_id, t.name, t.description, t.color, t.icon, t.is_system
		FROM tags t
		JOIN rel_entity_tags rel ON t.id = rel.tag_id
		WHERE rel.entity_id = $1
	`
	rows, err := db.QueryContext(ctx, query, entityID)
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
	return tags, nil
}

// AttachTag connects an existing tag to any entity (Note, Article, RSS).
func AttachTag(ctx context.Context, db *sql.DB, entityID, tagID uuid.UUID) error {
	query := `INSERT INTO rel_entity_tags (entity_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, query, entityID, tagID)
	return err
}

// AttachTagByTitle is a helper that finds a tag by name and attaches it to an entity.
func AttachTagByTitle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityID uuid.UUID, tagName string) error {
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
		SELECT id, owner_id, name, description, color, icon, is_system
		FROM tags
		WHERE owner_id = $1
		ORDER BY is_system DESC, name ASC
	`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
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
	return tags, nil
}
