package games

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
	"golang.org/x/text/unicode/norm"

	"atria/internal/core"
)

// Limits of a campaign file (docs/architecture/cryptogram-campaigns.md, section 8).
const (
	MinPassages       = 3
	MaxPassages       = 10
	MinPassageLetters = 40
	MaxPassageLetters = 200
)

// ImportFile is a campaign file: one book with its campaigns.
type ImportFile struct {
	Book struct {
		Slug      string   `json:"slug"`
		Title     string   `json:"title"`
		Author    string   `json:"author"`
		SourceURL string   `json:"source_url"`
		License   string   `json:"license"`
		Topics    []string `json:"topics"`
	} `json:"book"`
	Campaigns []ImportCampaign `json:"campaigns"`
}

// ImportCampaign is one story in a campaign file.
type ImportCampaign struct {
	Slug     string   `json:"slug"`
	Title    string   `json:"title"`
	Reviewed bool     `json:"reviewed"`
	Topics   []string `json:"topics"`
	Passages []string `json:"passages"`
}

// Plan states of a campaign.
const (
	PlanNew       = "new"
	PlanUnchanged = "unchanged"
	PlanChanged   = "changed"
)

// PlanItem says what the import does with one campaign.
type PlanItem struct {
	Slug  string
	Title string
	State string
	// Players is the number of users with progress on a changed campaign.
	Players int
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ReadImportFile parses and validates a campaign file.
func ReadImportFile(r io.Reader) (*ImportFile, error) {
	var file ImportFile
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, core.NewUserError("invalid campaign file: %v", err)
	}
	if err := file.validate(); err != nil {
		return nil, err
	}
	return &file, nil
}

func (f *ImportFile) validate() error {
	if !slugPattern.MatchString(f.Book.Slug) {
		return core.NewUserError("book slug %q must use a-z, 0-9 and hyphens", f.Book.Slug)
	}
	if strings.TrimSpace(f.Book.Title) == "" {
		return core.NewUserError("the book has no title")
	}
	if len(f.Campaigns) == 0 {
		return core.NewUserError("the file has no campaigns")
	}
	seen := map[string]bool{}
	for _, c := range f.Campaigns {
		name := c.Slug
		if !slugPattern.MatchString(c.Slug) {
			return core.NewUserError("campaign slug %q must use a-z, 0-9 and hyphens", c.Slug)
		}
		if seen[c.Slug] {
			return core.NewUserError("campaign %s is in the file twice", name)
		}
		seen[c.Slug] = true
		if strings.TrimSpace(c.Title) == "" {
			return core.NewUserError("campaign %s has no title", name)
		}
		if !c.Reviewed {
			return core.NewUserError("campaign %s is not reviewed: read it and set \"reviewed\": true", name)
		}
		if len(c.Passages) < MinPassages || len(c.Passages) > MaxPassages {
			return core.NewUserError("campaign %s has %d passages, it must have %d to %d", name, len(c.Passages), MinPassages, MaxPassages)
		}
		for i, passage := range c.Passages {
			letters, err := passageLetters(passage)
			if err != nil {
				return core.NewUserError("campaign %s, passage %d: %v", name, i+1, err)
			}
			if letters < MinPassageLetters || letters > MaxPassageLetters {
				return core.NewUserError("campaign %s, passage %d has %d letters, it must have %d to %d", name, i+1, letters, MinPassageLetters, MaxPassageLetters)
			}
		}
	}
	return nil
}

