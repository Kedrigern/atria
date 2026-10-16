package rss

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"atria/internal/core"
	"atria/internal/netutil"

	"github.com/gofrs/uuid/v5"
	"github.com/mmcdole/gofeed"
)

const dueFeedsQuery = `
	SELECT f.id, f.feed_url, f.etag_header, f.last_modified_header,
	       f.http_auth_type, f.http_auth_username, f.http_auth_token
	FROM rss_feeds f
	JOIN entities e ON e.id = f.id
	WHERE f.next_fetch_at <= NOW() AND e.deleted_at IS NULL
`

// FetchAllActiveFeeds iterates through all feeds of all users that are due
// for an update and fetches them concurrently using a worker pool. It is meant
// for the background worker; request handlers use FetchActiveFeedsForOwner.
func FetchAllActiveFeeds(ctx context.Context, db *sql.DB) error {
	return fetchDueFeeds(ctx, db, dueFeedsQuery)
}

// FetchActiveFeedsForOwner fetches the due feeds belonging to one user.
func FetchActiveFeedsForOwner(ctx context.Context, db *sql.DB, ownerID uuid.UUID) error {
	return fetchDueFeeds(ctx, db, dueFeedsQuery+` AND e.owner_id = $1`, ownerID)
}

func fetchDueFeeds(ctx context.Context, db *sql.DB, query string, args ...any) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}

	var feedsToFetch []FeedToFetch
	for rows.Next() {
		var f FeedToFetch
		if err := rows.Scan(&f.ID, &f.FeedURL, &f.ETag, &f.LastMod, &f.AuthType, &f.AuthUsername, &f.AuthToken); err != nil {
			log.Printf("RSS Worker: failed to scan row: %v", err)
			continue
		}
		feedsToFetch = append(feedsToFetch, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(feedsToFetch) == 0 {
		return nil
	}

	const maxWorkers = 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	client := netutil.SafeHTTPClient()

	for _, f := range feedsToFetch {
		wg.Add(1)

		go func(feedInfo FeedToFetch) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			log.Printf("RSS Worker: Fetching %s", feedInfo.FeedURL)
			if err := fetchAndStoreFeed(ctx, db, client, feedInfo, 30*time.Second); err != nil {
				log.Printf("RSS Worker: fetch failed for %s: %v", feedInfo.FeedURL, err)
			}
		}(f)
	}

	wg.Wait()
	return nil
}

// FetchFeed immediately fetches one feed, regardless of its schedule.
func FetchFeed(ctx context.Context, db *sql.DB, feedID uuid.UUID) error {
	query := `
		SELECT id, feed_url, etag_header, last_modified_header,
		       http_auth_type, http_auth_username, http_auth_token
		FROM rss_feeds
		WHERE id = $1
	`
	var f FeedToFetch
	err := db.QueryRowContext(ctx, query, feedID).Scan(&f.ID, &f.FeedURL, &f.ETag, &f.LastMod, &f.AuthType, &f.AuthUsername, &f.AuthToken)
	if err != nil {
		return err
	}

	log.Printf("RSS Worker: Vynucené stažení zdroje %s", f.FeedURL)
	return fetchAndStoreFeed(ctx, db, netutil.SafeHTTPClient(), f, 40*time.Second)
}

// maxFeedBytes caps the size of a downloaded feed document.
const maxFeedBytes = 5 << 20

