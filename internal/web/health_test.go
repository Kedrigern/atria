package web_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"atria/internal/testutil"
	"atria/internal/web"
)

func TestHealthz(t *testing.T) {
	t.Run("database reachable", func(t *testing.T) {
		db, _ := testutil.SetupTestDB(t)
		defer db.Close()

		w := httptest.NewRecorder()
		web.NewServer(db).SetupRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "ok\n", w.Body.String())
	})

	t.Run("database unreachable", func(t *testing.T) {
		db, err := sql.Open("postgres", "host=/nonexistent dbname=none sslmode=disable connect_timeout=1")
		require.NoError(t, err)
		defer db.Close()

		w := httptest.NewRecorder()
		web.NewServer(db).SetupRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
