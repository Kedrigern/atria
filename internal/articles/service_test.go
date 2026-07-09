package articles

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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

// syntheticSiteNameTitleFixture builds a fully synthetic HTML page (no real
// scraped content, names, or copyrighted text) that reproduces the two
// osel.cz failure modes reported by a user:
//  1. The <title> tag follows a "SiteName ::  - Headline" pattern with no
//     matching <h1> on the page, which defeats go-readability's title
//     splitting heuristic and makes it fall back to just the site name.
//  2. A large reader discussion thread below the article outweighs the
//     actual article body in readability's content-scoring, so it gets
//     extracted instead of the article.
func syntheticSiteNameTitleFixture() string {
	var comments strings.Builder
	for i := 0; i < 40; i++ {
		comments.WriteString("<p>Toto je ukazkovy testovaci komentar od fiktivniho ctenare, ktery zde pouze zabira misto, aby simuloval objemne diskuzni vlakno pod clankem pro ucely automatizovaneho testu.</p>")
	}

	return `<html><head>
		<title>:: EXAMPLE.CZ ::  - Fiktivni nazev testovaciho clanku o zajimavem tematu</title>
		<meta property="og:title" content="Fiktivni nazev testovaciho clanku o zajimavem tematu">
	</head><body>
		<div class="middle middle-detail">
			<div class="perex">Toto je uvodni odstavec fiktivniho testovaciho clanku, ktery slouzi vyhradne pro ucely automatizovaneho testovani extrakce obsahu.</div>
			<p>Prvni odstavec fiktivniho clanku popisuje smyslenou myslenku, ktera nema nic spolecneho se skutecnym obsahem zadne webove stranky. Text je zde jen proto, aby simuloval typickou delku odstavce novinoveho clanku a poskytl dostatek slov pro spravne rozpoznani hlavniho obsahu.</p>
			<p>Druhy odstavec pokracuje ve fiktivnim vypraveni a pridava dalsi smyslene detaily, aby extrakce obsahu mela dostatek textu k rozpoznani jako hlavni obsah stranky, nikoliv jako diskuzni vlakno pod clankem.</p>
			<p>Zaverecny odstavec fiktivniho clanku shrnuje smyslene zavery a konci tuto testovaci ukazku hlavniho obsahu stranky.</p>
			<div class="zapati_clanku"><h2>Diskuze:</h2></div>
			<div id="clanky_diskuse">` + comments.String() + `</div>
		</div>
	</body></html>`
}

// TestProcessArticleHTML_SiteNameTitleAndCommentThread regression-tests the
// two osel.cz failure modes using a synthetic fixture (see
// syntheticSiteNameTitleFixture) instead of real, scraped page content.
func TestProcessArticleHTML_SiteNameTitleAndCommentThread(t *testing.T) {
	parsedURL, err := url.Parse("https://www.example.cz/some-article.html")
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}

	article, title, err := processArticleHTML(syntheticSiteNameTitleFixture(), parsedURL)
	if err != nil {
		t.Fatalf("processArticleHTML failed: %v", err)
	}

	if strings.EqualFold(title, "example.cz") || strings.EqualFold(title, "example") {
		t.Errorf("title fell back to site name: %q", title)
	}
	const wantTitle = "Fiktivni nazev testovaciho clanku o zajimavem tematu"
	if !strings.Contains(title, wantTitle) {
		t.Errorf("title = %q, want it to contain %q (from og:title)", title, wantTitle)
	}

	if strings.Contains(article.TextContent, "Diskuze:") || strings.Contains(article.TextContent, "testovaci komentar") {
		t.Errorf("article content still contains the discussion thread")
	}
	const wantBodySubstr = "Zaverecny odstavec fiktivniho clanku"
	if !strings.Contains(article.TextContent, wantBodySubstr) {
		t.Errorf("article content is missing expected text %q; got len=%d", wantBodySubstr, len(article.TextContent))
	}
}

// TestFlattenCodeBlocks_LeavesOrdinaryCodeBlocksUntouched guards against the
// per-line-div heuristic being too aggressive: an ordinary code block using
// plain text or inline <span> tokens (the vast majority of sites) has no
// <div>/<p> children and must survive unchanged, newlines and all.
func TestFlattenCodeBlocks_LeavesOrdinaryCodeBlocksUntouched(t *testing.T) {
	htmlDoc := "<html><body><pre><code>func main() {\n\tfmt.Println(&#34;hi&#34;)\n}</code></pre></body></html>"

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlDoc))
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	flattenCodeBlocks(doc)

	got := doc.Find("pre code").First().Text()
	want := "func main() {\n\tfmt.Println(" + `"hi"` + ")\n}"
	if got != want {
		t.Errorf("ordinary code block was altered: got %q, want %q", got, want)
	}
}

// TestFlattenCodeBlocks_JoinsPerLineDivsWithNewlines regression-tests a
// marmelab.com-style code block (reproduced synthetically) where every
// source line is wrapped in its own <div><p>...</p></div> with no literal
// newline between lines. Left untouched, both the plain-text extraction and
// highlight.js's client-side .textContent re-tokenization collapse the
// whole snippet onto a single line.
func TestFlattenCodeBlocks_JoinsPerLineDivsWithNewlines(t *testing.T) {
	htmlDoc := `<html><body><pre data-language="sql"><code>` +
		`<div><p><span>CREATE</span><span> </span><span>TABLE</span><span> </span><span>users</span><span> (</span></p></div>` +
		`<div><p><span>    </span><span>id </span><span>SERIAL</span><span> </span><span>PRIMARY KEY</span><span>,</span></p></div>` +
		`<div><p><span>);</span></p></div>` +
		`</code></pre></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlDoc))
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	flattenCodeBlocks(doc)

	got := doc.Find("pre code").First().Text()
	want := "CREATE TABLE users (\n    id SERIAL PRIMARY KEY,\n);"
	if got != want {
		t.Errorf("flattened code = %q, want %q", got, want)
	}
}

// TestStripCommentSections_DoesNotRemoveLookalikeClasses guards against the
// comment-stripping heuristic being too aggressive: elements that merely
// contain a comment-related substring inside a longer, unrelated word (e.g.
// a "commentary" opinion-piece section) must survive, while actual comment
// containers are still removed.
func TestStripCommentSections_DoesNotRemoveLookalikeClasses(t *testing.T) {
	htmlDoc := `<html><body>
		<div class="commentary-section"><p>This is a real opinion article, not a comment thread.</p></div>
		<div id="comments"><p>Reader comment 1</p><p>Reader comment 2</p></div>
		<div class="js-comments"><p>Reader comment 3</p></div>
	</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlDoc))
	if err != nil {
		t.Fatalf("failed to parse html: %v", err)
	}

	stripCommentSections(doc)

	result, err := doc.Html()
	if err != nil {
		t.Fatalf("failed to serialize html: %v", err)
	}

	if !strings.Contains(result, "real opinion article") {
		t.Errorf("legitimate commentary-section content was incorrectly stripped")
	}
	if strings.Contains(result, "Reader comment 1") || strings.Contains(result, "Reader comment 3") {
		t.Errorf("actual comment sections were not stripped")
	}
}
