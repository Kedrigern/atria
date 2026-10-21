package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedPageShowsErrorBoxOnlyForRealErrors(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	feed, err := rss.CreateFeed(ctx, db, user.ID, "Feed", "https://example.com/feed")
	require.NoError(t, err)
	page := func() string {
		w := getWithCookie(r, "/rss/"+feed.ID.String(), nil)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.String()
	}

	require.NoError(t, rss.UpdateFetchStatus(ctx, db, feed.ID, 200, nil))
	assert.NotContains(t, page(), "Last fetch error", "a successful fetch must not show the error box")

	// Rows written by older versions have an empty string instead of NULL.
	_, err = db.Exec(`UPDATE rss_feeds SET last_fetch_error = '' WHERE id = $1`, feed.ID)
	require.NoError(t, err)
	assert.NotContains(t, page(), "Last fetch error")

	require.NoError(t, rss.UpdateFetchStatus(ctx, db, feed.ID, 500, errors.New("unexpected HTTP status 500")))
	body := page()
	assert.Contains(t, body, "Last fetch error")
	assert.Contains(t, body, "unexpected HTTP status 500")
}

func TestSearchPageUsesQueryFromURL(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	_, err := notes.CreateNote(context.Background(), db, user.ID, "Quasar notes", "/", "observations of a quasar")
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	// Full page load, as after a reload, the back button or a saved link.
	full := getWithCookie(r, "/search?q=quasar", nil)
	require.Equal(t, http.StatusOK, full.Code)
	assert.Contains(t, full.Body.String(), "Quasar notes")
	assert.Contains(t, full.Body.String(), `value="quasar"`)

	// The htmx update while typing returns only the results.
	req := httptest.NewRequest(http.MethodGet, "/search?q=quasar", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "search-results")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Quasar notes")
	assert.False(t, strings.Contains(w.Body.String(), "<html"), "partial must not contain the page frame")

	empty := getWithCookie(r, "/search", nil)
	assert.Contains(t, empty.Body.String(), "Enter a search term")
}
