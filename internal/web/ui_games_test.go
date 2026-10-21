package web

import (
	"net/http"
	"testing"

	"atria/internal/testutil"

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
