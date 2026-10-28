package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"atria/internal/tables"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTablePagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	w := getWithCookie(r, "/tables", nil)
	require.Equal(t, http.StatusOK, w.Code)
	main := mainContent(w.Body.String())
	assert.NotRegexp(t, czechLetters, main)
	assert.NotRegexp(t, inlineStyle, main)
	assert.Contains(t, main, `<label for="new-table-title">Title</label>`)

	table, err := tables.Create(context.Background(), db, user.ID, "Books", "books", []tables.Column{{ID: "c1", Name: "title", Type: tables.TypeText}})
	require.NoError(t, err)
	w = getWithCookie(r, "/tables/"+table.ID.String(), nil)
	require.Equal(t, http.StatusOK, w.Code)
	main = mainContent(w.Body.String())
	assert.NotRegexp(t, czechLetters, main)
	assert.NotRegexp(t, inlineStyle, main)
	assert.Contains(t, main, `<h1 class="page-title">Books</h1>`)
	assert.Contains(t, main, `hx-post="/api/entity/`+table.ID.String()+`/rename"`)
}

// A table can be renamed. The SQL name stays, so views keep working.
func TestTableRenameKeepsSQLName(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	table, err := tables.Create(context.Background(), db, user.ID, "Books", "books", []tables.Column{{ID: "c1", Name: "title", Type: tables.TypeText}})
	require.NoError(t, err)

	form := url.Values{"_csrf": {csrfTokenFor(user.Email)}}
	req := httptest.NewRequest(http.MethodPost, "/api/entity/"+table.ID.String()+"/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Prompt", "Library")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	renamed, err := tables.Get(context.Background(), db, user.ID, table.ID)
	require.NoError(t, err)
	assert.Equal(t, "Library", renamed.Title)
	assert.Equal(t, "books", renamed.SQLName)
}
