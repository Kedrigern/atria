package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"atria/internal/testutil"
	"atria/internal/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotesListEndpoint(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()

	t.Setenv("ATRIA_ENV", "development")
	t.Setenv("ATRIA_USER", user.Email)

	srv := web.NewServer(db)
	router := srv.SetupRouter()

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/notes", nil)
	require.NoError(t, err)

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Knowledge Base")
}
