package articles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"atria/internal/core"
	"atria/internal/netutil"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

// DuplicateArticleError is returned when saving an article would violate the
// unique (owner, type, title, parent) constraint on entities - typically
// because the source page's title could not be parsed correctly and
// collided with an already-saved entity of the same title.
type DuplicateArticleError struct {
	ExistingID    uuid.UUID
	ExistingTitle string
}

func (e *DuplicateArticleError) Error() string {
	return fmt.Sprintf("an entity titled %q already exists", e.ExistingTitle)
}

// ArticleSummary is a lightweight struct for listing articles.
type ArticleSummary struct {
	ID         uuid.UUID
	Title      string
	Domain     string
	CreatedAt  time.Time
	IsArchived bool
	CharCount  int
}

// commentSectionTokens are exact id/class-name tokens (case-insensitive,
// after splitting on non-alphanumeric characters like "-"/"_") that
// identify elements wrapping reader comments/discussion threads (e.g. Czech
// "diskuze"/"diskuse", Disqus embeds, WordPress-style #comments). These
// elements are stripped before running readability, since a long discussion
// thread can otherwise out-score the actual article body and get extracted
// instead of it.
//
// Matching is done on whole tokens rather than substrings so that unrelated
// words merely containing one of these as a fragment (e.g. a "commentary"
// class for an opinion-piece article) aren't accidentally swept away too.
var commentSectionTokens = map[string]bool{
	"diskuze":  true,
	"diskuse":  true,
	"diskuzni": true,
	"diskusni": true,
	"disqus":   true,
	"comment":  true,
	"comments": true,
	"respond":  true, // WordPress' reply form, always adjacent to its comments
}

// tokenSplitter splits id/class attribute values into individual words,
// e.g. "clanky_diskuse" -> ["clanky", "diskuse"] or "js-comments" ->
// ["js", "comments"].
var tokenSplitter = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// commentSectionHeadingLabels are common label texts for a lone heading
// element (e.g. <h2>Diskuze:</h2> or <div><h2>Comments</h2></div>) that
// typically introduces a comment/discussion container. When found
// immediately before an element removed by stripCommentSections, they're
// removed too, so a dangling "Diskuze:" label doesn't leak into the
// extracted article body.
var commentSectionHeadingLabels = map[string]bool{
	"diskuze":    true,
	"diskuse":    true,
	"komentare":  true,
	"comments":   true,
	"discussion": true,
}

// stripCommentSections removes elements that look like comment/discussion
// containers from the document, in place.
func stripCommentSections(doc *goquery.Document) {
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		combined := strings.ToLower(id + " " + class)
		for _, token := range tokenSplitter.Split(combined, -1) {
			if commentSectionTokens[token] {
				if prev := s.Prev(); prev.Length() > 0 {
					label := strings.ToLower(strings.TrimSpace(prev.Text()))
					label = strings.TrimRight(label, ": \t\n")
					if commentSectionHeadingLabels[label] {
						prev.Remove()
					}
				}
				s.Remove()
				return
			}
		}
	})
}

// normalizeForCompare lowercases s and strips everything but letters/digits,
// so titles like "OSEL.CZ" and domains like "osel.cz" can be compared
// regardless of punctuation and casing.
func normalizeForCompare(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksLikeSiteName reports whether title is empty or effectively just the
// site's domain/name (e.g. readability falling back to "OSEL.CZ" because the
// page's <title> tag has no recognizable headline segment). Such titles are
// unhelpful and prone to collide across different articles from the same
// domain, triggering the duplicate-title constraint.
func looksLikeSiteName(title, domain string) bool {
	nt := normalizeForCompare(title)
	if nt == "" {
		return true
	}
	if nt == normalizeForCompare(domain) {
		return true
	}
	if label := strings.SplitN(domain, ".", 2)[0]; nt == normalizeForCompare(label) {
		return true
	}
	return false
}

// fetchAndParseArticle fetches urlStr, strips known lazy-loaded image and
// comment-section markup, then extracts the article with readability. If
// readability's title looks like just the site name (a common failure mode
// on pages whose <title> tag is "SiteName - Headline" but lack a matching
// <h1>), it falls back to the page's Open Graph / Twitter Card title.
func fetchAndParseArticle(ctx context.Context, urlStr string) (*readability.Article, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid URL: %w", err)
	}

	client := netutil.SafeHTTPClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	limitReader := io.LimitReader(resp.Body, 6*1024*1024) // 6MB limit
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	return processArticleHTML(string(bodyBytes), parsedURL)
}

