package search_test

import (
	"context"
	"testing"
	"time"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/search"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Helper to insert an article directly via SQL (no HTTP fetch needed).
	insertArticle := func(t *testing.T, ownerID uuid.UUID, title, textContent string) uuid.UUID {
		t.Helper()
		id := core.NewUUID()
		slug := "article-" + id.String()[len(id.String())-8:]

		_, err := db.ExecContext(ctx,
			`INSERT INTO entities (id, owner_id, type, visibility, title, slug, path)
			 VALUES ($1, $2, 'article', 'private', $3, $4, '/')`,
			id, ownerID, title, slug,
		)
		require.NoError(t, err, "insert article entity")

		_, err = db.ExecContext(ctx,
			`INSERT INTO articles (id, original_url, domain, text_content, html_content)
			 VALUES ($1, 'http://example.com', 'example.com', $2, '')`,
			id, textContent,
		)
		require.NoError(t, err, "insert article row")

		// Fire the search_vector trigger.
		_, err = db.ExecContext(ctx, `UPDATE articles SET html_content = html_content WHERE id = $1`, id)
		require.NoError(t, err, "fire article trigger")

		return id
	}

	// Helper to insert an RSS feed + item directly via SQL.
	insertRSSItem := func(t *testing.T, ownerID uuid.UUID, feedTitle, itemTitle, itemContent string) {
		t.Helper()
		feedID := core.NewUUID()
		feedSlug := "feed-" + feedID.String()[len(feedID.String())-8:]

		_, err := db.ExecContext(ctx,
			`INSERT INTO entities (id, owner_id, type, visibility, title, slug, path)
			 VALUES ($1, $2, 'rss', 'private', $3, $4, '/')`,
			feedID, ownerID, feedTitle, feedSlug,
		)
		require.NoError(t, err, "insert rss entity")

		_, err = db.ExecContext(ctx,
			`INSERT INTO rss_feeds (id, feed_url) VALUES ($1, 'http://example.com/feed.xml')`,
			feedID,
		)
		require.NoError(t, err, "insert rss_feeds row")

		itemID := core.NewUUID()
		_, err = db.ExecContext(ctx,
			`INSERT INTO rss_items (id, feed_id, title, link, content, guid, published_at)
			 VALUES ($1, $2, $3, 'http://example.com/item', $4, $5, $6)`,
			itemID, feedID, itemTitle, itemContent, itemID.String(), time.Now(),
		)
		require.NoError(t, err, "insert rss_items row")

		// Fire the search_vector trigger.
		_, err = db.ExecContext(ctx, `UPDATE rss_items SET title = title WHERE id = $1`, itemID)
		require.NoError(t, err, "fire rss_items trigger")
	}

	t.Run("EmptyQueryReturnsNil", func(t *testing.T) {
		results, err := search.Search(ctx, db, user.ID, "", "", false)
		require.NoError(t, err)
		assert.Nil(t, results)
	})

	t.Run("FindsNoteByContent", func(t *testing.T) {
		_, err := notes.CreateNote(ctx, db, user.ID, "Photosynthesis Note", "/", "photosynthesis is the process by which plants use sunlight")
		require.NoError(t, err)

		results, err := search.Search(ctx, db, user.ID, "photosynthesis", "notes", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "note", results[0].Type)
		assert.Equal(t, "Photosynthesis Note", results[0].Title)
	})

	t.Run("FindsArticleByContent", func(t *testing.T) {
		insertArticle(t, user.ID, "Mitochondria Article", "mitochondria is the powerhouse of the cell")

		results, err := search.Search(ctx, db, user.ID, "mitochondria", "articles", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "article", results[0].Type)
		assert.Equal(t, "Mitochondria Article", results[0].Title)
	})

	t.Run("FilterNotesOnly_ExcludesArticles", func(t *testing.T) {
		keyword := "chlorophyll"

		_, err := notes.CreateNote(ctx, db, user.ID, "Chlorophyll Note", "/", "chlorophyll gives plants their green color")
		require.NoError(t, err)

		insertArticle(t, user.ID, "Chlorophyll Article", "chlorophyll is a pigment found in plants")

		results, err := search.Search(ctx, db, user.ID, keyword, "notes", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "note", results[0].Type)
	})

	t.Run("FilterArticlesOnly_ExcludesNotes", func(t *testing.T) {
		keyword := "ribosomes"

		_, err := notes.CreateNote(ctx, db, user.ID, "Ribosomes Note", "/", "ribosomes synthesize proteins in the cell")
		require.NoError(t, err)

		insertArticle(t, user.ID, "Ribosomes Article", "ribosomes are molecular machines")

		results, err := search.Search(ctx, db, user.ID, keyword, "articles", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "article", results[0].Type)
	})

	t.Run("Visibility_OtherUsersPrivateNoteNotReturned", func(t *testing.T) {
		user2, err := users.CreateUser(ctx, db, "user2_private@atria.local", "User Two Private", "pass", core.RoleUser)
		require.NoError(t, err)

		entity, err := notes.CreateNote(ctx, db, user2.ID, "Private Quasar Note", "/", "quasar emits enormous energy from galactic center")
		require.NoError(t, err)

		// Ensure visibility is private (default, but be explicit).
		_, err = db.ExecContext(ctx, `UPDATE entities SET visibility = 'private' WHERE id = $1`, entity.ID)
		require.NoError(t, err)

		results, err := search.Search(ctx, db, user.ID, "quasar", "notes", false)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})

	t.Run("Visibility_OtherUsersPublicNoteIsReturned", func(t *testing.T) {
		user3, err := users.CreateUser(ctx, db, "user3_public@atria.local", "User Three Public", "pass", core.RoleUser)
		require.NoError(t, err)

		entity, err := notes.CreateNote(ctx, db, user3.ID, "Public Nebula Note", "/", "nebula is a cloud of gas and dust in space")
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `UPDATE entities SET visibility = 'public' WHERE id = $1`, entity.ID)
		require.NoError(t, err)

		// Notes search_vector is based on markdown_content, touch it to re-fire trigger
		// (visibility change doesn't affect tsvector, only entity join visibility check, so no re-trigger needed)

		results, err := search.Search(ctx, db, user.ID, "nebula", "notes", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "note", results[0].Type)
	})

	t.Run("RSSItemSearch", func(t *testing.T) {
		insertRSSItem(t, user.ID, "Science Feed", "Supernova Explosion", "supernova occurs when a massive star collapses")

		results, err := search.Search(ctx, db, user.ID, "supernova", "rss", false)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "rss_item", results[0].Type)
		assert.Equal(t, "Supernova Explosion", results[0].Title)
	})

	t.Run("NoResultsForUnknownTerm", func(t *testing.T) {
		results, err := search.Search(ctx, db, user.ID, "xyzzy_nonexistent_term_9847", "", false)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("AllFilterSearchesAcrossTypes", func(t *testing.T) {
		keyword := "cytoplasm"

		_, err := notes.CreateNote(ctx, db, user.ID, "Cytoplasm Note", "/", "cytoplasm fills the cell and supports organelles")
		require.NoError(t, err)

		insertArticle(t, user.ID, "Cytoplasm Article", "cytoplasm is a thick solution inside the cell membrane")

		results, err := search.Search(ctx, db, user.ID, keyword, "", false)
		require.NoError(t, err)
		require.Len(t, results, 2)

		types := map[string]bool{}
		for _, r := range results {
			types[r.Type] = true
		}
		assert.True(t, types["note"], "expected note in results")
		assert.True(t, types["article"], "expected article in results")
	})
}