// fetchAndStoreFeed downloads one feed (conditionally, using the stored
// ETag/Last-Modified and the feed's HTTP auth), stores new items and records
// the outcome with UpdateFetchStatus. ctx is used for database writes; the
// HTTP request gets its own timeout.
func fetchAndStoreFeed(ctx context.Context, db *sql.DB, client *http.Client, f FeedToFetch, timeout time.Duration) error {
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Fetch failures describe the user's own feed (bad URL, HTTP status,
	// parse error), so they are safe and useful to show.
	fail := func(status int, err error) error {
		_ = UpdateFetchStatus(ctx, db, f.ID, status, err)
		return core.NewUserError("%w", err)
	}

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, f.FeedURL, nil)
	if err != nil {
		return fail(0, err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/feed+json, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	if f.ETag.Valid && f.ETag.String != "" {
		req.Header.Set("If-None-Match", f.ETag.String)
	}
	if f.LastMod.Valid && f.LastMod.String != "" {
		req.Header.Set("If-Modified-Since", f.LastMod.String)
	}
	applyFeedAuth(req, f)

	resp, err := client.Do(req)
	if err != nil {
		return fail(0, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return UpdateFetchStatus(ctx, db, f.ID, resp.StatusCode, nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fail(resp.StatusCode, fmt.Errorf("unexpected HTTP status %s", resp.Status))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return fail(resp.StatusCode, fmt.Errorf("read feed: %w", err))
	}
	if len(body) > maxFeedBytes {
		return fail(resp.StatusCode, fmt.Errorf("feed is larger than %d MB", maxFeedBytes>>20))
	}

	feed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
	if err != nil {
		return fail(resp.StatusCode, fmt.Errorf("parse feed: %w", err))
	}

	for _, item := range feed.Items {
		if err := storeFeedItem(ctx, db, f.ID, item); err != nil {
			log.Printf("RSS Worker: failed to save item %s: %v", item.Link, err)
		}
	}

	if err := saveCacheHeaders(ctx, db, f.ID, resp.Header); err != nil {
		log.Printf("RSS Worker: failed to save cache headers for %s: %v", f.FeedURL, err)
	}
	return UpdateFetchStatus(ctx, db, f.ID, resp.StatusCode, nil)
}

func applyFeedAuth(req *http.Request, f FeedToFetch) {
	if f.AuthType == nil || f.AuthToken == nil || *f.AuthToken == "" {
		return
	}
	switch *f.AuthType {
	case "basic":
		username := ""
		if f.AuthUsername != nil {
			username = *f.AuthUsername
		}
		req.SetBasicAuth(username, *f.AuthToken)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+*f.AuthToken)
	}
}

func saveCacheHeaders(ctx context.Context, db *sql.DB, feedID uuid.UUID, h http.Header) error {
	_, err := db.ExecContext(ctx, `
		UPDATE rss_feeds
		SET etag_header = NULLIF($1, ''), last_modified_header = NULLIF($2, '')
		WHERE id = $3`,
		truncateHeader(h.Get("ETag")), truncateHeader(h.Get("Last-Modified")), feedID)
	return err
}

// truncateHeader keeps cache validators within the VARCHAR(255) columns; an
// oversized validator is dropped rather than stored mangled.
func truncateHeader(v string) string {
	if len(v) > 255 {
		return ""
	}
	return v
}

func storeFeedItem(ctx context.Context, db *sql.DB, feedID uuid.UUID, item *gofeed.Item) error {
	pubDate := time.Now().UTC()
	if item.PublishedParsed != nil {
		pubDate = item.PublishedParsed.UTC()
	} else if item.UpdatedParsed != nil {
		pubDate = item.UpdatedParsed.UTC()
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO rss_items (id, feed_id, title, link, description, content, guid, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (feed_id, guid) DO NOTHING`,
		core.NewUUID(), feedID, item.Title, item.Link, item.Description, item.Content, itemKey(item), pubDate,
	)
	return err
}

// itemKey returns the identity used to deduplicate feed items. Many feeds omit
// <guid>; gofeed then reports "", which would collapse every such item into
// one row under UNIQUE(feed_id, guid). Fall back to the link, and if there is
// none, to a hash of the item's text.
func itemKey(item *gofeed.Item) string {
	if guid := strings.TrimSpace(item.GUID); guid != "" {
		return guid
	}
	if link := strings.TrimSpace(item.Link); link != "" {
		return link
	}
	sum := sha256.Sum256([]byte(item.Title + "\x00" + item.Description + "\x00" + item.Content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
