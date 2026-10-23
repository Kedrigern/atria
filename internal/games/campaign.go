package games

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

// CampaignSummary is one campaign with the progress of a user.
type CampaignSummary struct {
	ID         uuid.UUID
	Title      string
	BookID     uuid.UUID
	BookSlug   string
	BookTitle  string
	BookAuthor *string
	// BookInOrder means that the campaigns of the book open one by one.
	BookInOrder bool
	Topics      []string
	Total       int
	Solved      int
	// NextID is the first unsolved passage, or nil if the campaign is solved.
	NextID *uuid.UUID
	// LockedBy is the title of the campaign that the user must solve first,
	// or "" if the campaign is open.
	LockedBy string
}

// Done reports whether the user solved all passages.
func (c CampaignSummary) Done() bool { return c.Total > 0 && c.Solved == c.Total }

// Locked reports whether the campaign waits for the campaign before it.
func (c CampaignSummary) Locked() bool { return c.LockedBy != "" }

// Campaign is a campaign page: the summary, the source and the story so far.
type Campaign struct {
	CampaignSummary
	SourceURL *string
	License   *string
	// Story holds the solved passages in normal case, in the order of the story.
	Story []string
}

// Book is a book page: all campaigns of a book with their story so far.
type Book struct {
	ID        uuid.UUID
	Slug      string
	Title     string
	Author    *string
	SourceURL *string
	License   *string
	InOrder   bool
	Chapters  []Campaign
	Total     int
	Solved    int
	// NextID is the next passage to solve in the book, or nil.
	NextID *uuid.UUID
	// Current is the index in Chapters of the chapter with NextID, or of the
	// last chapter with a solved passage.
	Current int
}

// CampaignContext places a passage in its campaign.
type CampaignContext struct {
	ID       uuid.UUID
	Title    string
	Position int
	Total    int
	NextID   *uuid.UUID
}

// LockedError means that the user must solve the earlier passages first.
type LockedError struct {
	CampaignID uuid.UUID
}

func (e *LockedError) Error() string { return "solve the earlier passages first" }

// campaignColumns are the columns that scanSummary reads, for the campaign g
// of the book b. $1 is the user.
const campaignColumns = `g.id, g.title, b.id, b.slug, b.title, b.author, b.in_order, g.topics,
	(SELECT COUNT(*) FROM game_cryptograms c WHERE c.campaign_id = g.id),
	(SELECT COUNT(*) FROM game_cryptograms c JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $1
	 WHERE c.campaign_id = g.id AND p.completed_at IS NOT NULL),
	(SELECT c.id FROM game_cryptograms c
	 LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $1
	 WHERE c.campaign_id = g.id AND p.completed_at IS NULL
	 ORDER BY c.order_index LIMIT 1)`

type scanner interface{ Scan(dest ...any) error }

func scanSummary(row scanner, c *CampaignSummary) error {
	return row.Scan(&c.ID, &c.Title, &c.BookID, &c.BookSlug, &c.BookTitle, &c.BookAuthor, &c.BookInOrder,
		pq.Array(&c.Topics), &c.Total, &c.Solved, &c.NextID)
}

// markLocks sets LockedBy for the campaigns of books in order. The campaigns
// must be sorted by book and position.
func markLocks(campaigns []CampaignSummary) {
	for i := range campaigns {
		if i == 0 || !campaigns[i].BookInOrder || campaigns[i-1].BookID != campaigns[i].BookID {
			continue
		}
		previous := campaigns[i-1]
		if !previous.Done() {
			campaigns[i].LockedBy = previous.Title
		}
	}
}

