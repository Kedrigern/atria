package rss

import (
	"atria/internal/articles"
	"atria/internal/core"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

// FeedToFetch holds the data required to initiate a feed fetch.
type FeedToFetch struct {
	ID           uuid.UUID
	FeedURL      string
	ETag         sql.NullString
	LastMod      sql.NullString
	AuthType     *string
	AuthUsername *string
	AuthToken    *string
}

const (
	fetchInterval    = 1 * time.Hour
	maxFetchInterval = 24 * time.Hour
)

// nextFetchDelay returns how long to wait before the next fetch: the regular
// interval after a success, doubling with every consecutive failure up to
// maxFetchInterval.
func nextFetchDelay(consecutiveFailures int) time.Duration {
	delay := fetchInterval
	for i := 0; i < consecutiveFailures && delay < maxFetchInterval; i++ {
		delay *= 2
	}
	if delay > maxFetchInterval {
		delay = maxFetchInterval
	}
	return delay
}

// UpdateFetchStatus persists the result of a fetch attempt (success or error)
// and schedules the next one, backing off exponentially on repeated failures.
// next_fetch_at is computed in SQL from NOW() so it uses the same clock and
// time zone as the "due" comparison in dueFeedsQuery.
func UpdateFetchStatus(ctx context.Context, db *sql.DB, id uuid.UUID, status int, fetchErr error) error {
	if fetchErr == nil {
		_, err := db.ExecContext(ctx, `
			UPDATE rss_feeds
			SET last_fetch_at = NOW(),
			    last_fetch_status = $1,
			    last_fetch_error = '',
			    consecutive_failures = 0,
			    next_fetch_at = NOW() + make_interval(secs => $2)
			WHERE id = $3`,
			status, nextFetchDelay(0).Seconds(), id)
		return err
	}

	var failures int
	err := db.QueryRowContext(ctx, `
		UPDATE rss_feeds
		SET consecutive_failures = consecutive_failures + 1
		WHERE id = $1
		RETURNING consecutive_failures`, id).Scan(&failures)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		UPDATE rss_feeds
		SET last_fetch_at = NOW(),
		    last_fetch_status = $1,
		    last_fetch_error = $2,
		    next_fetch_at = NOW() + make_interval(secs => $3)
		WHERE id = $4`,
		status, fetchErr.Error(), nextFetchDelay(failures).Seconds(), id)
	return err
}

// CreateFeed initializes a new RSS subscription.
func CreateFeed(ctx context.Context, db *sql.DB, ownerID uuid.UUID, title, feedURL string) (*core.Entity, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	entityID := core.NewUUID()
	slug := core.LimitRunes(strings.ToLower(strings.ReplaceAll(title, " ", "-")), core.MaxSlugLength)
	now := time.Now().UTC()

	// 1. Create the base entity
	queryEntity := `
		INSERT INTO entities (id, owner_id, type, visibility, title, slug, path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = tx.ExecContext(ctx, queryEntity,
		entityID, ownerID, core.TypeRSS, core.VisibilityPrivate, title, slug, "/", now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert entity: %w", err)
	}

	// 2. Create the RSS specific record
	queryFeed := `
		INSERT INTO rss_feeds (id, feed_url, next_fetch_at)
		VALUES ($1, $2, NOW())
	`
	_, err = tx.ExecContext(ctx, queryFeed, entityID, feedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rss feed: %w", err)
	}

	return &core.Entity{ID: entityID, Title: title}, tx.Commit()
}

// ListFeeds retrieves all RSS subscriptions for a user, including their titles.
func ListFeeds(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]core.FeedSummary, error) {
	query := `
		SELECT id, title, feed_url, site_url, last_fetch_at, last_fetch_status, last_fetch_error
		FROM rss_feeds_full_view
		WHERE owner_id = $1
		ORDER BY created_at DESC
		`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []core.FeedSummary
	for rows.Next() {
		var f core.FeedSummary
		err := rows.Scan(&f.ID, &f.Title, &f.FeedURL, &f.SiteURL, &f.LastFetchedAt, &f.LastFetchStatus, &f.LastFetchError)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

// ListItemsToRead retrieves unread items using the database view.
func ListItemsToRead(ctx context.Context, db *sql.DB, ownerID uuid.UUID, limit, offset int) ([]core.RSSItem, error) {
	query := `
		SELECT id, feed_id, source_name, title, link, description, content, published_at, created_at
		FROM rss_to_read_view
		WHERE owner_id = $1
		ORDER BY published_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := db.QueryContext(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query triage items: %w", err)
	}
	defer rows.Close()

	var items []core.RSSItem
	for rows.Next() {
		var i core.RSSItem
		err := rows.Scan(
			&i.ID, &i.FeedID, &i.SourceName, &i.Title,
			&i.Link, &i.Description, &i.Content, &i.PublishedAt, &i.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// SaveItemAsArticle converts an RSS item into a full article and marks it as read.
func SaveItemAsArticle(ctx context.Context, db *sql.DB, ownerID, itemID uuid.UUID) (*core.Entity, error) {
	// 1. Get the link from the RSS item triage
	var link, feedTitle string
	queryGet := `
		SELECT i.link, e.title
		FROM rss_items i
		JOIN entities e ON i.feed_id = e.id
		WHERE i.id = $1 AND e.owner_id = $2
	`
	err := db.QueryRowContext(ctx, queryGet, itemID, ownerID).Scan(&link, &feedTitle)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("rss item not found")
	}
	if err != nil {
		return nil, err
	}

	autoNote := fmt.Sprintf("Uloženo z RSS: %s", feedTitle)

	// 2. Create the article using the articles package
	// This performs the readability extraction and saves it to the DB.
	// If the article was already saved before, find and return the existing one.
	articleEntity, err := articles.CreateArticle(ctx, db, ownerID, link, autoNote)
	if err != nil {
		existing, findErr := articles.FindArticleByURL(ctx, db, ownerID, link)
		if findErr != nil {
			// Return the original error, not the lookup error
			return nil, fmt.Errorf("failed to extract article: %w", err)
		}
		articleEntity = existing
	}

	// 3. Mark the RSS item as read to remove it from the triage view
	if err := MarkAsRead(ctx, db, ownerID, itemID); err != nil {
		return nil, err
	}

	return articleEntity, nil
}

// MarkAsRead updates the read_at timestamp for a specific item.
func MarkAsRead(ctx context.Context, db *sql.DB, ownerID, itemID uuid.UUID) error {
	query := `
		UPDATE rss_items
		SET read_at = COALESCE(read_at, NOW())
		WHERE id = $1 AND feed_id IN (SELECT id FROM entities WHERE owner_id = $2)
	`
	result, err := db.ExecContext(ctx, query, itemID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to mark item as read: %w", err)
	}
	return core.RequireAffected(result, "rss item not found")
}

// ListFeedTags returns all distinct non-system tags attached to any of the user's RSS feeds.
func ListFeedTags(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]core.Tag, error) {
	query := `
		SELECT DISTINCT t.id, t.name, t.color, t.icon
		FROM tags t
		JOIN rel_entity_tags ret ON ret.tag_id = t.id
		JOIN entities e ON e.id = ret.entity_id
		WHERE e.owner_id = $1
		  AND e.type = 'rss'
		  AND e.deleted_at IS NULL
		  AND t.is_system = false
		ORDER BY t.name
	`
	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []core.Tag
	for rows.Next() {
		var t core.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Icon); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListItemsToReadByTag retrieves unread items from feeds tagged with the given tag name.
func ListItemsToReadByTag(ctx context.Context, db *sql.DB, ownerID uuid.UUID, tagName string, limit, offset int) ([]core.RSSItem, error) {
	query := `
		SELECT id, feed_id, source_name, title, link, description, content, published_at, created_at
		FROM rss_to_read_view
		WHERE owner_id = $1
		  AND feed_id IN (
		      SELECT e.id FROM entities e
		      JOIN rel_entity_tags ret ON ret.entity_id = e.id
		      JOIN tags t ON t.id = ret.tag_id
		      WHERE e.owner_id = $1 AND t.name = $4
		  )
		ORDER BY published_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := db.QueryContext(ctx, query, ownerID, limit, offset, tagName)
	if err != nil {
		return nil, fmt.Errorf("failed to query triage items by tag: %w", err)
	}
	defer rows.Close()

	var items []core.RSSItem
	for rows.Next() {
		var i core.RSSItem
		if err := rows.Scan(
			&i.ID, &i.FeedID, &i.SourceName, &i.Title,
			&i.Link, &i.Description, &i.Content, &i.PublishedAt, &i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// MarkBatchAsRead marks a specific list of RSS items as read.
func MarkBatchAsRead(ctx context.Context, db *sql.DB, ownerID uuid.UUID, itemIDs []uuid.UUID) error {
	if len(itemIDs) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		UPDATE rss_items
		SET read_at = NOW()
		WHERE id = $1 AND read_at IS NULL
		  AND feed_id IN (SELECT id FROM entities WHERE owner_id = $2)
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range itemIDs {
		_, err := stmt.ExecContext(ctx, id, ownerID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// FeedDetail holds a feed's metadata, paginated items, and stats.
type FeedDetail struct {
	core.FeedSummary
	Items      []core.RSSItem
	HasMore    bool
	TotalItems int
	ReadItems  int
}

// GetFeedDetail fetches a single feed's info plus its items and read stats.
func GetFeedDetail(ctx context.Context, db *sql.DB, ownerID, feedID uuid.UUID, includeRead bool, limit, offset int) (*FeedDetail, error) {
	var fd FeedDetail

	queryFeed := `
		SELECT id, title, feed_url, site_url, last_fetch_at, last_fetch_status, last_fetch_error
		FROM rss_feeds_full_view
		WHERE id = $1 AND owner_id = $2
	`
	err := db.QueryRowContext(ctx, queryFeed, feedID, ownerID).Scan(
		&fd.ID, &fd.Title, &fd.FeedURL, &fd.SiteURL,
		&fd.LastFetchedAt, &fd.LastFetchStatus, &fd.LastFetchError,
	)
	if err == sql.ErrNoRows {
		return nil, core.NewUserError("feed not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed: %w", err)
	}

	readFilter := "AND ri.read_at IS NULL"
	if includeRead {
		readFilter = ""
	}

	itemsQuery := fmt.Sprintf(`
		SELECT ri.id, ri.feed_id,
		       (SELECT title FROM entities WHERE id = ri.feed_id) AS source_name,
		       ri.title, ri.link,
		       COALESCE(ri.description, '') AS description,
		       COALESCE(ri.content, '') AS content,
		       ri.published_at, ri.created_at, ri.read_at
		FROM rss_items ri
		WHERE ri.feed_id = $1 %s
		ORDER BY ri.published_at DESC
		LIMIT $2 OFFSET $3
	`, readFilter)

	rows, err := db.QueryContext(ctx, itemsQuery, feedID, limit+1, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query feed items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item core.RSSItem
		err := rows.Scan(
			&item.ID, &item.FeedID, &item.SourceName,
			&item.Title, &item.Link, &item.Description, &item.Content,
			&item.PublishedAt, &item.CreatedAt, &item.ReadAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan feed item: %w", err)
		}
		fd.Items = append(fd.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(fd.Items) > limit {
		fd.HasMore = true
		fd.Items = fd.Items[:limit]
	}

	queryStats := `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE read_at IS NOT NULL)
		FROM rss_items
		WHERE feed_id = $1
	`
	err = db.QueryRowContext(ctx, queryStats, feedID).Scan(&fd.TotalItems, &fd.ReadItems)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed stats: %w", err)
	}

	return &fd, nil
}

// MarkFeedAsRead marks all unread items in the given feed as read.
func MarkFeedAsRead(ctx context.Context, db *sql.DB, ownerID, feedID uuid.UUID) error {
	query := `
		UPDATE rss_items
		SET read_at = NOW()
		WHERE feed_id = $1 AND read_at IS NULL
		  AND feed_id IN (SELECT id FROM entities WHERE owner_id = $2)
	`
	_, err := db.ExecContext(ctx, query, feedID, ownerID)
	return err
}
