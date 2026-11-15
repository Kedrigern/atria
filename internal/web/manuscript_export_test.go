package web

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"atria/internal/manuscripts"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A book with a # title, two ## chapters, footnotes in both, page markers
// (one inside a paragraph, one with an unknown number), an image from the
// converter and Roman front matter.
const bookForExport = `---
title: "Paměti mlynáře"
author: "Jan Novák"
lang: "cs"
---

# Paměti mlynáře

<!-- page iv -->

Předmluva vydavatele.

## Kapitola první

<!-- page 1 -->

Mlýn stál u řeky a mlynář v něm mlel obilí[^1] pro celou ves.

![Obrázek mlýna](Pameti_artifacts/image_000001.png)

Druhého dne přijel kupec z Plzně a přivezl novou <!-- page 2 --> mlýnskou kůru.

<!-- page 3 -->

## Kapitola druhá

<!-- page ? (pdf 5) -->

Večer se ve mlýně sešli sousedé[^2].

<!-- page 4 -->

Konec.

[^1]: Žito a pšenice.

[^2]: Sousedé z dolní vsi.
`

func zipFiles(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "the download is a ZIP container")
	files := map[string]string{}
	for i, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		files[f.Name] = string(content)
		if i == 0 {
			files["(first)"] = f.Name
		}
	}
	return files
}

func wellFormed(t *testing.T, name, content string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(content))
	d.Strict = true
	d.Entity = xml.HTMLEntity
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		require.NoError(t, err, "%s is well-formed XHTML", name)
	}
}