// ListCampaigns returns the campaigns with the progress of a user. An empty
// topic returns all campaigns.
func ListCampaigns(ctx context.Context, db *sql.DB, userID uuid.UUID, topic string) ([]CampaignSummary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+campaignColumns+`
		FROM game_campaigns g JOIN game_books b ON b.id = g.book_id
		ORDER BY b.title, g.position, g.title
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	var all []CampaignSummary
	for rows.Next() {
		var c CampaignSummary
		if err := scanSummary(rows, &c); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The locks need all campaigns of a book, so the topic filter comes after.
	markLocks(all)
	if topic == "" {
		return all, nil
	}
	var filtered []CampaignSummary
	for _, c := range all {
		for _, t := range c.Topics {
			if t == topic {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered, nil
}

// loadStory fills the story so far and the progress of a campaign.
func loadStory(ctx context.Context, db *sql.DB, userID uuid.UUID, c *Campaign) error {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(c.display_text, c.plain_text), p.completed_at IS NOT NULL
		FROM game_cryptograms c
		LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $2
		WHERE c.campaign_id = $1
		ORDER BY c.order_index
	`, c.ID, userID)
	if err != nil {
		return fmt.Errorf("load campaign passages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		var solved bool
		if err := rows.Scan(&text, &solved); err != nil {
			return fmt.Errorf("scan campaign passage: %w", err)
		}
		if solved {
			c.Story = append(c.Story, text)
		}
	}
	return rows.Err()
}

