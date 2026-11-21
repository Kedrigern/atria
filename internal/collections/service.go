// Package collections keeps named lists of items with a manual or an
// automatic order. See docs/architecture/collections.md.
package collections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"

	"atria/internal/core"
)

// SortOrder is the order of the items of a collection.
type SortOrder string

const (
	// Manual is the order that the owner sets, for example of a book series.
	Manual   SortOrder = "manual"
	ByTitle  SortOrder = "title"
	ByAuthor SortOrder = "author"
	// ByAdded puts the most recently added items first.
	ByAdded SortOrder = "added"
)

// SortOrders lists the orders for forms, in this order.
var SortOrders = []SortOrder{Manual, ByTitle, ByAuthor, ByAdded}

// ItemTypes are the entity types that a collection can hold. The design
// starts with e-books and manuscripts (docs/architecture/collections.md §9).
var ItemTypes = []core.EntityType{core.TypeEbook, core.TypeManuscript}

// positionGap is the space between the positions of items added at the end.
const positionGap = 1024

// Collection is a collection without its items.
type Collection struct {
	core.Entity
	Description string
	SortOrder   SortOrder
	ItemCount   int
}

// Item is one item of a collection, with what a list or tile shows.
type Item struct {
	ID            uuid.UUID
	Type          core.EntityType
	Title         string
	Author        string
	CoverDiskPath string
	// Number is the place in a manual collection, from 1. It is 0 in a
	// collection with an automatic order.
	Number  int
	AddedAt time.Time
}

// Membership is a collection that an item is in.
type Membership struct {
	CollectionID uuid.UUID
	Title        string
	// Number is the place of the item in a manual collection, or 0.
	Number int
}

func (o SortOrder) valid() bool { return slices.Contains(SortOrders, o) }

func slug(title string) string {
	s := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	s = strings.ReplaceAll(s, "/", "-")
	return core.LimitRunes(s, core.MaxSlugLength)
}

func cleanInput(title string, order SortOrder) (string, error) {
	title = core.LimitRunes(strings.TrimSpace(title), core.MaxTitleLength)
	if title == "" {
		return "", core.NewUserError("collection title is required")
	}
	if !order.valid() {
		return "", core.NewUserError("unknown order %q", order)
	}
	return title, nil
}

