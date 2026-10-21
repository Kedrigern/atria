package web

import (
	"context"
	"net/http"
	"testing"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchPagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	note, err := notes.CreateNote(ctx, db, user.ID, "Garden plan", "/", "Plant the roses.")
	require.NoError(t, err)
	tag, err := core.CreateTag(ctx, db, user.ID, "garden", false)
	require.NoError(t, err)
	require.NoError(t, core.AttachTag(ctx, db, note.ID, tag.ID))
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	for _, path := range []string{"/search", "/search?q=roses", "/tags/garden"} {
		w := getWithCookie(r, path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		main := mainContent(w.Body.String())
		assert.NotRegexp(t, czechLetters, main, "%s: Czech text in the page", path)
		assert.NotRegexp(t, inlineStyle, main, "%s: inline style in the page", path)
	}
	assert.Contains(t, getWithCookie(r, "/tags/garden", nil).Body.String(), "Garden plan")
	assert.Contains(t, getWithCookie(r, "/search", nil).Body.String(), `<label for="search-input">`)
}
