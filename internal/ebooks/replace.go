package ebooks

import (
	"context"
	"database/sql"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"atria/internal/attachments"
	"atria/internal/core"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofrs/uuid/v5"
)

// inTx runs fn in a transaction. If fn or the commit fails, it removes the
// files that fn newly wrote, so the storage has no file without a row.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx, files *attachments.NewFiles) error) error {
	files := &attachments.NewFiles{}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin e-book transaction: %w", err)
	}
	if err := fn(tx, files); err != nil {
		_ = tx.Rollback()
		files.Remove()
		return err
	}
	if err := tx.Commit(); err != nil {
		files.Remove()
		return fmt.Errorf("commit e-book: %w", err)
	}
	return nil
}

// stored is the current state of an e-book, locked for an update.
type stored struct {
	metadata     EPUBMetadata
	epubID       uuid.UUID
	epubPath     string
	epubFilename string
	coverID      *uuid.UUID
}

func loadForUpdate(ctx context.Context, tx *sql.Tx, ownerID, ebookID uuid.UUID) (*stored, error) {
	var s stored
	var description, author, language sql.NullString
	var diskPath string
	err := tx.QueryRowContext(ctx, `
		SELECT e.title, eb.description, eb.author, eb.language, eb.epub_attachment_id, a.disk_path, a.filename, eb.cover_attachment_id
		FROM entities e
		JOIN ebooks eb ON eb.id = e.id
		JOIN attachments a ON a.id = eb.epub_attachment_id
		WHERE e.id = $1 AND e.owner_id = $2 AND e.type = $3 AND e.deleted_at IS NULL
		FOR UPDATE OF e, eb
	`, ebookID, ownerID, core.TypeEbook).Scan(&s.metadata.Title, &description, &author, &language, &s.epubID, &diskPath, &s.epubFilename, &s.coverID)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("e-book not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load e-book: %w", err)
	}
	s.metadata.Description, s.metadata.Author, s.metadata.Language = description.String, author.String, language.String
	s.epubPath = attachments.AbsPath(diskPath)
	return &s, nil
}

// saveState writes the metadata and the attachments of an e-book and moves
// the attachment links from the old to the new attachments.
func saveState(ctx context.Context, tx *sql.Tx, ebookID uuid.UUID, old *stored, metadata EPUBMetadata, epubID uuid.UUID, coverID *uuid.UUID) error {
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		return core.NewUserError("e-book title is required")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE entities SET title = $1, slug = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`,
		title, ebookSlug(title), ebookID); err != nil {
		return fmt.Errorf("update e-book title: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE ebooks SET description = NULLIF($1, ''), author = NULLIF($2, ''), language = NULLIF($3, ''),
		       epub_attachment_id = $4, cover_attachment_id = $5
		WHERE id = $6
	`, metadata.Description, metadata.Author, metadata.Language, epubID, coverID, ebookID); err != nil {
		return fmt.Errorf("update e-book metadata: %w", err)
	}
	unlink := []uuid.UUID{}
	if old.epubID != epubID {
		unlink = append(unlink, old.epubID)
	}
	if old.coverID != nil && (coverID == nil || *old.coverID != *coverID) {
		unlink = append(unlink, *old.coverID)
	}
	for _, id := range unlink {
		if _, err := tx.ExecContext(ctx, `DELETE FROM rel_entity_attachments WHERE entity_id = $1 AND attachment_id = $2`, ebookID, id); err != nil {
			return fmt.Errorf("unlink old e-book attachment: %w", err)
		}
	}
	if err := linkAttachment(ctx, tx, ebookID, epubID); err != nil {
		return err
	}
	if coverID != nil {
		return linkAttachment(ctx, tx, ebookID, *coverID)
	}
	return nil
}