// Create makes an empty collection.
func Create(ctx context.Context, db *sql.DB, ownerID uuid.UUID, title, description string, order SortOrder) (*Collection, error) {
	title, err := cleanInput(title, order)
	if err != nil {
		return nil, err
	}
	if err := checkTitleFree(ctx, db, ownerID, uuid.Nil, title); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	id := core.NewUUID()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO entities (id, owner_id, type, visibility, title, slug, path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, '/', $7, $7)`,
		id, ownerID, core.TypeCollection, core.VisibilityPrivate, title, slug(title), now); err != nil {
		return nil, fmt.Errorf("insert collection entity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO collections (id, description, sort_order) VALUES ($1, NULLIF($2, ''), $3)`,
		id, strings.TrimSpace(description), order); err != nil {
		return nil, fmt.Errorf("insert collection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return Get(ctx, db, ownerID, id)
}

func checkTitleFree(ctx context.Context, db *sql.DB, ownerID, except uuid.UUID, title string) error {
	var taken bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM entities WHERE owner_id = $1 AND type = $2 AND title = $3
		               AND deleted_at IS NULL AND id <> $4)`,
		ownerID, core.TypeCollection, title, except).Scan(&taken)
	if err != nil {
		return fmt.Errorf("check collection title: %w", err)
	}
	if taken {
		return core.NewUserError("you already have a collection named %q", title)
	}
	return nil
}

const collectionSelect = `
	SELECT e.id, e.short_id, e.owner_id, e.visibility, e.title, e.slug, e.created_at, e.updated_at,
	       COALESCE(c.description, ''), c.sort_order,
	       (SELECT COUNT(*) FROM collection_items ci JOIN entities ie ON ie.id = ci.entity_id
	        WHERE ci.collection_id = c.id AND ie.deleted_at IS NULL)
	FROM collections c JOIN entities e ON e.id = c.id`

func scanCollection(scan func(...any) error) (*Collection, error) {
	var c Collection
	if err := scan(&c.ID, &c.ShortID, &c.OwnerID, &c.Visibility, &c.Title, &c.Slug, &c.CreatedAt, &c.UpdatedAt,
		&c.Description, &c.SortOrder, &c.ItemCount); err != nil {
		return nil, err
	}
	c.Type = core.TypeCollection
	c.Path = "/"
	return &c, nil
}

// Get loads a collection of the owner.
func Get(ctx context.Context, db *sql.DB, ownerID, id uuid.UUID) (*Collection, error) {
	c, err := scanCollection(db.QueryRowContext(ctx, collectionSelect+`
		WHERE e.id = $1 AND e.owner_id = $2 AND e.deleted_at IS NULL`, id, ownerID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewUserError("collection not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load collection: %w", err)
	}
	return c, nil
}

// List returns the owner's collections by title.
func List(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]Collection, error) {
	rows, err := db.QueryContext(ctx, collectionSelect+`
		WHERE e.owner_id = $1 AND e.deleted_at IS NULL ORDER BY lower(e.title)`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()
	var list []Collection
	for rows.Next() {
		c, err := scanCollection(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

// Update changes the title, the description and the order. A collection that
// becomes manual starts with the order it had before.
func Update(ctx context.Context, db *sql.DB, ownerID, id uuid.UUID, title, description string, order SortOrder) error {
	title, err := cleanInput(title, order)
	if err != nil {
		return err
	}
	old, err := Get(ctx, db, ownerID, id)
	if err != nil {
		return err
	}
	if err := checkTitleFree(ctx, db, ownerID, id, title); err != nil {
		return err
	}
	var items []Item
	if order == Manual && old.SortOrder != Manual {
		if items, err = Items(ctx, db, ownerID, id); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE entities SET title = $1, slug = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND owner_id = $4`, title, slug(title), id, ownerID); err != nil {
		return fmt.Errorf("update collection entity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collections SET description = NULLIF($1, ''), sort_order = $2 WHERE id = $3`,
		strings.TrimSpace(description), order, id); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}
	for i, item := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE collection_items SET position = $1 WHERE collection_id = $2 AND entity_id = $3`,
			float64((i+1)*positionGap), id, item.ID); err != nil {
			return fmt.Errorf("renumber collection: %w", err)
		}
	}
	return tx.Commit()
}

// Add puts an item of the owner at the end of the collection. An item that is
// already in it stays where it is.
func Add(ctx context.Context, db *sql.DB, ownerID, collectionID, entityID uuid.UUID) error {
	if _, err := Get(ctx, db, ownerID, collectionID); err != nil {
		return err
	}
	var itemType core.EntityType
	err := db.QueryRowContext(ctx, `SELECT type FROM entities WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`,
		entityID, ownerID).Scan(&itemType)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewUserError("item not found")
	}
	if err != nil {
		return fmt.Errorf("load item: %w", err)
	}
	if !slices.Contains(ItemTypes, itemType) {
		return core.NewUserError("a collection can hold e-books and manuscripts, not a %s", itemType)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO collection_items (collection_id, entity_id, position)
		SELECT $1, $2, COALESCE(MAX(position), 0) + $3 FROM collection_items WHERE collection_id = $1
		ON CONFLICT (collection_id, entity_id) DO NOTHING`, collectionID, entityID, positionGap)
	if err != nil {
		return fmt.Errorf("add to collection: %w", err)
	}
	return nil
}

// Remove takes an item out of the collection. The item itself stays.
func Remove(ctx context.Context, db *sql.DB, ownerID, collectionID, entityID uuid.UUID) error {
	if _, err := Get(ctx, db, ownerID, collectionID); err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `DELETE FROM collection_items WHERE collection_id = $1 AND entity_id = $2`, collectionID, entityID)
	if err != nil {
		return fmt.Errorf("remove from collection: %w", err)
	}
	return core.RequireAffected(result, "the item is not in the collection")
}