// processArticleHTML strips known lazy-loaded image and comment-section
// markup from htmlStr, then extracts the article with readability. If
// readability's title looks like just the site name (a common failure mode
// on pages whose <title> tag is "SiteName - Headline" but lack a matching
// <h1>), it falls back to the page's Open Graph / Twitter Card title.
//
// It's split out from fetchAndParseArticle so the parsing logic can be
// exercised directly in tests against saved HTML fixtures, without needing
// a live HTTP fetch.
func processArticleHTML(htmlStr string, parsedURL *url.URL) (*readability.Article, string, error) {
	domain := strings.TrimPrefix(parsedURL.Host, "www.")

	var ogTitle, twitterTitle string
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

		stripCommentSections(doc)

		ogTitle, _ = doc.Find(`meta[property="og:title"]`).First().Attr("content")
		twitterTitle, _ = doc.Find(`meta[name="twitter:title"]`).First().Attr("content")

		if fixedHTML, err := doc.Html(); err == nil {
			htmlStr = fixedHTML
		}
	}

	parsedArticle, err := readability.FromReader(strings.NewReader(htmlStr), parsedURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse article: %w", err)
	}

	title := parsedArticle.Title
	if looksLikeSiteName(title, domain) {
		if ogTitle = strings.TrimSpace(ogTitle); ogTitle != "" && !looksLikeSiteName(ogTitle, domain) {
			title = ogTitle
		} else if twitterTitle = strings.TrimSpace(twitterTitle); twitterTitle != "" && !looksLikeSiteName(twitterTitle, domain) {
			title = twitterTitle
		}
	}
	if title == "" {
		title = "Untitled Article"
	}

	return &parsedArticle, title, nil
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

	parsedArticle, title, err := fetchAndParseArticle(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	// 3. Prepare data for the Entity table

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
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "idx_unique_active_entity" {
			if existingID, findErr := findActiveEntityByTitle(ctx, db, ownerID, core.TypeArticle, title); findErr == nil {
				return nil, &DuplicateArticleError{ExistingID: existingID, ExistingTitle: title}
			}
		}
		return nil, fmt.Errorf("failed to insert entity: %w", err)
	}

	// Save article-specific data
	queryArticle := `
        INSERT INTO articles (id, original_url, domain, html_content, text_content, user_note)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err = tx.ExecContext(ctx, queryArticle,
		entityID, urlStr, domain, parsedArticle.Content, parsedArticle.TextContent, userNote)
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

// findActiveEntityByTitle looks up the still-active entity that collides with
// the given owner/type/title/parent combination.
func findActiveEntityByTitle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityType core.EntityType, title string) (uuid.UUID, error) {
	var id uuid.UUID
	query := `
		SELECT id FROM entities
		WHERE owner_id = $1 AND type = $2 AND title = $3 AND parent_id IS NULL AND deleted_at IS NULL
		LIMIT 1
	`
	err := db.QueryRowContext(ctx, query, ownerID, entityType, title).Scan(&id)
	return id, err
}

// FindArticleByURL returns the entity for an existing article saved from the given URL.
func FindArticleByURL(ctx context.Context, db *sql.DB, ownerID uuid.UUID, urlStr string) (*core.Entity, error) {
	var e core.Entity
	query := `
		SELECT e.id, e.owner_id, e.type, e.title, e.slug, e.created_at
		FROM entities e
		JOIN articles a ON a.id = e.id
		WHERE e.owner_id = $1
		  AND a.original_url = $2
		  AND e.deleted_at IS NULL
		LIMIT 1
	`
	err := db.QueryRowContext(ctx, query, ownerID, urlStr).Scan(
		&e.ID, &e.OwnerID, &e.Type, &e.Title, &e.Slug, &e.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("article not found for url: %s", urlStr)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find article by url: %w", err)
	}
	return &e, nil
}

// GetArticle loads the full article entity including content and metadata.
func GetArticle(ctx context.Context, db *sql.DB, id, ownerID uuid.UUID) (*core.Article, error) {
	query := `
		SELECT id, short_id, parent_id, owner_id, type, visibility, title, slug, path, created_at, updated_at, deleted_at,
		       original_url, domain, is_archived, user_note, html_content, text_content
		FROM articles_full_view
		WHERE id = $1 AND owner_id = $2
	`
	var a core.Article
	err := db.QueryRowContext(ctx, query, id, ownerID).Scan(
		&a.ID, &a.ShortID, &a.ParentID, &a.OwnerID, &a.Type, &a.Visibility, &a.Title, &a.Slug, &a.Path, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
		&a.OriginalURL, &a.Domain, &a.IsArchived, &a.UserNote, &a.HTMLContent, &a.TextContent,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("article not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch article: %w", err)
	}
	return &a, nil
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
			SELECT id, title, domain, created_at, is_archived, COALESCE(LENGTH(text_content), 0)
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
		if err := rows.Scan(&a.ID, &a.Title, &a.Domain, &a.CreatedAt, &a.IsArchived, &a.CharCount); err != nil {
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

// RefetchArticle re-fetches the article content from its original URL and updates the database.
func RefetchArticle(ctx context.Context, db *sql.DB, ownerID, articleID uuid.UUID) error {
	var originalURL string
	err := db.QueryRowContext(ctx, "SELECT original_url FROM articles WHERE id = $1", articleID).Scan(&originalURL)
	if err != nil {
		return fmt.Errorf("failed to find original URL: %w", err)
	}

	parsedArticle, title, err := fetchAndParseArticle(ctx, originalURL)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "UPDATE entities SET title = $1, updated_at = NOW() WHERE id = $2", title, articleID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, "UPDATE articles SET html_content = $1, text_content = $2 WHERE id = $3",
		parsedArticle.Content, parsedArticle.TextContent, articleID)
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
