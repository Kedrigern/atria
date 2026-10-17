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

func TestStoragePage(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	admin, err := users.CreateUser(context.Background(), db, "boss@atria.local", "Boss", "pass", core.RoleAdmin)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))

	t.Run("user sees only their own usage", func(t *testing.T) {
		asUser(t, user)
		w := getWithCookie(r, "/settings/storage", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "Úložiště")
		assert.NotContains(t, w.Body.String(), "Všichni uživatelé")
		assert.NotContains(t, w.Body.String(), admin.Email)
	})

	t.Run("admin sees every user and the database size", func(t *testing.T) {
		asUser(t, admin)
		w := getWithCookie(r, "/settings/storage", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		body := w.Body.String()
		assert.Contains(t, body, "Všichni uživatelé")
		assert.Contains(t, body, user.Email)
		assert.Contains(t, body, admin.Email)
		assert.Contains(t, body, "Databáze celkem")
	})
}
