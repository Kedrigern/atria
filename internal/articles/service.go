package articles

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"atria/internal/core"
	"atria/internal/netutil"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"github.com/gofrs/uuid/v5"
	"github.com/microcosm-cc/bluemonday"
)

// ArticleSummary is a lightweight struct for listing articles.
type ArticleSummary struct {
	ID         uuid.UUID
	Title      string
	Domain     string
	CreatedAt  time.Time
	IsArchived bool
}

// CreateArticle fetches the URL, extracts content using readability, and saves it to the database.
func CreateArticle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, urlStr string, userNote string) (*core.Entity, error) {
	// 1. Extract domain from URL (for clean display and filtering)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	domain := parsedURL.Host
	domain = strings.TrimPrefix(domain, "www.")

	// 2. Fetch and parse the article
	fmt.Printf("Fetching and parsing: %s\n", urlStr)

	client := netutil.SafeHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	htmlStr := string(bodyBytes)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err == nil {
		doc.Find("img[src^='data:image/']").Each(func(i int, s *goquery.Selection) {
			s.RemoveAttr("src")
		})

		doc.Find("img").Each(func(i int, s *goquery.Selection) {
			if dataSrc, exists := s.Attr("data-src"); exists {
				s.SetAttr("src", dataSrc)
			} else if dataLazySrc, exists := s.Attr("data-lazy-src"); exists {
				s.SetAttr("src", dataLazySrc)
			}
		})

		if fixedHTML, err := doc.Html(); err == nil {
			htmlStr = fixedHTML
		}
	}

	parsedArticle, err := readability.FromReader(strings.NewReader(htmlStr), parsedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse article: %w", err)
	}

	p := bluemonday.UGCPolicy()
	cleanContent := p.Sanitize(parsedArticle.Content)

	// 3. Prepare data for the Entity table
	title := parsedArticle.Title
	if title == "" {
		title = "Untitled Article"
	}

	// Sanitize title for a safe slug
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, "\\", "-")
	if len(slug) > 100 {
		slug = slug[:100]
	}

	entityID := core.NewUUID()
	now := time.Now().UTC()

	// 4. Save to database (Transaction)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// ParentID is intentionally NULL - the article goes directly to the "Inbox"
	queryEntity := `
		INSERT INTO entities (id, owner_id, type, visibility, title, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = tx.ExecContext(ctx, queryEntity,
		entityID, ownerID, core.TypeArticle, core.VisibilityPrivate, title, slug, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert entity: %w", err)
	}

	// Save article-specific data
	queryArticle := `
        INSERT INTO articles (id, original_url, domain, html_content, text_content, user_note)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err = tx.ExecContext(ctx, queryArticle,
		entityID, urlStr, domain, cleanContent, parsedArticle.TextContent, userNote)
	if err != nil {
		return nil, fmt.Errorf("failed to insert article data: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &core.Entity{
		ID:        entityID,
		OwnerID:   ownerID,
		Type:      core.TypeArticle,
		Title:     title,
		Slug:      slug,
		CreatedAt: now,
	}, nil
}

// GetArticleText retrieves the plain text content of a saved article.
func GetArticleText(ctx context.Context, db *sql.DB, articleID uuid.UUID) (string, error) {
	var content sql.NullString
	query := `SELECT text_content FROM articles WHERE id = $1`
	err := db.QueryRowContext(ctx, query, articleID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("article content not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch article content: %w", err)
	}
	return content.String, nil
}

// GetArticleHTML retrieves the raw HTML content of a saved article.
func GetArticleHTML(ctx context.Context, db *sql.DB, articleID uuid.UUID) (string, error) {
	var htmlContent sql.NullString
	query := `SELECT html_content FROM articles WHERE id = $1`
	err := db.QueryRowContext(ctx, query, articleID).Scan(&htmlContent)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("article content not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch article HTML: %w", err)
	}
	return htmlContent.String, nil
}

