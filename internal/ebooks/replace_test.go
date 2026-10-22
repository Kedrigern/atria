package ebooks

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/attachments"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestEbookReplaceKeepsFileAndPageInStep(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)
	dir := t.TempDir()

	original := filepath.Join(dir, "book.epub")
	writeTestEPUB(t, original, true)
	book, err := Import(ctx, db, user.ID, ImportInput{Path: original, Filename: "book.epub"})
	require.NoError(t, err)
	require.NotNil(t, book.CoverAttachmentID, "the import keeps the cover of the file")
	var attachmentsBefore int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE owner_id = $1`, user.ID).Scan(&attachmentsBefore))
	assert.Equal(t, 2, attachmentsBefore)

	downloadPath := func() string {
		download, err := GetDownload(ctx, db, user.ID, book.ID)
		require.NoError(t, err)
		return attachments.AbsPath(download.DiskPath)
	}

	// Edit writes the metadata into the file. The old file goes away.
	require.NoError(t, UpdateMetadata(ctx, db, user.ID, book.ID, "New title", "", "Someone", "cs"))
	metadata, err := ReadEPUBMetadata(downloadPath())
	require.NoError(t, err)
	assert.Equal(t, "New title", metadata.Title)
	assert.Equal(t, "Someone", metadata.Author)
	assert.Equal(t, "cs", metadata.Language)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE owner_id = $1`, user.ID).Scan(&count))
	assert.Equal(t, 2, count, "the old EPUB is deleted, because nothing uses it")
	assert.Equal(t, 2, countFiles(t, storage), "and its file too")

	// A new cover goes into the file and onto the page.
	coverPath := filepath.Join(dir, "new-cover.jpg")
	f, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(f, testImage(t, 33, 44), nil))
	require.NoError(t, f.Close())
	require.NoError(t, ReplaceCover(ctx, db, user.ID, book.ID, coverPath, "new-cover.jpg"))

	extracted, err := ExtractEPUBCover(downloadPath(), dir)
	require.NoError(t, err)
	data, err := os.ReadFile(extracted.Path)
	require.NoError(t, err)
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 33, img.Bounds().Dx(), "the EPUB has the new cover")
	detail, err := GetDetail(ctx, db, user.ID, book.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.CoverDiskPath)
	pageCover, err := os.ReadFile(attachments.AbsPath(*detail.CoverDiskPath))
	require.NoError(t, err)
	assert.Equal(t, data, pageCover, "the page shows the same cover as the file")
	assert.Equal(t, "New title", detail.Title, "the metadata stays")
	assert.Equal(t, 2, countFiles(t, storage), "the old cover and EPUB are deleted")

	// A new file without a cover keeps the cover of Atria, inside the file.
	plain := filepath.Join(dir, "better.epub")
	writeTestEPUB(t, plain, false)
	require.NoError(t, ReplaceFile(ctx, db, user.ID, book.ID, plain, "better.epub"))
	detail, err = GetDetail(ctx, db, user.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "Old title", detail.Title, "the new file brings its own title")
	assert.Equal(t, "better.epub", detail.Filename)
	extracted, err = ExtractEPUBCover(downloadPath(), dir)
	require.NoError(t, err, "the cover of Atria goes into the new file")
	data, err = os.ReadFile(extracted.Path)
	require.NoError(t, err)
	img, err = jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 33, img.Bounds().Dx())
}

func TestEbookReplaceFailureLeavesNoFiles(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)
	dir := t.TempDir()

	original := filepath.Join(dir, "book.epub")
	writeTestEPUB(t, original, true)
	book, err := Import(ctx, db, user.ID, ImportInput{Path: original, Filename: "book.epub"})
	require.NoError(t, err)
	before := countFiles(t, storage)

	// A failure after a new file is stored rolls back the row and removes the file.
	other := filepath.Join(dir, "other.epub")
	writeTestEPUB(t, other, false)
	err = inTx(ctx, db, func(tx *sql.Tx, files *attachments.NewFiles) error {
		if _, _, err := storeEPUB(ctx, tx, files, user.ID, other, "other.epub", "", ""); err != nil {
			return err
		}
		require.Equal(t, before+1, countFiles(t, storage), "the file is written inside the transaction")
		return errors.New("a later step fails")
	})
	require.Error(t, err)
	assert.Equal(t, before, countFiles(t, storage), "the new file is removed")
	var rows int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE owner_id = $1 AND filename = 'other.epub'`, user.ID).Scan(&rows))
	assert.Zero(t, rows, "and the row is rolled back")

	notEPUB := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notEPUB, []byte("plain text"), 0o600))
	assert.Error(t, ReplaceFile(ctx, db, user.ID, book.ID, notEPUB, "notes.txt"))
	assert.Error(t, ReplaceCover(ctx, db, user.ID, book.ID, notEPUB, "notes.txt"))
	assert.Equal(t, before, countFiles(t, storage))
}
