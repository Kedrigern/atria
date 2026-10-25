package web

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGamePagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	// The migrations add sample cryptograms, so the list has a puzzle to open.
	paths := []string{"/games", "/games/cryptograms/01900000-0000-7000-8000-000000000001"}
	for _, path := range paths {
		w := getWithCookie(r, path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		main := mainContent(w.Body.String())
		assert.NotRegexp(t, czechLetters, main, "%s: Czech text in the page", path)
		assert.NotRegexp(t, inlineStyle, main, "%s: inline style in the page", path)
		assert.Contains(t, main, `class="page-title"`, path)
	}
}

// The cryptogram has its own text size, so the A-/A+ buttons do not change
// the size of articles and notes.
func TestCryptogramTextSizeIsSeparate(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)
	puzzle := "/games/cryptograms/01900000-0000-7000-8000-000000000001"

	page := mainContent(getWithCookie(r, puzzle, nil).Body.String())
	assert.Contains(t, page, `--cryptogram-scale: 1.00`)
	assert.Contains(t, page, `name="target" value="cryptogram"`)

	w := postForm(r, "/api/profile/font-size", user, url.Values{"target": {"cryptogram"}, "delta": {"-1"}})
	require.Equal(t, http.StatusFound, w.Code)

	page = mainContent(getWithCookie(r, puzzle, nil).Body.String())
	assert.Contains(t, page, `--cryptogram-scale: 0.90`)
	all, err := users.ListUsers(context.Background(), db)
	require.NoError(t, err)
	for _, u := range all {
		if u.ID == user.ID {
			assert.Equal(t, 90, u.Preferences.CryptogramFontSize)
			assert.Equal(t, 100, u.Preferences.FontSize)
		}
	}
}