// GetArticleMarkdown retrieves the HTML content and converts it to beautiful Markdown.
func GetArticleMarkdown(ctx context.Context, db *sql.DB, articleID uuid.UUID) (string, error) {
	var htmlContent sql.NullString
	query := `SELECT html_content FROM articles WHERE id = $1`
	err := db.QueryRowContext(ctx, query, articleID).Scan(&htmlContent)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("article content not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to fetch article content: %w", err)
	}

	converter := htmltomarkdown.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(htmlContent.String)
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to Markdown: %w", err)
	}

	return markdown, nil
}

// ListArticles retrieves paginated, unarchived articles for the inbox.
func ListArticles(ctx context.Context, db *sql.DB, ownerID uuid.UUID, limit, offset int) ([]ArticleSummary, error) {
	query := `
			SELECT id, title, domain, created_at, is_archived
			FROM articles_full_view
			WHERE owner_id = $1 AND is_archived = FALSE
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`

	rows, err := db.QueryContext(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list articles: %w", err)
	}
	defer rows.Close()

	var articlesList []ArticleSummary
	for rows.Next() {
		var a ArticleSummary
		if err := rows.Scan(&a.ID, &a.Title, &a.Domain, &a.CreatedAt, &a.IsArchived); err != nil {
			return nil, fmt.Errorf("failed to scan article: %w", err)
		}
		articlesList = append(articlesList, a)
	}
	return articlesList, nil
}

// ArchiveArticle marks an article as archived.
func ArchiveArticle(ctx context.Context, db *sql.DB, ownerID, articleID uuid.UUID) error {
	query := `
		UPDATE articles
		SET is_archived = TRUE
		WHERE id IN (
			SELECT id
			FROM entities
			WHERE id = $1 AND owner_id = $2 AND type = $3 AND deleted_at IS NULL
		)
	`
	_, err := db.ExecContext(ctx, query, articleID, ownerID, core.TypeArticle)
	if err != nil {
		return fmt.Errorf("failed to archive article: %w", err)
	}
	return nil
}

func RefetchArticle(ctx context.Context, db *sql.DB, ownerID, articleID uuid.UUID) error {
	var originalURL string
	err := db.QueryRowContext(ctx, "SELECT original_url FROM articles WHERE id = $1", articleID).Scan(&originalURL)
	if err != nil {
		return fmt.Errorf("failed to find original URL: %w", err)
	}

	parsedURL, _ := url.Parse(originalURL)
	client := netutil.SafeHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, originalURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	htmlStr := string(bodyBytes)

	reFakeSrc := regexp.MustCompile(`(?i)\s+src=["']data:image/[^"']+["']`)
	htmlStr = reFakeSrc.ReplaceAllString(htmlStr, "")
	reDataSrc := regexp.MustCompile(`(?i)\s+data-(?:lazy-)?src=(["'][^"']+["'])`)
	htmlStr = reDataSrc.ReplaceAllString(htmlStr, ` src=$1`)

	parsedArticle, err := readability.FromReader(strings.NewReader(htmlStr), parsedURL)
	if err != nil {
		return err
	}

	p := bluemonday.UGCPolicy()
	cleanContent := p.Sanitize(parsedArticle.Content)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "UPDATE entities SET title = $1, updated_at = NOW() WHERE id = $2", parsedArticle.Title, articleID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, "UPDATE articles SET html_content = $1, text_content = $2 WHERE id = $3",
		cleanContent, parsedArticle.TextContent, articleID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateUserNote update user note for the given article
func UpdateUserNote(ctx context.Context, db *sql.DB, ownerID, articleID uuid.UUID, userNote string) error {
	query := `
		UPDATE articles
		SET user_note = $1
		WHERE id IN (
			SELECT id FROM entities WHERE id = $2 AND owner_id = $3 AND type = $4
		)
	`
	_, err := db.ExecContext(ctx, query, userNote, articleID, ownerID, core.TypeArticle)
	if err != nil {
		return fmt.Errorf("failed to update user note: %w", err)
	}
	return nil
}
