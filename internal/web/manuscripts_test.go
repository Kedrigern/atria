package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A book from the PDF converter, with page 4 missing.
const convertedBook = `---
title: "Pracující suverén"
author: "Axel Honneth"
lang: "cs"
---

# Pracující suverén

Normativní teorie práce

<!-- page 2 -->

Z německého originálu přeložil Martin Profant.

<!-- page 3 -->

## Úvod

V liberalismu se mluví o právech, svobodách a společenskosti.[^1]

<!-- page 5 -->

Práce je pro většinu lidí hlavní částí bdělého života.

[^1]: Srov. Hegel, Základy filosofie práva.
`

// postFile sends a multipart form with one file, as the browser does.
func postFile(t *testing.T, r http.Handler, path string, user *core.User, fields map[string]string, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	require.NoError(t, form.WriteField("_csrf", csrfTokenFor(user.Email)))
	for k, v := range fields {
		require.NoError(t, form.WriteField(k, v))
	}
	if filename != "" {
		part, err := form.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, form.Close())
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The author uploads a converted book in the browser, reads the check, fixes
// the file and uploads it again on the edit page. The book stays private.
func TestManuscriptUploadCheckAndReplace(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	w := postFile(t, r, "/api/manuscripts/import", user, map[string]string{"path": "/knihy"}, "honneth.md", convertedBook)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	page := w.Header().Get("Location")
	require.True(t, strings.HasPrefix(page, "/manuscripts/"), "the upload opens the page of the new manuscript")

	detail := mainContent(getWithCookie(r, page, nil).Body.String())
	assert.Contains(t, detail, "Pracující suverén")
	assert.Contains(t, detail, "Axel Honneth", "the author comes from the front matter")
	assert.Contains(t, detail, "/knihy")
	assert.Contains(t, detail, "5 original pages", "pages 1 (title) to 5 of the original")
	assert.Contains(t, detail, "Markdown check: 1 warnings", "the page shows the check of the file")
	assert.Contains(t, detail, "page 4 is missing between 3 and 5")
	assert.Contains(t, detail, "V liberalismu se mluví", "the page shows the text of the book")
	assert.Contains(t, detail, "Srov. Hegel", "the page shows the footnotes")
	assert.NotContains(t, detail, "[^1]", "footnotes are rendered, not shown as Markdown")

	list := mainContent(getWithCookie(r, "/manuscripts", nil).Body.String())
	assert.Contains(t, list, `href="`+page+`"`, "the list links to the manuscript")
	home := getWithCookie(r, "/", nil).Body.String()
	assert.Contains(t, home, "Books: <strong>1</strong>", "the home page counts the manuscripts")
	assert.Contains(t, home, `href="/manuscripts"`, "the menu has the section")

	results := mainContent(getWithCookie(r, "/search?q=liberalismu", nil).Body.String())
	assert.Contains(t, results, `href="`+page+`"`, "search finds the text of a manuscript")

	again := postFile(t, r, "/api/manuscripts/import", user, map[string]string{"path": "/knihy"}, "honneth.md", convertedBook)
	assert.Equal(t, http.StatusBadRequest, again.Code, "the same title in the same folder is refused")
	assert.Contains(t, again.Body.String(), "already in /knihy")

	fixed := strings.Replace(convertedBook, "<!-- page 5 -->", "<!-- page 4 -->\n\nChybějící strana.\n\n<!-- page 5 -->", 1)
	w = postFile(t, r, "/api"+page+"/update", user, map[string]string{
		"title": "Pracující suverén", "author": "Axel Honneth (překlad Martin Profant)", "language": "cs", "path": "/knihy",
	}, "honneth-opraveno.md", fixed)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	assert.Equal(t, page, w.Header().Get("Location"), "the replaced manuscript keeps its page")
	detail = mainContent(getWithCookie(r, page, nil).Body.String())
	assert.Contains(t, detail, "Markdown check: no warnings", "the new file replaced the old one")
	assert.Contains(t, detail, "Axel Honneth (překlad Martin Profant)", "the form saves the metadata")

	download := getWithCookie(r, page+"/download", nil)
	require.Equal(t, http.StatusOK, download.Code)
	assert.Equal(t, fixed, download.Body.String(), "the download gives back the uploaded file")

	other, err := users.CreateUser(context.Background(), db, "jana@atria.local", "Jana", "heslo", core.RoleUser)
	require.NoError(t, err)
	asUser(t, other)
	assert.Equal(t, http.StatusNotFound, getWithCookie(r, page, nil).Code, "another user cannot open the manuscript")
	assert.NotContains(t, getWithCookie(r, "/search?q=liberalismu", nil).Body.String(), page, "another user does not find it")

	asUser(t, user)
	w = postForm(r, "/api"+page+"/delete", user, url.Values{})
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	assert.Contains(t, getWithCookie(r, "/manuscripts", nil).Body.String(), "You have no manuscripts yet", "the deleted manuscript leaves the list")
}

// A user who does not write books hides the section; the menu and the home
// page leave it out.
func TestHiddenManuscriptsSection(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	form := url.Values{"theme": {"system"}, "pagination_size": {"30"}, "font_size": {"100"}, "section": {"rss", "read", "notes"}}
	require.Equal(t, http.StatusFound, postForm(r, "/api/profile/preferences", user, form).Code)
	home := getWithCookie(r, "/", nil).Body.String()
	assert.NotContains(t, home, `href="/manuscripts"`, "a hidden section is not in the menu or on the home page")
	assert.Contains(t, getWithCookie(r, "/profile", nil).Body.String(), `value="manuscripts"`, "the profile lists the section")
}
