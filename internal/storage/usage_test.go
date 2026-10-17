package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/storage"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsage(t *testing.T) {
	db, heavy := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	t.Setenv("STORAGE_PATH", t.TempDir())

	light, err := users.CreateUser(ctx, db, "light@atria.local", "Light", "pass", core.RoleUser)
	require.NoError(t, err)

	// heavy: 2 notes, 1 attachment, 1 feed with 2 items
	_, err = notes.CreateNote(ctx, db, heavy.ID, "Big", "/", strings.Repeat("lorem ipsum dolor ", 2000))
	require.NoError(t, err)
	_, err = notes.CreateNote(ctx, db, heavy.ID, "Small", "/", "x")
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "f.bin")
	require.NoError(t, os.WriteFile(file, []byte(strings.Repeat("A", 5000)), 0o600))
	_, err = attachments.AddAttachment(ctx, db, heavy.ID, file, "f.bin")
	require.NoError(t, err)

	feed, err := rss.CreateFeed(ctx, db, heavy.ID, "Feed", "https://example.com/feed")
	require.NoError(t, err)
	for _, guid := range []string{"a", "b"} {
		_, err = db.Exec(`INSERT INTO rss_items (id, feed_id, title, link, content, guid, published_at)
			VALUES ($1, $2, 'Item', 'https://example.com', $3, $4, NOW())`,
			core.NewUUID(), feed.ID, strings.Repeat("<p>news</p>", 100), guid)
		require.NoError(t, err)
	}

	// light: 1 small note
	_, err = notes.CreateNote(ctx, db, light.ID, "Tiny", "/", "hi")
	require.NoError(t, err)

	u, err := storage.ForUser(ctx, db, heavy.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, u.NoteCount)
	assert.Positive(t, u.NoteBytes)
	assert.Less(t, u.NoteBytes, int64(36000), "stored size is the compressed size")
	assert.EqualValues(t, 1, u.AttachmentCount)
	assert.EqualValues(t, 5000, u.AttachmentBytes)
	assert.EqualValues(t, 2, u.RSSItemCount)
	assert.Positive(t, u.RSSBytes)
	assert.Zero(t, u.ArticleCount)
	assert.Equal(t, u.NoteBytes+u.RSSBytes+u.ArticleBytes+u.AttachmentBytes, u.TotalBytes())

	l, err := storage.ForUser(ctx, db, light.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, l.NoteCount)
	assert.Zero(t, l.AttachmentCount)
	assert.Zero(t, l.RSSItemCount, "another user's feed items must not be counted")

	all, err := storage.ForAllUsers(ctx, db)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, heavy.ID, all[0].UserID, "largest user first")
	assert.Equal(t, light.ID, all[1].UserID)

	size, err := storage.DatabaseSize(ctx, db)
	require.NoError(t, err)
	assert.Positive(t, size)
}
