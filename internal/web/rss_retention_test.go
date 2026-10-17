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

func TestFeedDetailShowsPrunedNote(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()

	feed, err := rss.CreateFeed(ctx, db, user.ID, "Retention feed", "https://example.com/feed")
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO rss_items (id, feed_id, title, link, guid, published_at, read_at, content_pruned_at)
		VALUES ($1, $2, 'Old story', 'https://example.com/old', 'old', NOW(), NOW(), NOW())`,
		core.NewUUID(), feed.ID)
	require.NoError(t, err)

	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	w := getWithCookie(r, "/rss/"+feed.ID.String()+"?archived=1", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Old story")
	assert.Contains(t, w.Body.String(), "odstraněn, aby nezabíral místo")
	assert.Contains(t, w.Body.String(), `href="https://example.com/old"`)
}
