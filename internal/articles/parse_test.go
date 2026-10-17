package articles

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/netutil"
)

func TestParseHTMLUsesTheFullPipeline(t *testing.T) {
	pageURL, _ := url.Parse("https://example.cz/clanek/1")
	parsed, err := ParseHTML(syntheticSiteNameTitleFixture(), pageURL)
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}
	// Same expectations as the processArticleHTML fixture test: the
	// og:title fallback and comment stripping must apply.
	if parsed.Title != "Fiktivni nazev testovaciho clanku o zajimavem tematu" {
		t.Errorf("title = %q", parsed.Title)
	}
	if strings.Contains(parsed.Text, "fiktivniho ctenare") {
		t.Error("comment thread leaked into the extracted text")
	}
	if parsed.URL != pageURL.String() || parsed.Length == 0 || parsed.HTML == "" {
		t.Errorf("incomplete result: %+v", parsed)
	}
}

func TestParseURL(t *testing.T) {
	page := fmt.Sprintf(`<html><head><title>Fetched story</title></head><body><article><h1>Fetched story</h1>%s</article></body></html>`,
		strings.Repeat("<p>Plenty of article text so readability treats this as the main content.</p>", 10))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.Error(w, "<html><body><p>Not found page with some text</p></body></html>", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, page)
	}))
	defer srv.Close()
	ctx := context.WithValue(context.Background(), netutil.AllowLocalKey, true)

	parsed, raw, err := ParseURL(ctx, srv.URL+"/story")
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	if parsed.Title != "Fetched story" || raw != page {
		t.Errorf("title = %q, raw page returned = %v", parsed.Title, raw == page)
	}

	_, _, err = ParseURL(ctx, srv.URL+"/missing")
	if err == nil {
		t.Fatal("a 404 page must not be parsed as an article")
	}
	if msg, ok := core.UserMessage(err); !ok || !strings.Contains(msg, "404") {
		t.Errorf("expected a user-facing 404 error, got %v", err)
	}
}
