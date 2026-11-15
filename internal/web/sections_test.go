package web

import (
	"net/http"
	"net/url"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A user who does not play games or use tables hides both in the profile. The
// menu and the home page leave them out, but the pages still open by address.
func TestHiddenSectionsLeaveTheMenuAndHomePage(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	save := func(sections ...string) {
		form := url.Values{"theme": {"system"}, "pagination_size": {"30"}, "font_size": {"100"}, "section": sections}
		w := postForm(r, "/api/profile/preferences", user, form)
		require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	}

	save("rss", "read", "notes", "ebooks")
	home := getWithCookie(r, "/", nil).Body.String()
	assert.NotContains(t, home, `href="/games"`, "a hidden section is not in the menu or on the home page")
	assert.NotContains(t, home, `href="/tables"`, "a hidden section is not in the menu or on the home page")
	assert.Contains(t, home, `href="/rss"`, "a shown section stays in the menu")
	assert.Contains(t, home, "Recent notes", "a shown section stays on the home page")
	assert.Equal(t, http.StatusOK, getWithCookie(r, "/games", nil).Code, "a hidden section still works by its address")
	profile := getWithCookie(r, "/profile", nil).Body.String()
	assert.Contains(t, profile, `value="rss" checked`, "the profile shows the saved choice")
	assert.NotContains(t, profile, `value="games" checked`, "the profile shows the saved choice")

	save("rss", "read", "notes", "ebooks", "games", "tables")
	assert.Contains(t, getWithCookie(r, "/", nil).Body.String(), `href="/games"`, "a section shows again after the user checks it")
}
