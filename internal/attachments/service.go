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
func AddAttachment(ctx context.Context, db *sql.DB, ownerID uuid.UUID, localPath string) (*core.Attachment, error) {
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
	destFile, err := os.Create(absDiskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return nil, fmt.Errorf("failed to write to storage: %w", err)
	}

	attachment := &core.Attachment{
		ID:         core.NewUUID(),
		OwnerID:    ownerID,
		Filename:   filepath.Base(localPath),
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

// LinkAttachment propojí existující přílohu s entitou (článek, poznámka).
func LinkAttachment(ctx context.Context, db *sql.DB, entityID, attachmentID uuid.UUID) error {
	query := `INSERT INTO rel_entity_attachments (entity_id, attachment_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, query, entityID, attachmentID)
	return err
}

// ListAttachments vrací seznam všech příloh uživatele.
func ListAttachments(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]core.Attachment, error) {
	query := `SELECT id, filename, mime_type, size_bytes, disk_path, created_at FROM attachments WHERE owner_id = $1 ORDER BY created_at DESC`
	rows, err := db.QueryContext(ctx, query, ownerID)
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
