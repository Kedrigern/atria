package rss

import (
	"context"
	"database/sql"
	"log"

	"atria/internal/core"

	"github.com/mmcdole/gofeed"
)

// FetchAllActiveFeeds iterates through all feeds that are due for an update
// and saves new items to the triage storage.
func FetchAllActiveFeeds(ctx context.Context, db *sql.DB) error {
	// 1. Find feeds where next_fetch_at has passed.
	query := `SELECT id, feed_url, etag_header, last_modified_header FROM rss_feeds WHERE next_fetch_at <= NOW()`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	fp := gofeed.NewParser()

	for rows.Next() {
		var f FeedToFetch
		if err := rows.Scan(&f.ID, &f.FeedURL, &f.ETag, &f.LastMod); err != nil {
			log.Printf("RSS Worker: failed to scan row: %v", err)
			continue
		}

		log.Printf("RSS Worker: Fetching %s", f.FeedURL)

		// 2. Parse the feed from URL.
		feed, err := fp.ParseURLWithContext(f.FeedURL, ctx)
		if err != nil {
			log.Printf("RSS Worker: fetch failed for %s: %v", f.FeedURL, err)
			_ = UpdateFetchStatus(ctx, db, f.ID, 0, err)
			continue
		}

		// 3. Save new items into the triage table (rss_items).
		for _, item := range feed.Items {
			itemID := core.NewUUID()

			// Use ON CONFLICT to avoid duplicates based on the feed_id + guid unique constraint.
			queryItem := `
				INSERT INTO rss_items (id, feed_id, title, link, description, content, guid)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (feed_id, guid) DO NOTHING
			`
			_, err = db.ExecContext(ctx, queryItem,
				itemID, f.ID, item.Title, item.Link, item.Description, item.Content, item.GUID,
			)
			if err != nil {
				log.Printf("RSS Worker: failed to save item %s: %v", item.Link, err)
			}
		}

		// 4. Mark the fetch attempt as successful (HTTP 200).
		_ = UpdateFetchStatus(ctx, db, f.ID, 200, nil)
	}

	return nil
}
