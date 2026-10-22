package web

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagMergeForm(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	_, err := core.CreateTag(ctx, db, user.ID, "books", false)
	require.NoError(t, err)
	_, err = core.CreateTag(ctx, db, user.ID, "reading", false)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	page := getWithCookie(r, "/settings/tags", nil).Body.String()
	assert.Contains(t, page, "Merge two tags")
	assert.Contains(t, page, `action="/api/tags/merge"`)

	w := postForm(r, "/api/tags/merge", user, url.Values{"from": {"books"}, "into": {"reading"}})
	assert.Equal(t, http.StatusSeeOther, w.Code)
	_, err = core.FindTagByName(ctx, db, user.ID, "books")
	assert.Error(t, err)
}
