package attachments

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"atria/internal/core"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofrs/uuid/v5"
)

// AddAttachment save local file into Atria storage
// Ensure deduplication by hash
func AddAttachment(ctx context.Context, db *sql.DB, ownerID uuid.UUID, localPath string, originalFilename string) (*core.Attachment, error) {
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments" // Fallback default value
	}

	srcFile, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, srcFile)
	if err != nil {
		return nil, fmt.Errorf("failed to hash file: %w", err)
	}
	fileHash := hex.EncodeToString(hash.Sum(nil))

	var existing core.Attachment
	queryCheck := `SELECT id, owner_id, filename, mime_type, size_bytes, file_hash, disk_path, created_at FROM attachments WHERE owner_id = $1 AND file_hash = $2`
	err = db.QueryRowContext(ctx, queryCheck, ownerID, fileHash).Scan(
		&existing.ID, &existing.OwnerID, &existing.Filename, &existing.MimeType,
		&existing.SizeBytes, &existing.FileHash, &existing.DiskPath, &existing.CreatedAt,
	)
	if err == nil {
		return &existing, nil
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("database error during deduplication check: %w", err)
	}

	mtype, err := mimetype.DetectFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect mime type: %w", err)
	}

	now := time.Now().UTC()
	yearMonth := now.Format("2006/01")
	extension := filepath.Ext(localPath)
	if extension == "" {
		extension = mtype.Extension() // Fallback to detected extension
	}

	relDiskPath := filepath.Join(yearMonth, fileHash+extension)
	absDiskPath := filepath.Join(storagePath, relDiskPath)

	if err := os.MkdirAll(filepath.Dir(absDiskPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directories: %w", err)
	}

	if _, err := srcFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to seek file: %w", err)
	}

	destDir := filepath.Dir(absDiskPath)
	tempFile, err := os.CreateTemp(destDir, "upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, srcFile); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("failed to write to temp storage: %w", err)
	}

	tempFile.Close()

	if err := os.Rename(tempPath, absDiskPath); err != nil {
		return nil, fmt.Errorf("failed to finalize file move: %w", err)
	}

	attachment := &core.Attachment{
		ID:         core.NewUUID(),
		OwnerID:    ownerID,
		Filename:   originalFilename,
		MimeType:   mtype.String(),
		SizeBytes:  int(size),
		FileHash:   fileHash,
		DiskPath:   relDiskPath,
		Visibility: core.VisibilityPrivate,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	queryInsert := `
		INSERT INTO attachments (id, owner_id, filename, mime_type, size_bytes, file_hash, disk_path, visibility, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = db.ExecContext(ctx, queryInsert,
		attachment.ID, attachment.OwnerID, attachment.Filename, attachment.MimeType,
		attachment.SizeBytes, attachment.FileHash, attachment.DiskPath, attachment.Visibility,
		attachment.CreatedAt, attachment.UpdatedAt,
	)
	if err != nil {
		os.Remove(absDiskPath)
		return nil, fmt.Errorf("failed to insert attachment record: %w", err)
	}

	return attachment, nil
}

// LinkAttachment associates an existing attachment with an entity (article, note).
func LinkAttachment(ctx context.Context, db *sql.DB, entityID, attachmentID uuid.UUID) error {
	query := `INSERT INTO rel_entity_attachments (entity_id, attachment_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, query, entityID, attachmentID)
	return err
}

// ListAttachments returns all attachments belonging to the given user.
func ListAttachments(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]core.Attachment, error) {
	query := `
		SELECT a.id, a.filename, a.mime_type, a.size_bytes, a.disk_path, a.created_at,
	    (SELECT COUNT(*) FROM rel_entity_attachments WHERE attachment_id = a.id) AS link_count
		FROM attachments a
		WHERE a.owner_id = $1
		ORDER BY a.created_at DESC
	`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []core.Attachment
	for rows.Next() {
		var a core.Attachment
		if err := rows.Scan(&a.ID, &a.Filename, &a.MimeType, &a.SizeBytes, &a.DiskPath, &a.CreatedAt, &a.LinkCount); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

// GetEntityAttachments returns a list of attachments associated with a specific entity.
func GetEntityAttachments(ctx context.Context, db *sql.DB, entityID uuid.UUID) ([]core.Attachment, error) {
	query := `
		SELECT a.id, a.filename, a.mime_type, a.size_bytes, a.disk_path, a.created_at
		FROM attachments a
		JOIN rel_entity_attachments rel ON a.id = rel.attachment_id
		WHERE rel.entity_id = $1
		ORDER BY a.created_at DESC
	`
	rows, err := db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []core.Attachment
	for rows.Next() {
		var a core.Attachment
		if err := rows.Scan(&a.ID, &a.Filename, &a.MimeType, &a.SizeBytes, &a.DiskPath, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

// FindAttachments resolves an attachment by full UUID, short UUID suffix, or exact filename.
func FindAttachments(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string) ([]core.Attachment, error) {
	query := `SELECT id, filename FROM attachments WHERE owner_id = $1`
	args := []interface{}{ownerID}

	if parsedID, err := core.ParseUUID(identifier); err == nil {
		args = append(args, parsedID)
		query += fmt.Sprintf(` AND id = $%d`, len(args))
	} else {
		args = append(args, "%"+identifier, identifier)
		query += fmt.Sprintf(` AND (id::text LIKE $%d OR filename = $%d)`, len(args)-1, len(args))
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("attachment search failed: %w", err)
	}
	defer rows.Close()

	var results []core.Attachment
	for rows.Next() {
		var a core.Attachment
		if err := rows.Scan(&a.ID, &a.Filename); err != nil {
			return nil, err
		}
		results = append(results, a)
	}
	return results, nil
}

// RenameAttachment updates the filename of an attachment in the database.
func RenameAttachment(ctx context.Context, db *sql.DB, ownerID, attachmentID uuid.UUID, newName string) error {
	query := `UPDATE attachments SET filename = $1, updated_at = NOW() WHERE id = $2 AND owner_id = $3`
	_, err := db.ExecContext(ctx, query, newName, attachmentID, ownerID)
	return err
}

// DeleteAttachment removes an orphaned attachment from both the database and disk.
func DeleteAttachment(ctx context.Context, db *sql.DB, ownerID, attachmentID uuid.UUID) error {
	var diskPath string
	var linkCount int

	// Verify the attachment exists and count how many entities reference it.
	queryCheck := `
		SELECT disk_path, (SELECT COUNT(*) FROM rel_entity_attachments WHERE attachment_id = a.id)
		FROM attachments a WHERE id = $1 AND owner_id = $2
	`
	err := db.QueryRowContext(ctx, queryCheck, attachmentID, ownerID).Scan(&diskPath, &linkCount)
	if err != nil {
		return err
	}

	if linkCount > 0 {
		return fmt.Errorf("attachment cannot be deleted: still linked to %d entities", linkCount)
	}

	// 1. Delete from database.
	_, err = db.ExecContext(ctx, `DELETE FROM attachments WHERE id = $1 AND owner_id = $2`, attachmentID, ownerID)
	if err != nil {
		return err
	}

	// 2. Delete from disk.
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}
	absPath := filepath.Join(storagePath, diskPath)

	// Physical removal may fail (e.g. file already gone); ignore the error.
	_ = os.Remove(absPath)

	return nil
}
