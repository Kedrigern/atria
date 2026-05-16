package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gofrs/uuid/v5"
)

// SearchResult holds a single hit returned by Search.
type SearchResult struct {
	ID       uuid.UUID
	Type     string // "note", "article", "rss_item"
	Title    string
	Headline string // ts_headline snippet
	Rank     float64
	OwnerID  uuid.UUID
}

const tsHeadlineOpts = `MaxWords=35, MinWords=15, ShortWord=3, HighlightAll=false, MaxFragments=2, FragmentDelimiter=" … "`

// notesSubquery is a sub-SELECT for notes.
// $1 = search query string, $2 = ownerID.
const notesSubquery = `
SELECT
    e.id,
    'note'                                                    AS type,
    e.title,
    ts_headline('english', n.markdown_content, q, '` + tsHeadlineOpts + `') AS headline,
    ts_rank(n.search_vector, q)                               AS rank,
    e.owner_id
FROM notes n
JOIN entities e ON e.id = n.id
, websearch_to_tsquery('english', $1) q
WHERE n.search_vector @@ q
  AND e.deleted_at IS NULL
  AND (e.owner_id = $2 OR e.visibility IN ('users', 'public'))`

// articlesSubqueryBase is the articles sub-SELECT without an archived filter.
// $1 = search query string, $2 = ownerID.
const articlesSubqueryBase = `
SELECT
    e.id,
    'article'                                                                              AS type,
    e.title,
    ts_headline('english', COALESCE(a.text_content, a.user_note, ''), q, '` + tsHeadlineOpts + `') AS headline,
    ts_rank(a.search_vector, q)                                                            AS rank,
    e.owner_id
FROM articles a
JOIN entities e ON e.id = a.id
, websearch_to_tsquery('english', $1) q
WHERE a.search_vector @@ q
  AND e.deleted_at IS NULL
  AND (e.owner_id = $2 OR e.visibility IN ('users', 'public'))`

// rssItemsSubqueryBase is the RSS items sub-SELECT without a read_at filter.
// $1 = search query string, $2 = ownerID.
const rssItemsSubqueryBase = `
SELECT
    ri.id,
    'rss_item'                                                                           AS type,
    ri.title,
    ts_headline('english', COALESCE(ri.content, ri.description, ''), q, '` + tsHeadlineOpts + `') AS headline,
    ts_rank(ri.search_vector, q)                                                         AS rank,
    f_entity.owner_id
FROM rss_items ri
JOIN rss_feeds rf ON rf.id = ri.feed_id
JOIN entities f_entity ON f_entity.id = rf.id
, websearch_to_tsquery('english', $1) q
WHERE ri.search_vector @@ q
  AND f_entity.owner_id = $2`

// Search performs a full-text search across notes, articles, and/or rss_items.
//
// filter selects which content types to search:
//   - ""         → all three types
//   - "notes"    → notes only
//   - "articles" → articles only
//   - "rss"      → rss_items only
//
// When includeArchived is false, archived articles and unread-only RSS items are
// excluded. When true, all matching content is returned regardless of archived/read state.
//
// Results are ranked by ts_rank DESC and capped at 50.
// Returns (nil, nil) when query is blank after trimming.
func Search(ctx context.Context, db *sql.DB, ownerID uuid.UUID, query string, filter string, includeArchived bool) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// Build dynamic sub-queries based on includeArchived.
	articlesSubquery := articlesSubqueryBase
	if !includeArchived {
		articlesSubquery += "\n  AND a.is_archived = false"
	}

	rssItemsSubquery := rssItemsSubqueryBase
	if !includeArchived {
		rssItemsSubquery += "\n  AND ri.read_at IS NULL"
	}

	// Collect the sub-queries that are relevant for this filter.
	var parts []string
	switch strings.ToLower(filter) {
	case "notes":
		parts = append(parts, notesSubquery)
	case "articles":
		parts = append(parts, articlesSubquery)
	case "rss":
		parts = append(parts, rssItemsSubquery)
	default: // "" or anything unrecognised → search everything
		parts = append(parts, notesSubquery, articlesSubquery, rssItemsSubquery)
	}

	// Wrap the union in an outer SELECT with ordering and a hard limit.
	sqlStr := fmt.Sprintf(
		`SELECT id, type, title, headline, rank, owner_id FROM (%s) combined ORDER BY rank DESC LIMIT 50`,
		strings.Join(parts, "\nUNION ALL\n"),
	)

	rows, err := db.QueryContext(ctx, sqlStr, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Headline, &r.Rank, &r.OwnerID); err != nil {
			return nil, fmt.Errorf("search scan failed: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows error: %w", err)
	}

	return results, nil
}
