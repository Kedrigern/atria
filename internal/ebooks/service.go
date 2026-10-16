package ebooks

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/export"

	"github.com/gabriel-vasile/mimetype"
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

// GenerateInput contains the source entities and metadata for an EPUB snapshot.
type GenerateInput struct {
	Title       string
	Description string
	Author      string
	Language    string
	CoverPath   string
	CoverName   string
	Filename    string
	Sources     []core.EntitySummary
}

// ImportInput contains metadata for an existing EPUB uploaded to Atria.
type ImportInput struct {
	Title       string
	Description string
	Author      string
	Language    string
	Path        string
	Filename    string
}

// Detail contains an e-book and the source entities recorded for it.
type Detail struct {
	ID            uuid.UUID
	Title         string
	Description   *string
	Author        *string
	Language      *string
	Filename      string
	CoverDiskPath *string
	GeneratedAt   time.Time
	Sources       []core.EntitySummary
}

// Summary contains the fields displayed by the e-book library and CLI list command.
type Summary struct {
	ID            uuid.UUID
	Title         string
	Description   *string
	Author        *string
	Language      *string
	Filename      string
	CoverDiskPath *string
	SourceCount   int
	GeneratedAt   time.Time
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
		return nil, core.NewUserError("e-book not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get e-book download: %w", err)
	}
	return &download, nil
}

// List returns active e-books owned by a user, newest first.
func List(ctx context.Context, db *sql.DB, ownerID uuid.UUID, author string) ([]Summary, error) {
	query := `
		SELECT e.id, e.title, eb.description, eb.author, eb.language, a.filename, cover.disk_path, eb.generated_at,
		       COUNT(l.target_id) AS source_count
		FROM entities e
		JOIN ebooks eb ON eb.id = e.id
		JOIN attachments a ON a.id = eb.epub_attachment_id
		LEFT JOIN attachments cover ON cover.id = eb.cover_attachment_id
		LEFT JOIN rel_entity_links l ON l.source_id = e.id AND l.context = 'ebook-source'
		WHERE e.owner_id = $1 AND e.type = $2 AND e.deleted_at IS NULL
		  AND ($3 = '' OR eb.author = $3)
		GROUP BY e.id, e.title, eb.description, eb.author, eb.language, a.filename, cover.disk_path, eb.generated_at
		ORDER BY eb.generated_at DESC
	`
	rows, err := db.QueryContext(ctx, query, ownerID, core.TypeEbook, author)
	if err != nil {
		return nil, fmt.Errorf("list e-books: %w", err)
	}
	defer rows.Close()

	var ebooks []Summary
	for rows.Next() {
		var ebook Summary
		if err := rows.Scan(&ebook.ID, &ebook.Title, &ebook.Description, &ebook.Author, &ebook.Language, &ebook.Filename, &ebook.CoverDiskPath, &ebook.GeneratedAt, &ebook.SourceCount); err != nil {
			return nil, fmt.Errorf("scan e-book: %w", err)
		}
		ebooks = append(ebooks, ebook)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate e-books: %w", err)
	}
	return ebooks, nil
}

