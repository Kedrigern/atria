package articles

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"atria/internal/core"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"
	"github.com/gofrs/uuid/v5"
)

// CreateArticle fetches the URL, extracts content using readability, and saves it to the database.
func CreateArticle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, urlStr string) (*core.Entity, error) {
	// 1. Extract domain from URL (for clean display and filtering)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	domain := parsedURL.Host
	domain = strings.TrimPrefix(domain, "www.")

	// 2. Fetch and parse the article (with a 15s timeout to prevent hanging)
	fmt.Printf("Fetching and parsing: %s\n", urlStr)
	parsedArticle, err := readability.FromURL(urlStr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to parse article: %w", err)
	}

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
		INSERT INTO articles (id, original_url, domain, html_content, text_content)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err = tx.ExecContext(ctx, queryArticle,
		entityID, urlStr, domain, parsedArticle.Content, parsedArticle.TextContent,
	)
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

// ArticleSummary is a lightweight struct for listing articles.
type ArticleSummary struct {
	ID        uuid.UUID
	Title     string
	Domain    string
	CreatedAt time.Time
}

// ListArticles retrieves articles using the dedicated full view.
func ListArticles(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]ArticleSummary, error) {
	// View vrací metadata i doménu
	query := `SELECT id, title, domain, created_at FROM articles_full_view WHERE owner_id = $1`

	rows, err := db.QueryContext(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list articles: %w", err)
	}
	defer rows.Close()

	var articlesList []ArticleSummary
	for rows.Next() {
		var a ArticleSummary
		// Skenujeme 4 pole definovaná v ArticleSummary
		if err := rows.Scan(&a.ID, &a.Title, &a.Domain, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan article: %w", err)
		}
		articlesList = append(articlesList, a)
	}
	return articlesList, nil
}
