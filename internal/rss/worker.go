package rss

import (
	"context"
	"database/sql"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"atria/internal/core"
	"atria/internal/netutil"

	"github.com/gofrs/uuid/v5"
	"github.com/mmcdole/gofeed"
)

// FetchAllActiveFeeds iterates through all feeds that are due for an update
// and fetches them concurrently using a worker pool.
func FetchAllActiveFeeds(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT id, feed_url, etag_header, last_modified_header,
		       http_auth_type, http_auth_username, http_auth_token
		FROM rss_feeds
		WHERE next_fetch_at <= NOW()
	`
	rows, err := db.QueryContext(ctx, query)
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
	rows.Close()

	if len(feedsToFetch) == 0 {
		return nil
	}

	const maxWorkers = 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	fp := gofeed.NewParser()
	fp.Client = netutil.SafeHTTPClient()

	for _, f := range feedsToFetch {
		wg.Add(1)

		go func(feedInfo FeedToFetch) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			log.Printf("RSS Worker: Fetching %s", feedInfo.FeedURL)

			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			feed, err := fp.ParseURLWithContext(feedInfo.FeedURL, fetchCtx)
			if err != nil {
				log.Printf("RSS Worker: fetch failed for %s: %v", feedInfo.FeedURL, err)
				_ = UpdateFetchStatus(ctx, db, feedInfo.ID, 0, err)
				return
			}

			for _, item := range feed.Items {
				itemID := core.NewUUID()

				pubDate := time.Now().UTC()
				if item.PublishedParsed != nil {
					pubDate = item.PublishedParsed.UTC()
				} else if item.UpdatedParsed != nil {
					pubDate = item.UpdatedParsed.UTC()
				}

				queryItem := `
								INSERT INTO rss_items (id, feed_id, title, link, description, content, guid, published_at)
								VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
								ON CONFLICT (feed_id, guid) DO NOTHING
							`
				_, err = db.ExecContext(ctx, queryItem,
					itemID, feedInfo.ID, item.Title, item.Link, item.Description, item.Content, item.GUID, pubDate,
				)
				if err != nil {
					log.Printf("RSS Worker: failed to save item %s: %v", item.Link, err)
				}
			}

			_ = UpdateFetchStatus(ctx, db, feedInfo.ID, 200, nil)

		}(f)
	}

	wg.Wait()
	return nil
}

// FetchFeed imidietly fetch one feed
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

	fp := gofeed.NewParser()
	fp.Client = netutil.SafeHTTPClient()

	log.Printf("RSS Worker: Vynucené stažení zdroje %s", f.FeedURL)

	fetchCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, f.FeedURL, nil)
	if err != nil {
		_ = UpdateFetchStatus(ctx, db, f.ID, 0, err)
		return err
	}

	resp, err := fp.Client.Do(req)
	if err != nil {
		_ = UpdateFetchStatus(ctx, db, f.ID, 0, err)
		return err
	}
	defer resp.Body.Close()

	// Safety cap: limit response body to 5 MB.
	limitedXMLReader := io.LimitReader(resp.Body, 5*1024*1024)
	feed, err := fp.Parse(limitedXMLReader)
	if err != nil {
		_ = UpdateFetchStatus(ctx, db, f.ID, resp.StatusCode, err)
		return err
	}

	for _, item := range feed.Items {
		itemID := core.NewUUID()
		pubDate := time.Now().UTC()
		if item.PublishedParsed != nil {
			pubDate = item.PublishedParsed.UTC()
		} else if item.UpdatedParsed != nil {
			pubDate = item.UpdatedParsed.UTC()
		}

		queryItem := `
			INSERT INTO rss_items (id, feed_id, title, link, description, content, guid, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (feed_id, guid) DO NOTHING
		`
		_, err = db.ExecContext(ctx, queryItem,
			itemID, f.ID, item.Title, item.Link, item.Description, item.Content, item.GUID, pubDate,
		)
		if err != nil {
			log.Printf("RSS Worker: nepodařilo se uložit položku %s: %v", item.Link, err)
		}
	}

	return UpdateFetchStatus(ctx, db, f.ID, 200, nil)
}
