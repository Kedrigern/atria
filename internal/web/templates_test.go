package web

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"atria/internal/notes"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pages are parsed once and then served from the template cache, including
// concurrently and for both full-page and htmx (partial) requests.
func TestTemplateCacheRendersRepeatedlyAndConcurrently(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	_, err := notes.CreateNote(t.Context(), db, user.ID, "Cached", "/folder", "body")
	require.NoError(t, err)

	srv := NewServer(db)
	r := newTestRouter(t, srv)
	asUser(t, user)

	get := func(path string, htmx bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	full := get("/notes", false)
	require.Equal(t, http.StatusOK, full.Code, full.Body.String())
	assert.Contains(t, full.Body.String(), "<html")
	assert.Contains(t, full.Body.String(), "Cached")

	partial := get("/notes", true)
	require.Equal(t, http.StatusOK, partial.Code)
	assert.NotContains(t, partial.Body.String(), "<html", "htmx requests get only the content block")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(htmx bool) {
			defer wg.Done()
			w := get("/notes", htmx)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "Cached")
		}(i%2 == 0)
	}
	wg.Wait()

	srv.templates.mu.Lock()
	defer srv.templates.mu.Unlock()
	assert.Contains(t, srv.templates.sets, "page:note_list.html")
	assert.Contains(t, srv.templates.sets, "hx:note_list.html")
}
