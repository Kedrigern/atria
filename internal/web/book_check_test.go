package web

import (
	"context"
	"net/http"
	"testing"

	"atria/internal/notes"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A note converted from a printed book shows the page count of the original
// and the Markdown check; an ordinary note shows neither.
func TestNotePageShowsOriginalPagesOfAConvertedBook(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	book, err := notes.CreateNote(ctx, db, user.ID, "Paměti mlynáře", "/knihy",
		"# Paměti mlynáře\n\n<!-- 1 -->\nMlýn stál u řeky.\n<!-- 3 -->\nKupec přijel z Plzně.\n")
	require.NoError(t, err)
	plain, err := notes.CreateNote(ctx, db, user.ID, "Nákup", "/", "mléko, chleba")
	require.NoError(t, err)

	page := getWithCookie(r, "/notes/"+book.ID.String(), nil)
	require.Equal(t, http.StatusOK, page.Code)
	main := mainContent(page.Body.String())
	assert.Contains(t, main, "3 original pages", "a marker starts its page, so pages 1 to 3 make 3 pages")
	assert.Contains(t, main, "Markdown check: 1 warnings", "the gap between pages 1 and 3 is reported")
	assert.Contains(t, main, "page 2 is missing between 1 and 3")

	main = mainContent(getWithCookie(r, "/notes/"+plain.ID.String(), nil).Body.String())
	assert.NotContains(t, main, "original pages", "a note without page markers is not a book")
	assert.NotContains(t, main, "Markdown check", "a note without page markers gets no check")
}