// GetCampaign returns a campaign page for a user.
func GetCampaign(ctx context.Context, db *sql.DB, userID, campaignID uuid.UUID) (*Campaign, error) {
	var c Campaign
	err := scanSummary(db.QueryRowContext(ctx, `
		SELECT `+campaignColumns+`
		FROM game_campaigns g JOIN game_books b ON b.id = g.book_id
		WHERE g.id = $2
	`, userID, campaignID), &c.CampaignSummary)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{}
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT source_url, license FROM game_books WHERE id = $1`, c.BookID).
		Scan(&c.SourceURL, &c.License); err != nil {
		return nil, fmt.Errorf("load book of campaign: %w", err)
	}
	if c.LockedBy, err = lockedBy(ctx, db, userID, campaignID); err != nil {
		return nil, err
	}
	if err := loadStory(ctx, db, userID, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetBook returns the book page of a user: all campaigns of a book in order,
// each with its story so far.
func GetBook(ctx context.Context, db *sql.DB, userID uuid.UUID, slug string) (*Book, error) {
	var b Book
	err := db.QueryRowContext(ctx, `
		SELECT id, slug, title, author, source_url, license, in_order FROM game_books WHERE slug = $1
	`, slug).Scan(&b.ID, &b.Slug, &b.Title, &b.Author, &b.SourceURL, &b.License, &b.InOrder)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{}
	}
	if err != nil {
		return nil, fmt.Errorf("load book: %w", err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+campaignColumns+`
		FROM game_campaigns g JOIN game_books b ON b.id = g.book_id
		WHERE b.id = $2
		ORDER BY g.position, g.title
	`, userID, b.ID)
	if err != nil {
		return nil, fmt.Errorf("list book campaigns: %w", err)
	}
	var summaries []CampaignSummary
	for rows.Next() {
		var c CampaignSummary
		if err := scanSummary(rows, &c); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		summaries = append(summaries, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	markLocks(summaries)
	for i, summary := range summaries {
		chapter := Campaign{CampaignSummary: summary}
		if err := loadStory(ctx, db, userID, &chapter); err != nil {
			return nil, err
		}
		b.Total += chapter.Total
		b.Solved += chapter.Solved
		if b.NextID == nil && chapter.NextID != nil && !chapter.Locked() {
			b.NextID, b.Current = chapter.NextID, i
		} else if b.NextID == nil && chapter.Solved > 0 {
			b.Current = i
		}
		b.Chapters = append(b.Chapters, chapter)
	}
	return &b, nil
}

// lockedBy returns the title of the campaign that the user must solve before
// the campaign opens, or "" if it is open. Only books in order have locks.
func lockedBy(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, userID, campaignID uuid.UUID) (string, error) {
	var inOrder bool
	var previousTitle sql.NullString
	var unsolved int
	err := q.QueryRowContext(ctx, `
		SELECT b.in_order, prev.title,
		       (SELECT COUNT(*) FROM game_cryptograms c
		        LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $2
		        WHERE c.campaign_id = prev.id AND p.completed_at IS NULL)
		FROM game_campaigns g
		JOIN game_books b ON b.id = g.book_id
		LEFT JOIN LATERAL (
			SELECT id, title FROM game_campaigns
			WHERE book_id = g.book_id AND position < g.position
			ORDER BY position DESC LIMIT 1
		) prev ON TRUE
		WHERE g.id = $1
	`, campaignID, userID).Scan(&inOrder, &previousTitle, &unsolved)
	if err != nil {
		return "", fmt.Errorf("check campaign order: %w", err)
	}
	if inOrder && previousTitle.Valid && unsolved > 0 {
		return previousTitle.String, nil
	}
	return "", nil
}

// campaignContext loads the place of a passage in its campaign and checks the
// lock: a passage opens only after the user solved the passage before it.
func campaignContext(ctx context.Context, tx *sql.Tx, userID, cryptogramID uuid.UUID) (*CampaignContext, error) {
	var campaignID uuid.NullUUID
	var position sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT campaign_id, order_index FROM game_cryptograms WHERE id = $1`, cryptogramID).
		Scan(&campaignID, &position)
	if err != nil {
		return nil, fmt.Errorf("load campaign of cryptogram: %w", err)
	}
	if !campaignID.Valid {
		return nil, nil
	}

	cc := &CampaignContext{ID: campaignID.UUID}
	var previousSolved sql.NullBool
	err = tx.QueryRowContext(ctx, `
		SELECT g.title,
		       (SELECT COUNT(*) FROM game_cryptograms WHERE campaign_id = g.id),
		       (SELECT COUNT(*) FROM game_cryptograms WHERE campaign_id = g.id AND order_index < $2),
		       (SELECT id FROM game_cryptograms WHERE campaign_id = g.id AND order_index > $2 ORDER BY order_index LIMIT 1),
		       (SELECT p.completed_at IS NOT NULL
		        FROM game_cryptograms c LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $3
		        WHERE c.campaign_id = g.id AND c.order_index < $2 ORDER BY c.order_index DESC LIMIT 1)
		FROM game_campaigns g WHERE g.id = $1
	`, campaignID.UUID, position.Int64, userID).Scan(&cc.Title, &cc.Total, &cc.Position, &cc.NextID, &previousSolved)
	if err != nil {
		return nil, fmt.Errorf("load campaign context: %w", err)
	}
	cc.Position++
	// previousSolved is NULL for the first passage, which is open if the
	// campaign is open.
	if previousSolved.Valid && !previousSolved.Bool {
		return nil, &LockedError{CampaignID: campaignID.UUID}
	}
	if locked, err := lockedBy(ctx, tx, userID, campaignID.UUID); err != nil {
		return nil, err
	} else if locked != "" {
		return nil, &LockedError{CampaignID: campaignID.UUID}
	}
	return cc, nil
}

// Topics returns the topics of the campaigns, or of the single puzzles, sorted.
func Topics(ctx context.Context, db *sql.DB, campaigns bool) ([]string, error) {
	query := `SELECT DISTINCT unnest(topics) AS t FROM game_cryptograms WHERE campaign_id IS NULL ORDER BY t`
	if campaigns {
		query = `SELECT DISTINCT unnest(topics) AS t FROM game_campaigns ORDER BY t`
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	var topics []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// TopicLabel turns a topic such as "fairy-tales" into "Fairy tales".
func TopicLabel(topic string) string {
	label := strings.ReplaceAll(topic, "-", " ")
	if label == "" {
		return label
	}
	return strings.ToUpper(label[:1]) + label[1:]
}