// removeUnused deletes the old attachments of an e-book that nothing uses
// any more. A failure here leaves an unused attachment, which the user can
// delete on the Attachments settings tab, so the error is ignored.
func removeUnused(ctx context.Context, db *sql.DB, ownerID uuid.UUID, ids ...uuid.UUID) {
	for _, id := range ids {
		var used bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM ebooks WHERE epub_attachment_id = $1 OR cover_attachment_id = $1)
		`, id).Scan(&used); err != nil || used {
			continue
		}
		_ = attachments.DeleteAttachment(ctx, db, ownerID, id)
	}
}

// storeEPUB stores the EPUB at epubPath and, if coverPath is set, the cover
// image file as attachments in the transaction.
func storeEPUB(ctx context.Context, tx *sql.Tx, files *attachments.NewFiles, ownerID uuid.UUID, epubPath, filename, coverPath, coverName string) (uuid.UUID, *uuid.UUID, error) {
	epub, err := attachments.AddAttachmentTx(ctx, tx, files, ownerID, epubPath, filename)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("store EPUB: %w", err)
	}
	if coverPath == "" {
		return epub.ID, nil, nil
	}
	cover, err := attachments.AddAttachmentTx(ctx, tx, files, ownerID, coverPath, coverName)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("store cover: %w", err)
	}
	return epub.ID, &cover.ID, nil
}

// writeCover writes an encoded cover to dir and returns its path and a file
// name with the right extension.
func writeCover(dir string, cover *Cover, name string) (string, string, error) {
	coverPath := filepath.Join(dir, "cover"+cover.Extension)
	if err := os.WriteFile(coverPath, cover.Data, 0o600); err != nil {
		return "", "", fmt.Errorf("write cover: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if base == "" || base == "." {
		base = "cover"
	}
	return coverPath, base + cover.Extension, nil
}

// UpdateMetadata changes the metadata of an e-book in Atria and in its EPUB
// file, so the downloaded file shows the same title, author and description.
func UpdateMetadata(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID, title, description, author, language string) error {
	metadata := EPUBMetadata{Title: strings.TrimSpace(title), Description: strings.TrimSpace(description),
		Author: strings.TrimSpace(author), Language: strings.TrimSpace(language)}
	if metadata.Title == "" {
		return core.NewUserError("e-book title is required")
	}
	var old *stored
	err := inTx(ctx, db, func(tx *sql.Tx, files *attachments.NewFiles) error {
		var err error
		if old, err = loadForUpdate(ctx, tx, ownerID, ebookID); err != nil {
			return err
		}
		tempDir, err := os.MkdirTemp("", "atria-ebook-*")
		if err != nil {
			return fmt.Errorf("create temporary directory: %w", err)
		}
		defer os.RemoveAll(tempDir)
		rewritten := filepath.Join(tempDir, old.epubFilename)
		if _, err := RewriteEPUB(old.epubPath, rewritten, &metadata, nil); err != nil {
			return fmt.Errorf("write metadata into the EPUB: %w", err)
		}
		epubID, _, err := storeEPUB(ctx, tx, files, ownerID, rewritten, old.epubFilename, "", "")
		if err != nil {
			return err
		}
		return saveState(ctx, tx, ebookID, old, metadata, epubID, old.coverID)
	})
	if err != nil {
		return err
	}
	removeUnused(ctx, db, ownerID, old.epubID)
	return nil
}

// ReplaceCover sets a new cover image for an e-book. The image goes into the
// EPUB file too, so an e-book reader shows the same cover as Atria.
func ReplaceCover(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID, imagePath, imageName string) error {
	img, err := DecodeCover(imagePath)
	if err != nil {
		return err
	}
	var old *stored
	err = inTx(ctx, db, func(tx *sql.Tx, files *attachments.NewFiles) error {
		var err error
		if old, err = loadForUpdate(ctx, tx, ownerID, ebookID); err != nil {
			return err
		}
		return rewriteAndStore(ctx, tx, files, ownerID, ebookID, old, old.epubPath, old.epubFilename, old.metadata, img, imageName)
	})
	if err != nil {
		return err
	}
	unused := []uuid.UUID{old.epubID}
	if old.coverID != nil {
		unused = append(unused, *old.coverID)
	}
	removeUnused(ctx, db, ownerID, unused...)
	return nil
}

// rewriteAndStore writes the EPUB at source with the metadata and the cover
// image, stores it with the cover, and saves the e-book.
func rewriteAndStore(ctx context.Context, tx *sql.Tx, files *attachments.NewFiles, ownerID, ebookID uuid.UUID, old *stored,
	source, filename string, metadata EPUBMetadata, img image.Image, coverName string) error {
	tempDir, err := os.MkdirTemp("", "atria-ebook-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	rewritten := filepath.Join(tempDir, filename)
	cover, err := RewriteEPUB(source, rewritten, &metadata, img)
	if err != nil {
		return fmt.Errorf("write the cover into the EPUB: %w", err)
	}
	coverPath, coverFilename, err := writeCover(tempDir, cover, coverName)
	if err != nil {
		return err
	}
	epubID, coverID, err := storeEPUB(ctx, tx, files, ownerID, rewritten, filename, coverPath, coverFilename)
	if err != nil {
		return err
	}
	return saveState(ctx, tx, ebookID, old, metadata, epubID, coverID)
}

// ReplaceFile replaces the EPUB file of an e-book, for example with a better
// edition. Atria reads the title, author, description, language and cover
// from the new file, as an import does. Where the new file has no value,
// Atria keeps its own value and writes it into the file.
func ReplaceFile(ctx context.Context, db *sql.DB, ownerID, ebookID uuid.UUID, epubPath, filename string) error {
	mimeType, err := mimetype.DetectFile(epubPath)
	if err != nil {
		return fmt.Errorf("detect file type: %w", err)
	}
	if mimeType.String() != "application/epub+zip" {
		return core.NewUserError("the file must be an EPUB, got %s", mimeType.String())
	}
	fileMetadata, err := ReadEPUBMetadata(epubPath)
	if err != nil {
		return core.NewUserError("cannot read the EPUB: %v", err)
	}
	filename = ebookFilename(fileMetadata.Title, filename)

	var old *stored
	err = inTx(ctx, db, func(tx *sql.Tx, files *attachments.NewFiles) error {
		var err error
		if old, err = loadForUpdate(ctx, tx, ownerID, ebookID); err != nil {
			return err
		}
		metadata := EPUBMetadata{
			Title:       firstNonEmpty(fileMetadata.Title, old.metadata.Title),
			Author:      firstNonEmpty(fileMetadata.Author, old.metadata.Author),
			Description: firstNonEmpty(fileMetadata.Description, old.metadata.Description),
			Language:    firstNonEmpty(fileMetadata.Language, old.metadata.Language),
		}

		tempDir, err := os.MkdirTemp("", "atria-ebook-*")
		if err != nil {
			return fmt.Errorf("create temporary directory: %w", err)
		}
		defer os.RemoveAll(tempDir)

		if extracted, err := ExtractEPUBCover(epubPath, tempDir); err == nil {
			// The new file has a cover: keep its bytes as they are.
			rewritten := filepath.Join(tempDir, filename)
			if _, err := RewriteEPUB(epubPath, rewritten, &metadata, nil); err != nil {
				return fmt.Errorf("write metadata into the EPUB: %w", err)
			}
			epubID, coverID, err := storeEPUB(ctx, tx, files, ownerID, rewritten, filename, extracted.Path, extracted.Filename)
			if err != nil {
				return err
			}
			return saveState(ctx, tx, ebookID, old, metadata, epubID, coverID)
		}
		// The new file has no cover: keep the cover of Atria and write it into the file.
		var img image.Image
		if old.coverID != nil {
			var diskPath, name string
			if err := tx.QueryRowContext(ctx, `SELECT disk_path, filename FROM attachments WHERE id = $1`, *old.coverID).Scan(&diskPath, &name); err == nil {
				img, _ = DecodeCover(attachments.AbsPath(diskPath))
			}
		}
		if img == nil {
			rewritten := filepath.Join(tempDir, filename)
			if _, err := RewriteEPUB(epubPath, rewritten, &metadata, nil); err != nil {
				return fmt.Errorf("write metadata into the EPUB: %w", err)
			}
			epubID, _, err := storeEPUB(ctx, tx, files, ownerID, rewritten, filename, "", "")
			if err != nil {
				return err
			}
			return saveState(ctx, tx, ebookID, old, metadata, epubID, nil)
		}
		return rewriteAndStore(ctx, tx, files, ownerID, ebookID, old, epubPath, filename, metadata, img, "cover")
	})
	if err != nil {
		return err
	}
	unused := []uuid.UUID{old.epubID}
	if old.coverID != nil {
		unused = append(unused, *old.coverID)
	}
	removeUnused(ctx, db, ownerID, unused...)
	return nil
}
