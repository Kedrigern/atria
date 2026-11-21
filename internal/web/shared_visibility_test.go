package web

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Jana makes a note visible to all users of the Atria. Martin finds it in
// search and opens it read-only, with its image; her private note stays
// hidden. Before the fix the search result led to "Note not found".
func TestNoteVisibleToAllUsersOpensReadOnlyForOthers(t *testing.T) {
	db, jana := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	martin, err := users.CreateUser(ctx, db, "martin@atria.local", "Martin Profant", "heslo", core.RoleUser)
	require.NoError(t, err)

	image := filepath.Join(t.TempDir(), "mlyn.png")
	var pic bytes.Buffer
	require.NoError(t, pngEncode(&pic))
	require.NoError(t, os.WriteFile(image, pic.Bytes(), 0o600))
	att, err := attachments.AddAttachment(ctx, db, jana.ID, image, "mlyn.png")
	require.NoError(t, err)
	shared, err := notes.CreateNote(ctx, db, jana.ID, "Mlýny na Úslavě", "/rodina",
		"Mlýn u Vejvanova mlel do roku 1951.\n\n![Mlýn](/data/attachments/"+att.DiskPath+")\n")
	require.NoError(t, err)
	require.NoError(t, attachments.LinkAttachment(ctx, db, shared.ID, att.ID))
	require.NoError(t, core.UpdateVisibility(ctx, db, jana.ID, shared.ID, core.VisibilityUsers))
	private, err := notes.CreateNote(ctx, db, jana.ID, "Mlýnské účty", "/rodina", "Dluh u kupce z Plzně.")
	require.NoError(t, err)

	asUser(t, martin)
	results := mainContent(getWithCookie(r, "/search?q=Vejvanova", nil).Body.String())
	require.Contains(t, results, `href="/notes/`+shared.ID.String()+`"`, "search shows the note visible to all users")

	page := getWithCookie(r, "/notes/"+shared.ID.String(), nil)
	require.Equal(t, http.StatusOK, page.Code, "the search result opens")
	main := mainContent(page.Body.String())
	assert.Contains(t, main, "Mlýn u Vejvanova mlel do roku 1951")
	assert.Contains(t, main, "Shared by "+jana.DisplayName, "the reader sees whose note it is")
	assert.NotContains(t, main, "/api/notes/", "another user cannot edit or delete the note")
	assert.NotContains(t, main, "/api/entity/", "another user cannot change tags, links or visibility")
	assert.Equal(t, http.StatusOK, getWithCookie(r, "/data/attachments/"+att.DiskPath, nil).Code, "the image of the shared note loads")

	assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/notes/"+private.ID.String(), nil).Code, "a private note stays hidden")

	require.NoError(t, core.UpdateVisibility(ctx, db, jana.ID, shared.ID, core.VisibilityPrivate))
	assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/notes/"+shared.ID.String(), nil).Code, "a note made private again is hidden")
	assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/data/attachments/"+att.DiskPath, nil).Code, "and so is its image")

	asUser(t, jana)
	assert.Contains(t, mainContent(getWithCookie(r, "/notes/"+shared.ID.String(), nil).Body.String()), "/api/notes/"+shared.ID.String()+"/update",
		"the owner still gets the full page")
}

// An article that its owner made visible to all users opens read-only for
// another user, with the link to the original but without the owner's note.
func TestArticleVisibleToAllUsersOpensReadOnlyForOthers(t *testing.T) {
	db, jana := testutil.SetupTestDB(t)
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	martin, err := users.CreateUser(ctx, db, "martin@atria.local", "Martin Profant", "heslo", core.RoleUser)
	require.NoError(t, err)

	id := core.NewUUID()
	_, err = db.ExecContext(ctx, `INSERT INTO entities (id, owner_id, type, visibility, title, slug) VALUES ($1, $2, 'article', 'users', 'Jak se mele mouka', 'jak-se-mele-mouka')`, id, jana.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO articles (id, original_url, domain, html_content, text_content, user_note)
		VALUES ($1, 'https://example.com/mouka', 'example.com', '<p>Zrno se drtí mezi kameny.</p>', 'Zrno se drtí mezi kameny.', 'Poslat tátovi.')`, id)
	require.NoError(t, err)

	asUser(t, martin)
	page := getWithCookie(r, "/read/"+id.String(), nil)
	require.Equal(t, http.StatusOK, page.Code, "the shared article opens")
	main := mainContent(page.Body.String())
	assert.Contains(t, main, "Zrno se drtí mezi kameny.")
	assert.Contains(t, main, `href="https://example.com/mouka"`, "the reader can open the original")
	assert.NotContains(t, main, "Poslat tátovi", "the owner's own note on the article stays private")
	assert.NotContains(t, main, "/api/articles/", "another user cannot archive or delete the article")
}
