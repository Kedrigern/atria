package rss

import (
	"context"
	"database/sql"
	"log"
	"sync"

	"atria/internal/core"

	"github.com/mmcdole/gofeed"
)

// FetchAllActiveFeeds iterates through all feeds that are due for an update
// and fetches them concurrently using a worker pool.
func FetchAllActiveFeeds(ctx context.Context, db *sql.DB) error {
	query := `SELECT id, feed_url, etag_header, last_modified_header FROM rss_feeds WHERE next_fetch_at <= NOW()`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}

	var feedsToFetch []FeedToFetch
	for rows.Next() {
		var f FeedToFetch
		if err := rows.Scan(&f.ID, &f.FeedURL, &f.ETag, &f.LastMod); err != nil {
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

	for _, f := range feedsToFetch {
		wg.Add(1)

		go func(feedInfo FeedToFetch) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			log.Printf("RSS Worker: Fetching %s", feedInfo.FeedURL)

			feed, err := fp.ParseURLWithContext(feedInfo.FeedURL, ctx)
			if err != nil {
				log.Printf("RSS Worker: fetch failed for %s: %v", feedInfo.FeedURL, err)
				_ = UpdateFetchStatus(ctx, db, feedInfo.ID, 0, err)
				return
			}

			for _, item := range feed.Items {
				itemID := core.NewUUID()

				queryItem := `
					INSERT INTO rss_items (id, feed_id, title, link, description, content, guid)
					VALUES ($1, $2, $3, $4, $5, $6, $7)
					ON CONFLICT (feed_id, guid) DO NOTHING
				`
				_, err = db.ExecContext(ctx, queryItem,
					itemID, feedInfo.ID, item.Title, item.Link, item.Description, item.Content, item.GUID,
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
