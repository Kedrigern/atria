package web

import (
	"context"
	"net/http"
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
}
