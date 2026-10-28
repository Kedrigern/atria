package web

import (
	"context"
	"net/http"
	"testing"

	"atria/internal/rss"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The home page counts all saved articles and unread RSS items, not only the
// first page of 100 items.
func TestHomePageCountsMoreThanOneHundredItems(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO entities (id, owner_id, type, title, slug)
		SELECT gen_random_uuid(), $1, 'article', 'Article ' || n, 'article-' || n
		FROM generate_series(1, 121) AS n`, user.ID)
	require.NoError(t, err)
	// One article is archived, so the inbox has 120 articles.
	_, err = db.ExecContext(ctx, `
		INSERT INTO articles (id, original_url, domain, is_archived)
		SELECT id, 'https://example.com/' || slug, 'example.com', slug = 'article-121'
		FROM entities WHERE owner_id = $1 AND type = 'article'`, user.ID)
	require.NoError(t, err)

	feed, err := rss.CreateFeed(ctx, db, user.ID, "Daily news", "https://example.com/feed")
	require.NoError(t, err)
	// One item is read, so 130 items are unread.
	_, err = db.ExecContext(ctx, `
		INSERT INTO rss_items (id, feed_id, title, link, guid, published_at, read_at)
		SELECT gen_random_uuid(), $1, 'Story ' || n, 'https://example.com/story-' || n, 'story-' || n, NOW(),
		       CASE WHEN n = 131 THEN NOW() END
		FROM generate_series(1, 131) AS n`, feed.ID)
	require.NoError(t, err)

	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	w := getWithCookie(r, "/", nil)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()
	assert.Contains(t, page, "Saved articles: <strong>120</strong>")
	assert.Contains(t, page, "Unread items: <strong>130</strong>")
}
