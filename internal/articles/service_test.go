package articles

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestLooksLikeSiteName(t *testing.T) {
	cases := []struct {
		title  string
		domain string
		want   bool
	}{
		{"OSEL.CZ", "osel.cz", true},
		{"osel", "osel.cz", true},
		{"", "osel.cz", true},
		{"  :: OSEL.CZ ::  ", "osel.cz", true},
		{"Záhada římského dvanáctistěnu", "osel.cz", false},
		{"Some Article - OSEL.CZ", "osel.cz", false},
	}
	for _, c := range cases {
		if got := looksLikeSiteName(c.title, c.domain); got != c.want {
			t.Errorf("looksLikeSiteName(%q, %q) = %v, want %v", c.title, c.domain, got, c.want)
		}
	}
}

// TestProcessArticleHTML_Osel regression-tests a real page from osel.cz whose
// <title> tag ("ʼ:: OSEL.CZ :: - <headline>ʼ") confuses go-readability's
// title heuristic (it falls back to just "OSEL.CZ"), and whose long reader
// discussion thread otherwise gets extracted instead of the actual article
// body.
func TestProcessArticleHTML_Osel(t *testing.T) {
	htmlBytes, err := os.ReadFile("testdata/osel.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	parsedURL, err := url.Parse("https://www.osel.cz/14780-zahada-rimskeho-dvanactistenu-rozlouskl-300-let-stary-rebus-selsky-rozum-z-vltavskeho-mlyna.html")
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}

	article, title, err := processArticleHTML(string(htmlBytes), parsedURL)
	if err != nil {
		t.Fatalf("processArticleHTML failed: %v", err)
	}

	if strings.EqualFold(title, "osel.cz") || strings.EqualFold(title, "osel") {
		t.Errorf("title fell back to site name: %q", title)
	}
	const wantSubstr = "Záhada římského dvanáctistěnu"
	if !strings.Contains(title, wantSubstr) {
		t.Errorf("title = %q, want it to contain %q (from og:title)", title, wantSubstr)
	}

	if strings.Contains(article.TextContent, "Diskuze:") {
		t.Errorf("article content still contains the discussion thread")
	}
	const articleSubstr = "Galo-římský dodekaedr"
	if !strings.Contains(article.TextContent, articleSubstr) {
		t.Errorf("article content is missing expected text %q; got len=%d", articleSubstr, len(article.TextContent))
	}
}