// The author downloads the book as EPUB: each chapter is a section with its
// own footnotes, and the e-book reader gets the pages of the printed original.
func TestManuscriptEPUBHasChaptersFootnotesAndOriginalPages(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	m, err := manuscripts.Create(context.Background(), db, user.ID, "/knihy", manuscripts.FromFile("pameti.md", []byte(bookForExport)))
	require.NoError(t, err)

	w := getWithCookie(r, "/manuscripts/"+m.ID.String()+"/export/epub", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	disposition, err := url.PathUnescape(w.Header().Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Contains(t, disposition, "paměti-mlynáře.epub", "the file is named after the book")
	files := zipFiles(t, w.Body.Bytes())
	assert.Equal(t, "mimetype", files["(first)"], "an EPUB starts with its mimetype")

	front, first, second := files["EPUB/xhtml/chapter-001.xhtml"], files["EPUB/xhtml/chapter-002.xhtml"], files["EPUB/xhtml/chapter-003.xhtml"]
	require.NotEmpty(t, second, "the title part and the two ## chapters are three sections")
	for name, content := range map[string]string{"front": front, "first": first, "second": second, "nav": files["EPUB/nav.xhtml"]} {
		wellFormed(t, name, content)
	}
	assert.Contains(t, front, "Předmluva vydavatele")
	assert.Contains(t, first, "Mlýn stál u řeky")
	assert.Contains(t, first, "Žito a pšenice", "a chapter carries the footnotes it uses")
	assert.Contains(t, second, "Sousedé z dolní vsi")
	assert.NotContains(t, first, "<img", "images of the converter are not part of the manuscript")
	assert.Equal(t, 1, strings.Count(second, "doc-pagebreak"), "a page with an unknown number gets no page break, page 4 does")

	assert.Contains(t, first, `<span epub:type="pagebreak" role="doc-pagebreak" id="page-2" title="2"></span>`, "a marker inside a paragraph becomes a page break there")
	assert.Contains(t, first, "novou <span", "the page break stays inside the paragraph")
	nav := files["EPUB/nav.xhtml"]
	labels := regexp.MustCompile(`#page-[^"]+">([^<]+)</a>`).FindAllStringSubmatch(nav, -1)
	var got []string
	for _, l := range labels {
		got = append(got, l[1])
	}
	assert.Equal(t, []string{"iv", "1", "2", "3", "4"}, got, "the page list has the pages of the original in reading order")
	assert.Contains(t, nav, `<a href="xhtml/chapter-002.xhtml#page-1">`, "each page links to its chapter")
}

// The author publishes the manuscript; the e-book has its metadata and links
// back to it.
func TestPublishManuscriptAsEbook(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	m, err := manuscripts.Create(context.Background(), db, user.ID, "/knihy", manuscripts.FromFile("pameti.md", []byte(bookForExport)))
	require.NoError(t, err)

	w := postForm(r, "/api/manuscripts/"+m.ID.String()+"/publish", user, url.Values{})
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	ebookPage := w.Header().Get("Location")
	require.True(t, strings.HasPrefix(ebookPage, "/ebooks/"), "publishing opens the new e-book")

	page := mainContent(getWithCookie(r, ebookPage, nil).Body.String())
	assert.Contains(t, page, "Paměti mlynáře")
	assert.Contains(t, page, "Jan Novák", "the e-book takes the author of the manuscript")
	assert.Contains(t, page, `href="/manuscripts/`+m.ID.String()+`"`, "the e-book links to its manuscript")

	download := getWithCookie(r, ebookPage+"/download", nil)
	require.Equal(t, http.StatusOK, download.Code)
	assert.Contains(t, zipFiles(t, download.Body.Bytes())["EPUB/nav.xhtml"], `epub:type="page-list"`, "the e-book has the page list")
}

// DOCX needs pandoc. Without it the page offers no DOCX; with it the
// document has the text and the page numbers of the original in brackets.
func TestManuscriptDOCXNeedsPandoc(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	// An image path that exists on the server: pandoc must not embed it.
	secret := filepath.Join(t.TempDir(), "secret.png")
	var pic bytes.Buffer
	require.NoError(t, pngEncode(&pic))
	require.NoError(t, os.WriteFile(secret, pic.Bytes(), 0o600))
	book := strings.Replace(bookForExport, "Pameti_artifacts/image_000001.png", secret, 1)
	m, err := manuscripts.Create(context.Background(), db, user.ID, "/knihy", manuscripts.FromFile("pameti.md", []byte(book)))
	require.NoError(t, err)
	page := "/manuscripts/" + m.ID.String()

	pandoc, err := exec.LookPath("pandoc")
	if err != nil {
		t.Skip("pandoc is not installed")
	}

	t.Run("without pandoc", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		detail := mainContent(getWithCookie(r, page, nil).Body.String())
		assert.NotContains(t, detail, "/export/docx", "the page offers no DOCX without pandoc")
		assert.Contains(t, detail, "/export/epub", "EPUB needs no tools")
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, page+"/export/docx", nil).Code)
	})

	t.Run("with pandoc", func(t *testing.T) {
		t.Setenv("PATH", strings.TrimSuffix(pandoc, "/pandoc"))
		assert.Contains(t, mainContent(getWithCookie(r, page, nil).Body.String()), page+"/export/docx")
		w := getWithCookie(r, page+"/export/docx", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		files := zipFiles(t, w.Body.Bytes())
		doc := files["word/document.xml"]
		assert.Contains(t, doc, "Mlýn stál u řeky")
		assert.Contains(t, doc, "[1] Mlýn stál", "a marker on its own line starts the next paragraph")
		assert.Contains(t, doc, "novou [2] mlýnskou", "a marker inside a paragraph stays there")
		assert.NotContains(t, doc, "[?]", "a page with an unknown number has no number")
		assert.Contains(t, doc, "<w:t xml:space=\"preserve\">[3]</w:t>", "a marker before a heading is a paragraph of its own")
		assert.NotContains(t, doc, "[3] Kapitola", "a page number never goes into a heading or the table of contents")
		for name := range files {
			assert.NotContains(t, name, "media/", "the DOCX embeds no files from the server")
		}
	})
}

func pngEncode(w io.Writer) error {
	return png.Encode(w, image.NewRGBA(image.Rect(0, 0, 4, 4)))
}
