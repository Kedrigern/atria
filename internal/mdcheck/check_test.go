package mdcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The marker forms from docs/architecture/manuscripts.md §3: an HTML comment
// with an Arabic number, optionally after a page word, or a Roman number for
// front matter. The PDF converter adds "(pdf N)" and writes "?" for a page
// with an unknown number. Other text is not a marker; a comment with a digit
// in an unknown form gets a warning.
func TestPageMarkerForms(t *testing.T) {
	cases := []struct {
		marker      string
		arabic      bool // the marker counts as an Arabic page of the original
		pageWarning bool // a [pages] warning
	}{
		{"<!-- 15 -->", true, false},
		{"<!--15-->", true, false},
		{"<!-- p. 15 -->", true, false},
		{"<!-- P. 15 -->", true, false},
		{"<!-- s. 15 -->", true, false},
		{"<!-- str. 15 -->", true, false},
		{"<!-- page 15 -->", true, false},
		{"<!-- page 41 (pdf 38) -->", true, false},
		{"<!-- page ? (pdf 12) -->", false, true},
		{"<!-- page v (pdf 5) -->", false, false},
		{"<!-- xiv -->", false, false},
		{"<!-- p. XIV -->", false, false},
		{"[15]", false, false},
		{"<!-- poznámka redakce -->", false, false},
		{"<!-- 15a -->", false, true},
		{"<!-- see page 15 -->", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.marker, func(t *testing.T) {
			report := Check([]byte("# Kniha\n\n" + tc.marker + "\nText strany.\n"))
			assert.Equal(t, tc.arabic, report.OriginalPages > 0, "marker %q counts as an Arabic page", tc.marker)
			warned := false
			for _, w := range report.Warnings {
				if w.Rule == RulePages {
					warned = true
				}
			}
			assert.Equal(t, tc.pageWarning, warned, "page warning for %q", tc.marker)
		})
	}
}

// A marker starts its page (the convention of the PDF converter). The page
// count runs from the first to the highest Arabic marker; body text before
// the first marker is the page before it. Gaps are reported, also around
// pages whose printed number is not known ("?").
func TestPageSequence(t *testing.T) {
	cases := []struct {
		name     string
		doc      string
		pages    int
		warnings []string
	}{
		{
			name:  "title page without a marker before page 2",
			doc:   "# Pracující suverén\n\nAxel Honneth\n\n<!-- page 2 -->\nTiráž.\n<!-- page 3 -->\nText.\n",
			pages: 3,
		},
		{
			name:  "article from page 211 with only the title before it and front matter",
			doc:   "---\ntitle: \"Stoffwechsel\"\npages: \"211-213\"\n---\n\n# Stoffwechsel\n\n<!-- page 211 (pdf 1) -->\nText.\n<!-- page 212 (pdf 2) -->\nText.\n<!-- page 213 (pdf 3) -->\nText.\n",
			pages: 3,
		},
		{
			name:     "one missing page",
			doc:      "# Kniha\n<!-- 1 -->\na\n<!-- 2 -->\nb\n<!-- 4 -->\nc\n",
			pages:    4,
			warnings: []string{"page 3 is missing between 2 and 4"},
		},
		{
			name:     "unknown numbers fill the gap",
			doc:      "# Kniha\n<!-- page 12 (pdf 11) -->\na\n<!-- page ? (pdf 12) -->\nb\n<!-- page ? (pdf 13) -->\nc\n<!-- page 15 (pdf 14) -->\nd\n",
			pages:    4,
			warnings: []string{"the printed page number is not known", "the printed page number is not known"},
		},
		{
			name:     "unknown numbers do not fill the gap",
			doc:      "# Kniha\n<!-- page 12 (pdf 11) -->\na\n<!-- page ? (pdf 12) -->\nb\n<!-- page 17 (pdf 13) -->\nc\n",
			pages:    6,
			warnings: []string{"the printed page number is not known", "3 pages missing between 12 and 17, besides 1 page without a number"},
		},
		{
			name:     "more unknown pages than the gap",
			doc:      "# Kniha\n<!-- 12 -->\na\n<!-- ? -->\nb\n<!-- ? -->\nc\n<!-- 14 -->\nd\n",
			pages:    3,
			warnings: []string{"the printed page number is not known", "the printed page number is not known", "2 pages without a number lie between 12 and 14, but there is room for only 1 page"},
		},
		{
			name:     "notes block at the end marked with their pages",
			doc:      "# Kniha\n<!-- 1 -->\na\n<!-- 2 -->\nb\n<!-- 3 -->\nc\n\n<!-- footnotes without a marker in the text -->\n\n<!-- 1 --> 1 Poznámka.\n\n<!-- 2 --> 2 Poznámka.\n",
			pages:    3,
			warnings: []string{"page 1 comes after page 3; the next markers that go back are not reported"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Check([]byte(tc.doc))
			assert.Equal(t, tc.pages, report.OriginalPages, "pages of the original in:\n%s", tc.doc)
			var got []string
			for _, w := range report.Warnings {
				if w.Rule == RulePages {
					got = append(got, w.Message)
				}
			}
			assert.Equal(t, tc.warnings, got, "page warnings in:\n%s", tc.doc)
		})
	}
}

// Front matter is metadata, not text of the book: it adds no characters and
// its first line is not reported as text without a heading.
func TestFrontMatterIsNotText(t *testing.T) {
	body := "# Kniha\n\nText.\n"
	withMeta := Check([]byte("---\ntitle: \"Kniha\"\nauthor: \"Jan Novák\"\n---\n" + body))
	plain := Check([]byte(body))
	assert.Equal(t, plain.Characters, withMeta.Characters, "front matter does not count towards standard pages")
	assert.Empty(t, withMeta.Warnings, "a book with front matter and a # title has no warnings")
}
