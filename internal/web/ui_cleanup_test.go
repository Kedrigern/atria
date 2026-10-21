package web

import (
	"net/http"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutdatedContentIsGone(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	w := getWithCookie(r, "/settings", nil)
	assert.Equal(t, http.StatusSeeOther, w.Code, "the General tab is gone, /settings opens the RSS tab")
	assert.Equal(t, "/settings/rss", w.Header().Get("Location"))

	home := getWithCookie(r, "/", nil)
	require.Equal(t, http.StatusOK, home.Code)
	assert.NotContains(t, home.Body.String(), "Proof of Concept")

	profile := getWithCookie(r, "/profile", nil)
	require.Equal(t, http.StatusOK, profile.Code)
	assert.NotContains(t, profile.Body.String(), "Remote-Email", "no proxy debug table")

	rssSettings := getWithCookie(r, "/settings/rss", nil)
	require.Equal(t, http.StatusOK, rssSettings.Code)
	assert.NotContains(t, rssSettings.Body.String(), ">General<")
}