// Move moves an item of a manual collection one place up (-1) or down (+1).
func Move(ctx context.Context, db *sql.DB, ownerID, collectionID, entityID uuid.UUID, step int) error {
	c, err := Get(ctx, db, ownerID, collectionID)
	if err != nil {
		return err
	}
	if c.SortOrder != Manual {
		return core.NewUserError("only a collection with a manual order can move items")
	}
	if step != -1 && step != 1 {
		return core.NewUserError("an item moves one place up or down")
	}
	items, err := Items(ctx, db, ownerID, collectionID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(items, func(it Item) bool { return it.ID == entityID })
	if i < 0 {
		return core.NewUserError("the item is not in the collection")
	}
	j := i + step
	if j < 0 || j >= len(items) {
		return nil // already first or last
	}
	// Swap the two positions in one statement.
	_, err = db.ExecContext(ctx, `
		UPDATE collection_items ci SET position = other.position
		FROM collection_items other
		WHERE ci.collection_id = $1 AND other.collection_id = $1
		  AND ((ci.entity_id = $2 AND other.entity_id = $3) OR (ci.entity_id = $3 AND other.entity_id = $2))`,
		collectionID, items[i].ID, items[j].ID)
	if err != nil {
		return fmt.Errorf("move in collection: %w", err)
	}
	return nil
}

// itemOrder maps a sort order to SQL; only these fixed strings reach the query.
var itemOrder = map[SortOrder]string{
	Manual:   "ci.position",
	ByTitle:  "lower(e.title)",
	ByAuthor: "lower(NULLIF(COALESCE(eb.author, m.author), '')) NULLS LAST, lower(e.title)",
	ByAdded:  "ci.added_at DESC",
}

// Items returns the items of a collection that are not in the trash, in the
// order of the collection.
func Items(ctx context.Context, db *sql.DB, ownerID, collectionID uuid.UUID) ([]Item, error) {
	c, err := Get(ctx, db, ownerID, collectionID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.type, e.title, COALESCE(eb.author, m.author, ''), COALESCE(cover.disk_path, ''), ci.added_at
		FROM collection_items ci
		JOIN entities e ON e.id = ci.entity_id
		LEFT JOIN ebooks eb ON eb.id = e.id
		LEFT JOIN manuscripts m ON m.id = e.id
		LEFT JOIN attachments cover ON cover.id = eb.cover_attachment_id
		WHERE ci.collection_id = $1 AND e.deleted_at IS NULL
		ORDER BY `+itemOrder[c.SortOrder], collectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection items: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Type, &it.Title, &it.Author, &it.CoverDiskPath, &it.AddedAt); err != nil {
			return nil, err
		}
		if c.SortOrder == Manual {
			it.Number = len(items) + 1
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Memberships returns, for each of the given items, the owner's collections
// that hold it, by collection title.
func Memberships(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityIDs []uuid.UUID) (map[uuid.UUID][]Membership, error) {
	result := map[uuid.UUID][]Membership{}
	if len(entityIDs) == 0 {
		return result, nil
	}
	ids := make([]string, len(entityIDs))
	for i, id := range entityIDs {
		ids[i] = id.String()
	}
	rows, err := db.QueryContext(ctx, `
		SELECT x.entity_id, x.collection_id, x.title, CASE WHEN x.sort_order = 'manual' THEN x.number ELSE 0 END
		FROM (
			SELECT ci.entity_id, c.id AS collection_id, ce.title, c.sort_order,
			       row_number() OVER (PARTITION BY ci.collection_id ORDER BY ci.position) AS number
			FROM collection_items ci
			JOIN collections c ON c.id = ci.collection_id
			JOIN entities ce ON ce.id = c.id
			JOIN entities ie ON ie.id = ci.entity_id
			WHERE ce.owner_id = $1 AND ce.deleted_at IS NULL AND ie.deleted_at IS NULL
		) x
		WHERE x.entity_id = ANY($2::uuid[])
		ORDER BY lower(x.title)`, ownerID, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entityID uuid.UUID
		var m Membership
		if err := rows.Scan(&entityID, &m.CollectionID, &m.Title, &m.Number); err != nil {
			return nil, err
		}
		result[entityID] = append(result[entityID], m)
	}
	return result, rows.Err()
}
