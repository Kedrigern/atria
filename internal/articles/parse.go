package articles

import (
	"context"
	"net/url"
	"time"

	"atria/internal/core"

	"github.com/go-shiori/go-readability"
)

// ParsedArticle is the result of the extraction pipeline for one page —
// exactly what CreateArticle would store, without touching the database.
type ParsedArticle struct {
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Byline        string     `json:"byline,omitempty"`
	Excerpt       string     `json:"excerpt,omitempty"`
	SiteName      string     `json:"site_name,omitempty"`
	Language      string     `json:"language,omitempty"`
	PublishedTime *time.Time `json:"published_time,omitempty"`
	Image         string     `json:"image,omitempty"`
	// Length is the number of characters in Text.
	Length int `json:"length"`
	// HTML is the extracted article (stored as html_content). The web UI
	// sanitizes it before display; see core.SanitizeHTML.
	HTML string `json:"html"`
	// Text is the plain text used for search (stored as text_content).
	Text string `json:"text"`
}

func newParsedArticle(pageURL string, a *readability.Article, title string) *ParsedArticle {
	return &ParsedArticle{
		URL:           pageURL,
		Title:         title,
		Byline:        a.Byline,
		Excerpt:       a.Excerpt,
		SiteName:      a.SiteName,
		Language:      a.Language,
		PublishedTime: a.PublishedTime,
		Image:         a.Image,
		Length:        a.Length,
		HTML:          a.Content,
		Text:          a.TextContent,
	}
}

// ParseURL downloads pageURL and extracts the article. It also returns the
// downloaded page, e.g. to save it as a test fixture.
func ParseURL(ctx context.Context, pageURL string) (*ParsedArticle, string, error) {
	body, parsedURL, err := fetchPage(ctx, pageURL)
	if err != nil {
		return nil, "", err
	}
	parsed, err := ParseHTML(body, parsedURL)
	return parsed, body, err
}

// ParseHTML extracts the article from an already downloaded page. pageURL is
// the page's original address: it resolves relative links and images and is
// used to recognise site-name-only titles.
func ParseHTML(page string, pageURL *url.URL) (*ParsedArticle, error) {
	a, title, err := processArticleHTML(page, pageURL)
	if err != nil {
		return nil, core.NewUserError("%w", err)
	}
	return newParsedArticle(pageURL.String(), a, title), nil
}
