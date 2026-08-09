package ebooks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
)

// CreateInput contains the metadata and existing attachments for a generated e-book.
type CreateInput struct {
	Title             string
	Description       string
	Author            string
	Language          string
	CoverAttachmentID *uuid.UUID
	EPUBAttachmentID  uuid.UUID
	SourceEntityIDs   []uuid.UUID
}

// Summary contains the fields displayed by the e-book library and CLI list command.
type Summary struct {
	ID          uuid.UUID
	Title       string
	Author      *string
	Language    *string
	Filename    string
	SourceCount int
	GeneratedAt time.Time
}

// Download contains the stored EPUB file information for an e-book.
type Download struct {
	Filename string
	DiskPath string
}

// GetDownload returns the EPUB attachment for an active e-book owned by a user.
func GetDownload(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID) (*Download, error) {
	var download Download
	err := db.QueryRowContext(ctx, `
		SELECT a.filename, a.disk_path
		FROM entities e
		JOIN ebooks eb ON eb.id = e.id
		JOIN attachments a ON a.id = eb.epub_attachment_id
		WHERE e.id = $1 AND e.owner_id = $2 AND e.type = $3 AND e.deleted_at IS NULL
	`, ebookID, ownerID, core.TypeEbook).Scan(&download.Filename, &download.DiskPath)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("e-book not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get e-book download: %w", err)
	}
	return &download, nil
}

// List returns active e-books owned by a user, newest first.
func List(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]Summary, error) {
	query := `
		SELECT e.id, e.title, eb.author, eb.language, a.filename, eb.generated_at,
		       COUNT(l.target_id) AS source_count
		FROM entities e
		JOIN ebooks eb ON eb.id = e.id
		JOIN attachments a ON a.id = eb.epub_attachment_id
		LEFT JOIN rel_entity_links l ON l.source_id = e.id AND l.context = 'ebook-source'
		WHERE e.owner_id = $1 AND e.type = $2 AND e.deleted_at IS NULL
		GROUP BY e.id, e.title, eb.author, eb.language, a.filename, eb.generated_at
		ORDER BY eb.generated_at DESC
	`
	rows, err := db.QueryContext(ctx, query, ownerID, core.TypeEbook)
	if err != nil {
		return nil, fmt.Errorf("list e-books: %w", err)
	}
	defer rows.Close()

	var ebooks []Summary
	for rows.Next() {
		var ebook Summary
		if err := rows.Scan(&ebook.ID, &ebook.Title, &ebook.Author, &ebook.Language, &ebook.Filename, &ebook.GeneratedAt, &ebook.SourceCount); err != nil {
			return nil, fmt.Errorf("scan e-book: %w", err)
		}
		ebooks = append(ebooks, ebook)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate e-books: %w", err)
	}
	return ebooks, nil
}

// Create creates an e-book entity, links its attachments, and records its source entities.
func Create(ctx context.Context, db *sql.DB, ownerID uuid.UUID, input CreateInput) (*core.Ebook, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("e-book title is required")
	}
	if input.EPUBAttachmentID == uuid.Nil {
		return nil, fmt.Errorf("EPUB attachment is required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin e-book transaction: %w", err)
	}
	defer tx.Rollback()

	if err := verifyAttachmentOwner(ctx, tx, ownerID, input.EPUBAttachmentID); err != nil {
		return nil, fmt.Errorf("EPUB attachment: %w", err)
	}
	if input.CoverAttachmentID != nil {
		if err := verifyAttachmentOwner(ctx, tx, ownerID, *input.CoverAttachmentID); err != nil {
			return nil, fmt.Errorf("cover attachment: %w", err)
		}
	}

	now := time.Now().UTC()
	ebookID := core.NewUUID()
	slug := ebookSlug(title)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO entities (id, owner_id, type, visibility, title, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
	`, ebookID, ownerID, core.TypeEbook, core.VisibilityPrivate, title, slug, now)
	if err != nil {
		return nil, fmt.Errorf("create e-book entity: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ebooks (id, description, author, language, cover_attachment_id, epub_attachment_id, generated_at)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7)
	`, ebookID, input.Description, input.Author, input.Language, input.CoverAttachmentID, input.EPUBAttachmentID, now)
	if err != nil {
		return nil, fmt.Errorf("create e-book metadata: %w", err)
	}

	if err := linkAttachment(ctx, tx, ebookID, input.EPUBAttachmentID); err != nil {
		return nil, err
	}
	if input.CoverAttachmentID != nil {
		if err := linkAttachment(ctx, tx, ebookID, *input.CoverAttachmentID); err != nil {
			return nil, err
		}
	}
	for _, sourceID := range input.SourceEntityIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rel_entity_links (source_id, target_id, context)
			SELECT $1, id, 'ebook-source'
			FROM entities
			WHERE id = $2 AND owner_id = $3 AND type IN ($4, $5) AND deleted_at IS NULL
			ON CONFLICT (source_id, target_id) DO NOTHING
		`, ebookID, sourceID, ownerID, core.TypeArticle, core.TypeNote); err != nil {
			return nil, fmt.Errorf("link e-book source: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit e-book: %w", err)
	}

	return &core.Ebook{
		Entity: core.Entity{
			ID:         ebookID,
			OwnerID:    ownerID,
			Type:       core.TypeEbook,
			Visibility: core.VisibilityPrivate,
			Title:      title,
			Slug:       slug,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		ID:                ebookID,
		Description:       stringPointer(input.Description),
		Author:            stringPointer(input.Author),
		Language:          stringPointer(input.Language),
		CoverAttachmentID: input.CoverAttachmentID,
		EPUBAttachmentID:  input.EPUBAttachmentID,
		GeneratedAt:       now,
	}, nil
}

func verifyAttachmentOwner(ctx context.Context, tx *sql.Tx, ownerID, attachmentID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM attachments WHERE id = $1 AND owner_id = $2)`, attachmentID, ownerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("attachment not found")
	}
	return nil
}

func linkAttachment(ctx context.Context, tx *sql.Tx, ebookID, attachmentID uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rel_entity_attachments (entity_id, attachment_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, ebookID, attachmentID); err != nil {
		return fmt.Errorf("link e-book attachment: %w", err)
	}
	return nil
}

func ebookSlug(title string) string {
	slug := strings.ToLower(strings.Join(strings.Fields(title), "-"))
	slug = strings.NewReplacer("/", "-", "\\", "-").Replace(slug)
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
