package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStalePageReloadsAfterDeploy(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	full := getWithCookie(r, "/notes", nil)
	require.Equal(t, http.StatusOK, full.Code)
	assert.Contains(t, full.Body.String(), `<meta name="atria-build" content="`+uiBuild()+`"`)

	htmxGet := func(build string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/notes", nil)
		req.Header.Set("HX-Request", "true")
		if build != "" {
			req.Header.Set("X-Atria-Build", build)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	current := htmxGet(uiBuild())
	assert.Empty(t, current.Header().Get("HX-Refresh"))
	assert.Contains(t, current.Body.String(), "Knowledge Base")

	old := htmxGet("0123456789ab")
	assert.Equal(t, "true", old.Header().Get("HX-Refresh"), "a page of an older build must reload")
	assert.NotContains(t, old.Body.String(), "Knowledge Base")

	// Pages loaded before this check existed send no build. They get content as before.
	assert.Contains(t, htmxGet("").Body.String(), "Knowledge Base")
}
