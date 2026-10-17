package search_test

import (
	"context"
	"testing"

	"atria/internal/core"
	"atria/internal/search"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Feeds without full content store content = ” (not NULL); the snippet must
// then come from the description instead of being empty.
func TestRSSHeadlineFallsBackToDescription(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()

	feedID := core.NewUUID()
	_, err := db.ExecContext(ctx, `INSERT INTO entities (id, owner_id, type, visibility, title, slug, path)
		VALUES ($1, $2, 'rss', 'private', 'Feed', 'feed', '/')`, feedID, user.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO rss_feeds (id, feed_url) VALUES ($1, 'https://example.com/f')`, feedID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO rss_items (id, feed_id, title, link, guid, content, description, published_at)
		VALUES ($1, $2, 'Item', 'https://example.com/1', 'g1', '', 'a pulsar was observed last night', NOW())`,
		core.NewUUID(), feedID)
	require.NoError(t, err)

	results, err := search.Search(ctx, db, user.ID, "pulsar", "rss", false)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, search.HeadlineText(results[0].Headline), "*pulsar*")
}
