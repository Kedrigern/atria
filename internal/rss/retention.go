package rss

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

// DefaultRetention is how long read items keep their full content when
// RSS_RETENTION_DAYS is not set.
const DefaultRetention = 180 * 24 * time.Hour

// RetentionFromEnv reads RSS_RETENTION_DAYS (whole days, > 0).
func RetentionFromEnv() time.Duration {
	if s := os.Getenv("RSS_RETENTION_DAYS"); s != "" {
		if days, err := strconv.Atoi(s); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
		log.Printf("RSS: invalid RSS_RETENTION_DAYS=%q, using %d days", s, int(DefaultRetention.Hours()/24))
	}
	return DefaultRetention
}

// PruneReadItems shrinks read items that were read longer than retention ago:
// content and description (nearly all of the stored size) are cleared, while
// title, link, GUID and read time stay. Keeping the row matters: deleting it
// would make an item that is still in the feed reappear as unread on the next
// fetch. Unread items are never touched. It returns the number of items
// shrunk.
func PruneReadItems(ctx context.Context, db *sql.DB, retention time.Duration) (int64, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE rss_items
		SET content = NULL, description = NULL, content_pruned_at = NOW()
		WHERE read_at IS NOT NULL
		  AND content_pruned_at IS NULL
		  AND read_at < NOW() - make_interval(secs => $1)`,
		retention.Seconds())
	if err != nil {
		return 0, fmt.Errorf("prune read RSS items: %w", err)
	}
	return result.RowsAffected()
}
