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
	BookTitle  string
	BookAuthor *string
	Topics     []string
	Total      int
	Solved     int
	// NextID is the first unsolved passage, or nil if the campaign is solved.
	NextID *uuid.UUID
}

// Done reports whether the user solved all passages.
func (c CampaignSummary) Done() bool { return c.Total > 0 && c.Solved == c.Total }

// Campaign is a campaign page: the summary, the source and the story so far.
type Campaign struct {
	CampaignSummary
	SourceURL *string
	License   *string
	// Story holds the solved passages in normal case, in the order of the story.
	Story []string
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

// ListCampaigns returns the campaigns with the progress of a user. An empty
// topic returns all campaigns.
func ListCampaigns(ctx context.Context, db *sql.DB, userID uuid.UUID, topic string) ([]CampaignSummary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT g.id, g.title, g.book_title, g.book_author, g.topics,
		       COUNT(c.id), COUNT(p.completed_at),
		       (SELECT c2.id FROM game_cryptograms c2
		        LEFT JOIN game_cryptogram_progress p2 ON p2.cryptogram_id = c2.id AND p2.user_id = $1
		        WHERE c2.campaign_id = g.id AND p2.completed_at IS NULL
		        ORDER BY c2.order_index LIMIT 1)
		FROM game_campaigns g
		JOIN game_cryptograms c ON c.campaign_id = g.id
		LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $1
		WHERE $2 = '' OR $2 = ANY(g.topics)
		GROUP BY g.id
		ORDER BY g.book_title, g.position, g.title
	`, userID, topic)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	var campaigns []CampaignSummary
	for rows.Next() {
		var c CampaignSummary
		if err := rows.Scan(&c.ID, &c.Title, &c.BookTitle, &c.BookAuthor, pq.Array(&c.Topics), &c.Total, &c.Solved, &c.NextID); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// GetCampaign returns a campaign page for a user.
func GetCampaign(ctx context.Context, db *sql.DB, userID, campaignID uuid.UUID) (*Campaign, error) {
	var c Campaign
	err := db.QueryRowContext(ctx, `
		SELECT id, title, book_title, book_author, topics, source_url, license
		FROM game_campaigns WHERE id = $1
	`, campaignID).Scan(&c.ID, &c.Title, &c.BookTitle, &c.BookAuthor, pq.Array(&c.Topics), &c.SourceURL, &c.License)
	if err == sql.ErrNoRows {
		return nil, &NotFoundError{}
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.display_text, c.plain_text), p.completed_at IS NOT NULL
		FROM game_cryptograms c
		LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $2
		WHERE c.campaign_id = $1
		ORDER BY c.order_index
	`, campaignID, userID)
	if err != nil {
		return nil, fmt.Errorf("load campaign passages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var text string
		var solved bool
		if err := rows.Scan(&id, &text, &solved); err != nil {
			return nil, fmt.Errorf("scan campaign passage: %w", err)
		}
		c.Total++
		if solved {
			c.Solved++
			c.Story = append(c.Story, text)
		} else if c.NextID == nil {
			next := id
			c.NextID = &next
		}
	}
	return &c, rows.Err()
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
	// previousSolved is NULL for the first passage, which is always open.
	if previousSolved.Valid && !previousSolved.Bool {
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
