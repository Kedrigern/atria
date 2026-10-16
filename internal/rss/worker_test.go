package rss_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"atria/internal/netutil"
	"atria/internal/rss"
	"atria/internal/testutil"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const guidlessFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>No GUIDs</title>
  <item><title>First</title><link>https://example.com/1</link></item>
  <item><title>Second</title><link>https://example.com/2</link></item>
  <item><title>Third, no link</title><description>only text</description></item>
</channel></rss>`

// fakeFeed serves a configurable response and records the last request.
type fakeFeed struct {
	mu      sync.Mutex
	status  int
	body    string
	etag    string
	lastReq *http.Request
}

func (f *fakeFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastReq = r.Clone(context.Background())
	if f.etag != "" {
		if r.Header.Get("If-None-Match") == f.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", f.etag)
	}
	w.Header().Set("Content-Type", "application/rss+xml")
	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	fmt.Fprint(w, f.body)
}

func (f *fakeFeed) set(status int, body, etag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body, f.etag = status, body, etag
}

func (f *fakeFeed) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

type feedState struct {
	items      int
	status     sql.NullInt64
	lastError  sql.NullString
	failures   int
	etag       sql.NullString
	dueInHours float64
}

func loadFeedState(t *testing.T, db *sql.DB, feedID uuid.UUID) feedState {
	t.Helper()
	var s feedState
	require.NoError(t, db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM rss_items WHERE feed_id = f.id),
		       f.last_fetch_status, f.last_fetch_error, f.consecutive_failures, f.etag_header,
		       EXTRACT(EPOCH FROM (f.next_fetch_at - NOW()::timestamp)) / 3600
		FROM rss_feeds f WHERE f.id = $1`, feedID).
		Scan(&s.items, &s.status, &s.lastError, &s.failures, &s.etag, &s.dueInHours))
	return s
}

func setupFeed(t *testing.T) (context.Context, *sql.DB, uuid.UUID, *fakeFeed) {
	t.Helper()
	db, user := testutil.SetupTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.WithValue(context.Background(), netutil.AllowLocalKey, true)

	fake := &fakeFeed{body: guidlessFeed}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	feed, err := rss.CreateFeed(ctx, db, user.ID, "Fake", server.URL+"/feed")
	require.NoError(t, err)
	return ctx, db, feed.ID, fake
}

func TestFetchKeepsItemsWithoutGUID(t *testing.T) {
	ctx, db, feedID, _ := setupFeed(t)

	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	assert.Equal(t, 3, loadFeedState(t, db, feedID).items, "every GUID-less item must be stored")

	// Refetching the same document must not create duplicates.
	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	assert.Equal(t, 3, loadFeedState(t, db, feedID).items)
}

func TestFetchSchedulesWithDatabaseClock(t *testing.T) {
	ctx, db, feedID, _ := setupFeed(t)

	// When the database session time zone differs from the Go process's local
	// zone, a Go-computed next_fetch_at used to be shifted by the offset.
	// Pin one connection so the SET applies to every query below; +14h
	// differs from any zone CI or a developer machine runs in.
	db.SetMaxOpenConns(1)
	_, err := db.Exec(`SET TIME ZONE 'Pacific/Kiritimati'`)
	require.NoError(t, err)

	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	state := loadFeedState(t, db, feedID)
	assert.InDelta(t, 1.0, state.dueInHours, 0.05, "next fetch should be due in one hour")
}

func TestFetchUsesConditionalGET(t *testing.T) {
	ctx, db, feedID, fake := setupFeed(t)
	fake.set(0, guidlessFeed, `"v1"`)

	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	state := loadFeedState(t, db, feedID)
	assert.Equal(t, `"v1"`, state.etag.String, "ETag must be stored")
	assert.EqualValues(t, http.StatusOK, state.status.Int64)

	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	assert.Equal(t, `"v1"`, fake.last().Header.Get("If-None-Match"))
	state = loadFeedState(t, db, feedID)
	assert.EqualValues(t, http.StatusNotModified, state.status.Int64)
	assert.Equal(t, 3, state.items)
	assert.Zero(t, state.failures)
}

func TestFetchErrorBacksOff(t *testing.T) {
	ctx, db, feedID, fake := setupFeed(t)
	fake.set(http.StatusInternalServerError, "<html>oops</html>", "")

	require.Error(t, rss.FetchFeed(ctx, db, feedID))
	state := loadFeedState(t, db, feedID)
	assert.EqualValues(t, http.StatusInternalServerError, state.status.Int64)
	assert.Contains(t, state.lastError.String, "500")
	assert.Equal(t, 1, state.failures)
	assert.InDelta(t, 2.0, state.dueInHours, 0.05)
	assert.Zero(t, state.items, "an error page must not be parsed as a feed")

	require.Error(t, rss.FetchFeed(ctx, db, feedID))
	state = loadFeedState(t, db, feedID)
	assert.Equal(t, 2, state.failures)
	assert.InDelta(t, 4.0, state.dueInHours, 0.05)

	fake.set(0, guidlessFeed, "")
	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	state = loadFeedState(t, db, feedID)
	assert.Zero(t, state.failures, "a success resets the backoff")
	assert.Empty(t, state.lastError.String)
	assert.InDelta(t, 1.0, state.dueInHours, 0.05)
}

func TestFetchRejectsOversizedFeed(t *testing.T) {
	ctx, db, feedID, fake := setupFeed(t)
	fake.set(0, `<?xml version="1.0"?><rss version="2.0"><channel><title>`+strings.Repeat("x", 6<<20)+`</title></channel></rss>`, "")

	err := rss.FetchFeed(ctx, db, feedID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than")
}

func TestFetchSendsFeedCredentials(t *testing.T) {
	ctx, db, feedID, fake := setupFeed(t)

	_, err := db.Exec(`UPDATE rss_feeds SET http_auth_type = 'bearer', http_auth_token = 'tok' WHERE id = $1`, feedID)
	require.NoError(t, err)
	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	assert.Equal(t, "Bearer tok", fake.last().Header.Get("Authorization"))

	_, err = db.Exec(`UPDATE rss_feeds SET http_auth_type = 'basic', http_auth_username = 'me', http_auth_token = 'secret' WHERE id = $1`, feedID)
	require.NoError(t, err)
	require.NoError(t, rss.FetchFeed(ctx, db, feedID))
	user, pass, ok := fake.last().BasicAuth()
	assert.True(t, ok)
	assert.Equal(t, "me", user)
	assert.Equal(t, "secret", pass)
}

func TestScheduledFetchSkipsDeletedFeeds(t *testing.T) {
	ctx, db, feedID, fake := setupFeed(t)

	_, err := db.Exec(`UPDATE entities SET deleted_at = NOW() WHERE id = $1`, feedID)
	require.NoError(t, err)

	require.NoError(t, rss.FetchAllActiveFeeds(ctx, db))
	assert.Nil(t, fake.last(), "deleted feeds must not be fetched")
}
