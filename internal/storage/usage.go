// Package storage reports how much space each user's data occupies.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/gofrs/uuid/v5"
)

// Usage is one user's stored data. Text sizes are PostgreSQL's on-disk
// (possibly compressed) column sizes, so they approximate real disk use;
// attachment sizes are the stored file sizes. Items in the trash count too:
// they still occupy space until purged.
type Usage struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`

	AttachmentCount int64 `json:"attachment_count"`
	AttachmentBytes int64 `json:"attachment_bytes"`
	ArticleCount    int64 `json:"article_count"`
	ArticleBytes    int64 `json:"article_bytes"`
	NoteCount       int64 `json:"note_count"`
	NoteBytes       int64 `json:"note_bytes"`
	RSSItemCount    int64 `json:"rss_item_count"`
	RSSBytes        int64 `json:"rss_bytes"`
}

// Add accumulates other into u (for instance-wide totals).
func (u *Usage) Add(other Usage) {
	u.AttachmentCount += other.AttachmentCount
	u.AttachmentBytes += other.AttachmentBytes
	u.ArticleCount += other.ArticleCount
	u.ArticleBytes += other.ArticleBytes
	u.NoteCount += other.NoteCount
	u.NoteBytes += other.NoteBytes
	u.RSSItemCount += other.RSSItemCount
	u.RSSBytes += other.RSSBytes
}

// DatabaseBytes is the text stored in the database (everything but files).
func (u Usage) DatabaseBytes() int64 { return u.ArticleBytes + u.NoteBytes + u.RSSBytes }

// TotalBytes is database text plus attachment files.
func (u Usage) TotalBytes() int64 { return u.DatabaseBytes() + u.AttachmentBytes }

const usageQuery = `
	WITH att AS (
		SELECT owner_id, COUNT(*) AS n, COALESCE(SUM(size_bytes), 0) AS bytes
		FROM attachments GROUP BY owner_id
	), art AS (
		SELECT e.owner_id, COUNT(*) AS n,
		       COALESCE(SUM(COALESCE(pg_column_size(a.html_content), 0)
		                  + COALESCE(pg_column_size(a.text_content), 0)
		                  + COALESCE(pg_column_size(a.user_note), 0)), 0) AS bytes
		FROM articles a JOIN entities e ON e.id = a.id GROUP BY e.owner_id
	), nt AS (
		SELECT e.owner_id, COUNT(*) AS n,
		       COALESCE(SUM(COALESCE(pg_column_size(n.markdown_content), 0)), 0) AS bytes
		FROM notes n JOIN entities e ON e.id = n.id GROUP BY e.owner_id
	), rss AS (
		SELECT e.owner_id, COUNT(*) AS n,
		       COALESCE(SUM(COALESCE(pg_column_size(i.title), 0)
		                  + COALESCE(pg_column_size(i.description), 0)
		                  + COALESCE(pg_column_size(i.content), 0)), 0) AS bytes
		FROM rss_items i JOIN entities e ON e.id = i.feed_id GROUP BY e.owner_id
	)
	SELECT u.id, u.email, u.display_name,
	       COALESCE(att.n, 0), COALESCE(att.bytes, 0),
	       COALESCE(art.n, 0), COALESCE(art.bytes, 0),
	       COALESCE(nt.n, 0),  COALESCE(nt.bytes, 0),
	       COALESCE(rss.n, 0), COALESCE(rss.bytes, 0)
	FROM users u
	LEFT JOIN att ON att.owner_id = u.id
	LEFT JOIN art ON art.owner_id = u.id
	LEFT JOIN nt  ON nt.owner_id = u.id
	LEFT JOIN rss ON rss.owner_id = u.id
`

func queryUsage(ctx context.Context, db *sql.DB, query string, args ...any) ([]Usage, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query storage usage: %w", err)
	}
	defer rows.Close()

	var list []Usage
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.UserID, &u.Email, &u.DisplayName,
			&u.AttachmentCount, &u.AttachmentBytes,
			&u.ArticleCount, &u.ArticleBytes,
			&u.NoteCount, &u.NoteBytes,
			&u.RSSItemCount, &u.RSSBytes); err != nil {
			return nil, fmt.Errorf("scan storage usage: %w", err)
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// ForUser returns the storage used by one user.
func ForUser(ctx context.Context, db *sql.DB, userID uuid.UUID) (*Usage, error) {
	list, err := queryUsage(ctx, db, usageQuery+` WHERE u.id = $1`, userID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return &Usage{UserID: userID}, nil
	}
	return &list[0], nil
}

// ForAllUsers returns every user's usage, largest total first. Admin only.
func ForAllUsers(ctx context.Context, db *sql.DB) ([]Usage, error) {
	list, err := queryUsage(ctx, db, usageQuery)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].TotalBytes() != list[j].TotalBytes() {
			return list[i].TotalBytes() > list[j].TotalBytes()
		}
		return list[i].Email < list[j].Email
	})
	return list, nil
}

// DatabaseSize returns the total size of the current database, including
// indexes and internal overhead. Admin only.
func DatabaseSize(ctx context.Context, db *sql.DB) (int64, error) {
	var size int64
	err := db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&size)
	return size, err
}
