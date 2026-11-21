package web

import (
	"context"
	"net/http"
	"path/filepath"
	"regexp"
	"testing"

	"atria/internal/ebooks"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "Dobrá znamení" has two authors in one field. The author filter offers each
// author once, and the book shows up under both of them.
func TestEbookAuthorFilterSplitsSeveralAuthors(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	epub := filepath.Join(t.TempDir(), "book.epub")
	writeSmallEPUB(t, epub)
	for title, author := range map[string]string{
		"Dobrá znamení":   "Terry Pratchett, Neil Gaiman",
		"Američtí bohové": "Neil Gaiman",
		"Barva kouzel":    "Terry Pratchett",
		"Egypťan Sinuhet": "Mika Waltari",
	} {
		_, err := ebooks.Import(ctx, db, user.ID, ebooks.ImportInput{Title: title, Author: author, Path: epub, Filename: "book.epub"})
		require.NoError(t, err)
	}

	page := mainContent(getWithCookie(r, "/ebooks", nil).Body.String())
	var options []string
	for _, m := range regexp.MustCompile(`<option value="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		options = append(options, m[1])
	}
	assert.Equal(t, []string{"Mika Waltari", "Neil Gaiman", "Terry Pratchett"}, options, "each author once, the two-author field is not an author of its own")

	gaiman := mainContent(getWithCookie(r, "/ebooks?author=Neil+Gaiman", nil).Body.String())
	assert.Contains(t, gaiman, "Dobrá znamení", "a book with two authors shows up under the second author")
	assert.Contains(t, gaiman, "Američtí bohové")
	assert.NotContains(t, gaiman, "Barva kouzel")

	pratchett := getWithCookie(r, "/ebooks?author=Terry+Pratchett", nil)
	require.Equal(t, http.StatusOK, pratchett.Code)
	assert.Contains(t, mainContent(pratchett.Body.String()), "Dobrá znamení", "and under the first author")
	assert.NotContains(t, mainContent(pratchett.Body.String()), "Egypťan Sinuhet")
}
