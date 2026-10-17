package rss_test

import (
	"testing"
	"time"

	"atria/internal/rss"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneReadItems(t *testing.T) {
	ctx, db, feedID, _ := setupFeed(t)
	require.NoError(t, rss.FetchFeed(ctx, db, feedID))

	// "First" was read long ago, "Second" yesterday, "Third, no link" is unread.
	_, err := db.Exec(`UPDATE rss_items SET read_at = NOW() - INTERVAL '200 days' WHERE feed_id = $1 AND title = 'First'`, feedID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE rss_items SET read_at = NOW() - INTERVAL '1 day' WHERE feed_id = $1 AND title = 'Second'`, feedID)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE rss_items SET description = 'old body', content = '<p>old body</p>' WHERE feed_id = $1`, feedID)
	require.NoError(t, err)

	n, err := rss.PruneReadItems(ctx, db, 180*24*time.Hour)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the item read more than 180 days ago is shrunk")

	type row struct {
		hasBody, pruned, read bool
	}
	load := func(title string) row {
		t.Helper()
		var r row
		require.NoError(t, db.QueryRow(`
			SELECT content IS NOT NULL OR description IS NOT NULL, content_pruned_at IS NOT NULL, read_at IS NOT NULL
			FROM rss_items WHERE feed_id = $1 AND title = $2`, feedID, title).Scan(&r.hasBody, &r.pruned, &r.read))
		return r
	}

	assert.Equal(t, row{hasBody: false, pruned: true, read: true}, load("First"))
	assert.Equal(t, row{hasBody: true, pruned: false, read: true}, load("Second"), "recently read items keep their content")
	assert.Equal(t, row{hasBody: true, pruned: false, read: false}, load("Third, no link"), "unread items are never touched")

	t.Run("second run has nothing to do", func(t *testing.T) {
		n, err := rss.PruneReadItems(ctx, db, 180*24*time.Hour)
		require.NoError(t, err)
		assert.Zero(t, n)
	})

	t.Run("shrunk item still in the feed is not re-imported as unread", func(t *testing.T) {
		require.NoError(t, rss.FetchFeed(ctx, db, feedID))
		var total, unread int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE read_at IS NULL) FROM rss_items WHERE feed_id = $1`, feedID).Scan(&total, &unread))
		assert.Equal(t, 3, total)
		assert.Equal(t, 1, unread)
		assert.Equal(t, row{hasBody: false, pruned: true, read: true}, load("First"))
	})

	t.Run("title stays searchable", func(t *testing.T) {
		var found bool
		require.NoError(t, db.QueryRow(`SELECT search_vector @@ to_tsquery('english', 'first') FROM rss_items WHERE feed_id = $1 AND title = 'First'`, feedID).Scan(&found))
		assert.True(t, found)
	})
}

func TestRetentionFromEnv(t *testing.T) {
	cases := map[string]time.Duration{
		"":     rss.DefaultRetention,
		"30":   30 * 24 * time.Hour,
		"0":    rss.DefaultRetention,
		"-5":   rss.DefaultRetention,
		"half": rss.DefaultRetention,
	}
	for value, want := range cases {
		t.Setenv("RSS_RETENTION_DAYS", value)
		assert.Equal(t, want, rss.RetentionFromEnv(), "RSS_RETENTION_DAYS=%q", value)
	}
}
