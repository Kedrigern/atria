package web

import (
	"context"
	"net/http"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsPagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	_, err := db.Exec(`UPDATE users SET role = $1 WHERE id = $2`, core.RoleAdmin, user.ID)
	require.NoError(t, err)
	user.Role = core.RoleAdmin
	_, err = users.CreateUser(context.Background(), db, "other@example.com", "Other", "a-long-password-1", core.RoleUser)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	paths := []string{"/settings/users", "/settings/rss", "/settings/tags", "/settings/attachments", "/settings/storage", "/profile"}
	for _, path := range paths {
		w := getWithCookie(r, path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		main := mainContent(w.Body.String())
		assert.NotRegexp(t, czechLetters, main, "%s: Czech text in the page", path)
		assert.NotRegexp(t, inlineStyle, main, "%s: inline style in the page", path)
	}
	assert.Contains(t, getWithCookie(r, "/settings/users", nil).Body.String(), "Delete permanently")
}

func TestSettingsHideUsersTabFromUsers(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	user.Role = core.RoleUser
	_, err := db.Exec(`UPDATE users SET role = $1 WHERE id = $2`, core.RoleUser, user.ID)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	body := getWithCookie(r, "/settings/rss", nil).Body.String()
	assert.NotContains(t, body, `href="/settings/users"`)
}
