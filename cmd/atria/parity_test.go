package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/ebooks"
	"atria/internal/notes"
	"atria/internal/testutil"

	"github.com/gofrs/uuid/v5"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLI(t, args...)
	require.NoError(t, err, out)
	return out
}

func TestParityCommands(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("DATABASE_URL", testutil.TestDatabaseURL(t))
	t.Setenv("ATRIA_USER", user.Email)
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()

	note, err := notes.CreateNote(ctx, db, user.ID, "Garage", "/", "solar notes for the garage")
	require.NoError(t, err)

	articleID := core.NewUUID()
	_, err = db.Exec(`INSERT INTO entities (id, owner_id, type, visibility, title, slug) VALUES ($1, $2, 'article', 'private', 'Roof story', 'roof-story')`, articleID, user.ID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO articles (id, original_url, domain, html_content, text_content) VALUES ($1, 'https://example.com/roof', 'example.com', '<p>solar roof</p>', 'solar roof')`, articleID)
	require.NoError(t, err)

	feedID := core.NewUUID()
	_, err = db.Exec(`INSERT INTO entities (id, owner_id, type, visibility, title, slug) VALUES ($1, $2, 'rss', 'private', 'Energy', 'energy')`, feedID, user.ID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO rss_feeds (id, feed_url) VALUES ($1, 'https://example.com/feed')`, feedID)
	require.NoError(t, err)
	itemIDs := []uuid.UUID{core.NewUUID(), core.NewUUID()}
	for i, id := range itemIDs {
		_, err = db.Exec(`INSERT INTO rss_items (id, feed_id, title, link, guid, published_at) VALUES ($1, $2, $3, 'https://example.com/i', $4, NOW())`,
			id, feedID, []string{"Wind", "Sun"}[i], id.String())
		require.NoError(t, err)
	}

	t.Run("search", func(t *testing.T) {
		out := mustCLI(t, "search", "solar")
		assert.Contains(t, out, "Garage")
		assert.Contains(t, out, "Roof story")
		assert.Contains(t, out, "*solar*")
		assert.NotContains(t, mustCLI(t, "search", "solar", "--in", "notes"), "Roof story")
	})

	t.Run("article actions", func(t *testing.T) {
		mustCLI(t, "article", "note", "Roof story", "check battery")
		mustCLI(t, "article", "visibility", "Roof story", "public")
		mustCLI(t, "article", "rename", "Roof story", "Our roof")
		mustCLI(t, "article", "archive", "Our roof")
		var title, visibility, userNote string
		var archived bool
		require.NoError(t, db.QueryRow(`SELECT e.title, e.visibility, a.user_note, a.is_archived FROM entities e JOIN articles a ON a.id = e.id WHERE e.id = $1`, articleID).
			Scan(&title, &visibility, &userNote, &archived))
		assert.Equal(t, []any{"Our roof", "public", "check battery", true}, []any{title, visibility, userNote, archived})

		mustCLI(t, "article", "rm", "Our roof")
		_, err := runCLI(t, "article", "archive", "Our roof")
		assert.Error(t, err, "trashed article is no longer found")
	})

	t.Run("note edit, rename, visibility", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "n.md")
		require.NoError(t, os.WriteFile(file, []byte("fresh content"), 0o600))
		mustCLI(t, "note", "edit", "Garage", "--file", file)
		mustCLI(t, "note", "edit", "Garage", "--title", "Workshop", "--path", "/home")
		mustCLI(t, "note", "visibility", "Workshop", "users")

		n, err := notes.GetNote(ctx, db, note.ID, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "fresh content", n.MarkdownContent)
		assert.Equal(t, "Workshop", n.Title)
		assert.Equal(t, "/home", n.Path)
		assert.Equal(t, core.VisibilityUsers, n.Visibility)

		t.Setenv("EDITOR", "sed -i s/fresh/edited/")
		mustCLI(t, "note", "edit", "Workshop")
		n, err = notes.GetNote(ctx, db, note.ID, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "edited content", n.MarkdownContent)

		_, err = runCLI(t, "note", "visibility", "Workshop", "everyone")
		assert.Error(t, err)
	})

	t.Run("rss items by short id", func(t *testing.T) {
		short := itemIDs[0].String()[len(itemIDs[0].String())-8:]
		out := mustCLI(t, "rss", "read", short)
		assert.Contains(t, out, "Read: Wind")
		mustCLI(t, "rss", "read-all", "Energy")
		var unread int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM rss_items WHERE feed_id = $1 AND read_at IS NULL`, feedID).Scan(&unread))
		assert.Zero(t, unread)
		mustCLI(t, "rss", "rename", "Energy", "Energy news")

		_, err := runCLI(t, "rss", "read", "deadbeef")
		assert.ErrorContains(t, err, "no rss item found")
	})

	t.Run("attachments", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "a.txt")
		require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))
		att, err := attachments.AddAttachment(ctx, db, user.ID, src, "a.txt")
		require.NoError(t, err)

		mustCLI(t, "attachment", "rename", att.ID.String(), "renamed.txt")
		mustCLI(t, "attachment", "rm", "renamed.txt")
		list, err := attachments.ListAttachments(ctx, db, user.ID)
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("ebook edit and rm", func(t *testing.T) {
		book, err := ebooks.Generate(ctx, db, user.ID, ebooks.GenerateInput{
			Title:   "Reading",
			Sources: []core.EntitySummary{{ID: note.ID, Title: "Workshop", Type: core.TypeNote}},
		})
		require.NoError(t, err)

		mustCLI(t, "ebook", "edit", "Reading", "--author", "Me", "--language", "cs")
		d, err := ebooks.GetDetail(ctx, db, user.ID, book.ID)
		require.NoError(t, err)
		require.NotNil(t, d.Author)
		assert.Equal(t, "Me", *d.Author)
		assert.Equal(t, "Reading", d.Title, "fields not given stay unchanged")

		_, err = runCLI(t, "ebook", "edit", "Reading")
		assert.ErrorContains(t, err, "nothing to change")

		out := mustCLI(t, "ebook", "rm", "Reading")
		assert.True(t, strings.Contains(out, "trash"))
	})
}