// ToASCII changes a passage to the letters of the game: "Cæsar" becomes
// "Caesar" and "café" becomes "cafe". Typographic quotes become plain ones.
func ToASCII(text string) string {
	replacer := strings.NewReplacer("æ", "ae", "Æ", "AE", "œ", "oe", "Œ", "OE", "ß", "ss",
		"‘", "'", "’", "'", "“", `"`, "”", `"`, "—", "-", "–", "-")
	decomposed := norm.NFD.String(replacer.Replace(text))
	var b strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// passageLetters counts the letters of a passage and rejects a letter that
// the game cannot show (outside A to Z after ToASCII).
func passageLetters(passage string) (int, error) {
	count := 0
	for _, r := range ToASCII(passage) {
		if !unicode.IsLetter(r) {
			continue
		}
		upper := unicode.ToUpper(r)
		if upper < 'A' || upper > 'Z' {
			return 0, fmt.Errorf("the letter %q is not in A to Z", r)
		}
		count++
	}
	return count, nil
}

func campaignSlug(f *ImportFile, c ImportCampaign) string { return f.Book.Slug + "/" + c.Slug }

func campaignTopics(f *ImportFile, c ImportCampaign) []string {
	topics := append(slices.Clone(f.Book.Topics), c.Topics...)
	for i := range topics {
		topics[i] = strings.ToLower(strings.TrimSpace(topics[i]))
	}
	slices.Sort(topics)
	return slices.Compact(topics)
}

// Plan compares the file with the database. It changes nothing.
func Plan(ctx context.Context, db *sql.DB, f *ImportFile) ([]PlanItem, error) {
	items := make([]PlanItem, 0, len(f.Campaigns))
	for _, c := range f.Campaigns {
		item := PlanItem{Slug: campaignSlug(f, c), Title: c.Title, State: PlanNew}
		var id uuid.UUID
		err := db.QueryRowContext(ctx, `SELECT id FROM game_campaigns WHERE slug = $1`, item.Slug).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("find campaign %s: %w", item.Slug, err)
		}
		if err == nil {
			var existing []string
			if err := db.QueryRowContext(ctx, `
				SELECT COALESCE(array_agg(display_text ORDER BY order_index), '{}') FROM game_cryptograms WHERE campaign_id = $1
			`, id).Scan(pq.Array(&existing)); err != nil {
				return nil, fmt.Errorf("load campaign %s: %w", item.Slug, err)
			}
			item.State = PlanUnchanged
			if !slices.Equal(existing, c.Passages) {
				item.State = PlanChanged
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(DISTINCT p.user_id) FROM game_cryptogram_progress p
					JOIN game_cryptograms c ON c.id = p.cryptogram_id WHERE c.campaign_id = $1
				`, id).Scan(&item.Players); err != nil {
					return nil, fmt.Errorf("count players of %s: %w", item.Slug, err)
				}
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// Import adds the new campaigns of a file in one transaction. A changed
// campaign stops the import, unless replace is set: then the import deletes
// the campaign with the progress of all users and imports it again.
func Import(ctx context.Context, db *sql.DB, f *ImportFile, replace bool) ([]PlanItem, error) {
	plan, err := Plan(ctx, db, f)
	if err != nil {
		return nil, err
	}
	if !replace {
		for _, item := range plan {
			if item.State == PlanChanged {
				return plan, core.NewUserError("campaign %s is in the database with other passages; use --replace to import it again", item.Slug)
			}
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin campaign import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, c := range f.Campaigns {
		item := plan[i]
		if item.State == PlanUnchanged {
			continue
		}
		if item.State == PlanChanged {
			if _, err := tx.ExecContext(ctx, `DELETE FROM game_campaigns WHERE slug = $1`, item.Slug); err != nil {
				return nil, fmt.Errorf("delete campaign %s: %w", item.Slug, err)
			}
		}
		if err := insertCampaign(ctx, tx, f, c, i); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit campaign import: %w", err)
	}
	return plan, nil
}

func insertCampaign(ctx context.Context, tx *sql.Tx, f *ImportFile, c ImportCampaign, position int) error {
	campaignID := core.NewUUID()
	topics := campaignTopics(f, c)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO game_campaigns (id, slug, title, book_title, book_author, source_url, license, position, topics)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9)
	`, campaignID, campaignSlug(f, c), c.Title, f.Book.Title, f.Book.Author, f.Book.SourceURL, f.Book.License, position, pq.Array(topics))
	if err != nil {
		return fmt.Errorf("insert campaign %s: %w", c.Slug, err)
	}
	for i, passage := range c.Passages {
		letters, _ := passageLetters(passage)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO game_cryptograms (id, title, category, order_index, plain_text, display_text, difficulty_score, campaign_id, topics)
			VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9)
		`, core.NewUUID(), fmt.Sprintf("%s (%d/%d)", c.Title, i+1, len(c.Passages)), f.Book.Author, i,
			strings.ToUpper(ToASCII(passage)), passage, difficultyFor(letters), campaignID, pq.Array(topics))
		if err != nil {
			return fmt.Errorf("insert passage %d of %s: %w", i+1, c.Slug, err)
		}
	}
	return nil
}

// difficultyFor estimates the difficulty from the length: a longer passage
// has more repeated letters, so it is easier to start but takes longer.
func difficultyFor(letters int) int {
	switch {
	case letters < 70:
		return 2
	case letters < 130:
		return 3
	default:
		return 4
	}
}
