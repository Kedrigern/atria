package web

import (
	"context"
	"net/http"
	"testing"

	"atria/internal/core"
	"atria/internal/rss"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAndRSSPagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	feed, err := rss.CreateFeed(context.Background(), db, user.ID, "Daily news", "https://example.com/feed")
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO rss_items (id, feed_id, title, link, guid, published_at, description)
		VALUES ($1, $2, 'Fresh story', 'https://example.com/fresh', 'fresh', NOW(), 'Short text.')`,
		core.NewUUID(), feed.ID)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	for _, path := range []string{"/read", "/rss", "/rss/" + feed.ID.String()} {
		w := getWithCookie(r, path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		main := mainContent(w.Body.String())
		assert.NotRegexp(t, czechLetters, main, "%s: Czech text in the page", path)
		assert.NotRegexp(t, inlineStyle, main, "%s: inline style in the page", path)
	}

	rssPage := getWithCookie(r, "/rss", nil).Body.String()
	assert.Contains(t, rssPage, "Fresh story")
	assert.Contains(t, rssPage, `class="inline-content" hidden`, "the full text starts hidden, without an inline style")
	assert.Contains(t, getWithCookie(r, "/rss/"+feed.ID.String(), nil).Body.String(), "data-confirm=")
}
