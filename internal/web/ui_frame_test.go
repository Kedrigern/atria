package web

import (
	"net/http"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeAndFrameUseComponents(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	w := getWithCookie(r, "/", nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "🎲", "the menu uses SVG icons, not emoji")
	assert.Contains(t, body, `aria-label="Games"`)
	assert.Contains(t, body, `<h1 class="home-logo">`)
}