// Authors returns distinct e-book authors owned by a user.
func Authors(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT author FROM ebooks eb JOIN entities e ON e.id = eb.id WHERE e.owner_id = $1 AND e.deleted_at IS NULL AND author IS NOT NULL AND author <> '' ORDER BY author`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list e-book authors: %w", err)
	}
	defer rows.Close()
	var authors []string
	for rows.Next() {
		var author string
		if err := rows.Scan(&author); err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}
	return authors, rows.Err()
}

// Generate creates an immutable EPUB snapshot, stores its attachments, and creates the e-book entity.
func Generate(ctx context.Context, db *sql.DB, ownerID uuid.UUID, input GenerateInput) (*core.Ebook, error) {
	if len(input.Sources) == 0 {
		return nil, core.NewUserError("at least one source entity is required")
	}

	tempDir, err := os.MkdirTemp("", "atria-ebook-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	filename := ebookFilename(input.Title, input.Filename)
	outputPath := filepath.Join(tempDir, filename)
	if err := export.ExportEbookEPUB(ctx, db, input.Sources, outputPath, export.EbookMetadata{
		Title:              input.Title,
		Description:        input.Description,
		Author:             input.Author,
		Language:           input.Language,
		CoverPath:          input.CoverPath,
		IncludeChapterTags: true,
	}); err != nil {
		return nil, fmt.Errorf("generate EPUB: %w", err)
	}

	epubAttachment, err := attachments.AddAttachment(ctx, db, ownerID, outputPath, filename)
	if err != nil {
		return nil, fmt.Errorf("store EPUB attachment: %w", err)
	}

	var coverAttachmentID *uuid.UUID
	if input.CoverPath != "" {
		coverName := input.CoverName
		if coverName == "" {
			coverName = filepath.Base(input.CoverPath)
		}
		cover, err := attachments.AddAttachment(ctx, db, ownerID, input.CoverPath, coverName)
		if err != nil {
			return nil, fmt.Errorf("store e-book cover: %w", err)
		}
		coverAttachmentID = &cover.ID
	}

	sourceIDs := make([]uuid.UUID, 0, len(input.Sources))
	for _, source := range input.Sources {
		sourceIDs = append(sourceIDs, source.ID)
	}
	return Create(ctx, db, ownerID, CreateInput{
		Title:             input.Title,
		Description:       input.Description,
		Author:            input.Author,
		Language:          input.Language,
		CoverAttachmentID: coverAttachmentID,
		EPUBAttachmentID:  epubAttachment.ID,
		SourceEntityIDs:   sourceIDs,
	})
}

// Import stores an existing EPUB and creates an e-book without source entity links.
func Import(ctx context.Context, db *sql.DB, ownerID uuid.UUID, input ImportInput) (*core.Ebook, error) {
	mimeType, err := mimetype.DetectFile(input.Path)
	if err != nil {
		return nil, fmt.Errorf("detect imported file type: %w", err)
	}
	if mimeType.String() != "application/epub+zip" {
		return nil, core.NewUserError("uploaded file must be an EPUB, got %s", mimeType.String())
	}

	metadata, _ := ReadEPUBMetadata(input.Path)
	if metadata != nil {
		input.Title = firstNonEmpty(input.Title, metadata.Title)
		input.Description = firstNonEmpty(input.Description, metadata.Description)
		input.Author = firstNonEmpty(input.Author, metadata.Author)
		input.Language = firstNonEmpty(input.Language, metadata.Language)
	}
	if strings.TrimSpace(input.Title) == "" {
		input.Title = strings.TrimSuffix(filepath.Base(input.Filename), filepath.Ext(input.Filename))
	}

	attachment, err := attachments.AddAttachment(ctx, db, ownerID, input.Path, input.Filename)
	if err != nil {
		return nil, fmt.Errorf("store imported EPUB: %w", err)
	}

	var coverAttachmentID *uuid.UUID
	cover, err := ExtractEPUBCover(input.Path, filepath.Dir(input.Path))
	if err == nil {
		defer os.Remove(cover.Path)
		coverAttachment, err := attachments.AddAttachment(ctx, db, ownerID, cover.Path, cover.Filename)
		if err != nil {
			return nil, fmt.Errorf("store imported EPUB cover: %w", err)
		}
		coverAttachmentID = &coverAttachment.ID
	}

	return Create(ctx, db, ownerID, CreateInput{
		Title:             input.Title,
		Description:       input.Description,
		Author:            input.Author,
		Language:          input.Language,
		CoverAttachmentID: coverAttachmentID,
		EPUBAttachmentID:  attachment.ID,
	})
}

// GetDetail returns an active e-book and its linked sources.
func GetDetail(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID) (*Detail, error) {
	var detail Detail
	err := db.QueryRowContext(ctx, `
		SELECT e.id, e.title, eb.description, eb.author, eb.language, a.filename, cover.disk_path, eb.generated_at
		FROM entities e
		JOIN ebooks eb ON eb.id = e.id
		JOIN attachments a ON a.id = eb.epub_attachment_id
		LEFT JOIN attachments cover ON cover.id = eb.cover_attachment_id
		WHERE e.id = $1 AND e.owner_id = $2 AND e.type = $3 AND e.deleted_at IS NULL
	`, ebookID, ownerID, core.TypeEbook).Scan(
		&detail.ID, &detail.Title, &detail.Description, &detail.Author, &detail.Language, &detail.Filename, &detail.CoverDiskPath, &detail.GeneratedAt,
	)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("e-book not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get e-book: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.title, e.type
		FROM rel_entity_links l
		JOIN entities e ON e.id = l.target_id
		WHERE l.source_id = $1 AND l.context = 'ebook-source' AND e.deleted_at IS NULL
		ORDER BY e.created_at ASC
	`, ebookID)
	if err != nil {
		return nil, fmt.Errorf("list e-book sources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var source core.EntitySummary
		if err := rows.Scan(&source.ID, &source.Title, &source.Type); err != nil {
			return nil, fmt.Errorf("scan e-book source: %w", err)
		}
		detail.Sources = append(detail.Sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate e-book sources: %w", err)
	}
	return &detail, nil
}

// UpdateMetadata updates editable Atria metadata without changing the EPUB attachment.
func UpdateMetadata(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID, title, description, author, language string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return core.NewUserError("e-book title is required")
	}
	result, err := db.ExecContext(ctx, `
		UPDATE entities e
		SET title = $1, slug = $2, updated_at = CURRENT_TIMESTAMP
		FROM ebooks eb
		WHERE e.id = eb.id AND e.id = $3 AND e.owner_id = $4 AND e.type = $5 AND e.deleted_at IS NULL
	`, title, ebookSlug(title), ebookID, ownerID, core.TypeEbook)
	if err != nil {
		return fmt.Errorf("update e-book title: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return core.NewUserError("e-book not found")
	}
	_, err = db.ExecContext(ctx, `UPDATE ebooks SET description = NULLIF($1, ''), author = NULLIF($2, ''), language = NULLIF($3, '') WHERE id = $4`, description, author, language, ebookID)
	if err != nil {
		return fmt.Errorf("update e-book metadata: %w", err)
	}
	return nil
}

// Delete marks an active e-book owned by the user as deleted.
func Delete(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID) error {
	result, err := db.ExecContext(ctx, `
		UPDATE entities
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND owner_id = $2 AND type = $3 AND deleted_at IS NULL
	`, ebookID, ownerID, core.TypeEbook)
	if err != nil {
		return fmt.Errorf("delete e-book: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm e-book deletion: %w", err)
	}
	if rows == 0 {
		return core.NewUserError("e-book not found")
	}
	return nil
}

// Create creates an e-book entity, links its attachments, and records its source entities.
func Create(ctx context.Context, db *sql.DB, ownerID uuid.UUID, input CreateInput) (*core.Ebook, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, core.NewUserError("e-book title is required")
	}
	if input.EPUBAttachmentID == uuid.Nil {
		return nil, core.NewUserError("EPUB attachment is required")
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
		return core.NewUserError("attachment not found")
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

func ebookFilename(title, filename string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." {
		filename = strings.ToLower(strings.Join(strings.Fields(title), "-"))
		filename = strings.NewReplacer("/", "-", "\\", "-").Replace(filename)
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".epub") {
		filename += ".epub"
	}
	return filename
}

func ebookSlug(title string) string {
	slug := strings.ToLower(strings.Join(strings.Fields(title), "-"))
	slug = strings.NewReplacer("/", "-", "\\", "-").Replace(slug)
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
