package web

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/ebooks"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// titlesIn returns the e-book and manuscript titles that a page links, in order.
func titlesIn(page string) []string {
	var titles []string
	for _, m := range regexp.MustCompile(`href="/(?:ebooks|manuscripts)/[0-9a-f-]{36}"[^>]*>([^<]+)</a>`).FindAllStringSubmatch(page, -1) {
		titles = append(titles, m[1])
	}
	return titles
}

// The user turns three Discworld e-books into the series "Zeměplocha" from
// the e-book pages, puts them in reading order, and finds the series with its
// numbers in the e-book list.
func TestBookSeriesAsCollection(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	epub := filepath.Join(t.TempDir(), "book.epub")
	writeSmallEPUB(t, epub)
	books := map[string]uuid.UUID{}
	for _, title := range []string{"Mort", "Barva kouzel", "Lehké fantastično", "Egypťan Sinuhet"} {
		b, err := ebooks.Import(ctx, db, user.ID, ebooks.ImportInput{Title: title, Author: "Terry Pratchett", Path: epub, Filename: "book.epub"})
		require.NoError(t, err)
		books[title] = b.ID
	}
	add := func(title string, form url.Values) *http.Response {
		form.Set("entity_id", books[title].String())
		w := postForm(r, "/api/collections/add-item", user, form)
		require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
		return w.Result()
	}

	// The first book creates the collection from its page.
	created := add("Mort", url.Values{"new_title": {"Zeměplocha"}})
	collectionPage := created.Header.Get("Location")
	require.True(t, strings.HasPrefix(collectionPage, "/collections/"), "a new collection opens its page")
	collectionID := strings.TrimPrefix(collectionPage, "/collections/")
	add("Barva kouzel", url.Values{"collection_id": {collectionID}})
	add("Lehké fantastično", url.Values{"collection_id": {collectionID}})

	page := mainContent(getWithCookie(r, collectionPage, nil).Body.String())
	assert.Equal(t, []string{"Mort", "Barva kouzel", "Lehké fantastično"}, titlesIn(page), "a new item goes to the end")
	assert.NotContains(t, page, "Egypťan Sinuhet")

	w := postForm(r, "/api/collections/"+collectionID+"/items/"+books["Barva kouzel"].String()+"/move", user, url.Values{"step": {"-1"}})
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	page = mainContent(getWithCookie(r, collectionPage, nil).Body.String())
	assert.Equal(t, []string{"Barva kouzel", "Mort", "Lehké fantastično"}, titlesIn(page), "the arrow moves a book one place up")
	assert.Regexp(t, `collection-number">1</span>[\s\S]*Barva kouzel[\s\S]*collection-number">2</span>[\s\S]*Mort`, page, "a manual collection numbers its items")

	list := mainContent(getWithCookie(r, "/ebooks?collection="+collectionID, nil).Body.String())
	assert.Equal(t, []string{"Barva kouzel", "Mort", "Lehké fantastično"}, titlesIn(list), "the collection filter shows the series in its order")
	assert.Contains(t, list, `>Zeměplocha 2</a>`, "the card shows the series and the number")
	assert.Contains(t, mainContent(getWithCookie(r, "/ebooks/"+books["Mort"].String(), nil).Body.String()), "Zeměplocha 2", "the e-book page lists its collections")

	update := func(order string) {
		w := postForm(r, "/api/collections/"+collectionID+"/update", user, url.Values{"title": {"Zeměplocha"}, "sort_order": {order}})
		require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	}
	update("title")
	page = mainContent(getWithCookie(r, collectionPage, nil).Body.String())
	assert.Equal(t, []string{"Barva kouzel", "Lehké fantastično", "Mort"}, titlesIn(page), "an automatic order sorts by title")
	assert.NotContains(t, page, "collection-number", "an automatic order has no numbers")
	update("manual")
	page = mainContent(getWithCookie(r, collectionPage, nil).Body.String())
	assert.Equal(t, []string{"Barva kouzel", "Lehké fantastično", "Mort"}, titlesIn(page), "a collection that becomes manual starts with its current order")

	require.NoError(t, core.SoftDeleteEntity(ctx, db, user.ID, books["Lehké fantastično"]))
	page = mainContent(getWithCookie(r, collectionPage, nil).Body.String())
	assert.Equal(t, []string{"Barva kouzel", "Mort"}, titlesIn(page), "a book in the trash leaves the collection")
	assert.Contains(t, mainContent(getWithCookie(r, "/ebooks?collection="+collectionID, nil).Body.String()), ">Zeměplocha 2</a>", "the numbers close the gap")
}

// A collection belongs to its owner: another user cannot open it, add to it,
// or add someone else's book to their own collection.
func TestCollectionsBelongToTheirOwner(t *testing.T) {
	db, jana := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	martin, err := users.CreateUser(ctx, db, "martin@atria.local", "Martin Profant", "heslo", core.RoleUser)
	require.NoError(t, err)

	epub := filepath.Join(t.TempDir(), "book.epub")
	writeSmallEPUB(t, epub)
	martinsBook, err := ebooks.Import(ctx, db, martin.ID, ebooks.ImportInput{Title: "Paměti mlynáře", Path: epub, Filename: "book.epub"})
	require.NoError(t, err)
	janasNote, err := notes.CreateNote(ctx, db, jana.ID, "Nákup", "/", "mléko")
	require.NoError(t, err)

	asUser(t, jana)
	w := postForm(r, "/api/collections/create", jana, url.Values{"title": {"Gaiman"}, "sort_order": {"title"}})
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	collectionID := strings.TrimPrefix(w.Header().Get("Location"), "/collections/")

	w = postForm(r, "/api/collections/add-item", jana, url.Values{"collection_id": {collectionID}, "entity_id": {martinsBook.ID.String()}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "the owner cannot add the book of another user")
	w = postForm(r, "/api/collections/add-item", jana, url.Values{"collection_id": {collectionID}, "entity_id": {janasNote.ID.String()}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "a collection holds e-books and manuscripts only")

	asUser(t, martin)
	assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/collections/"+collectionID, nil).Code, "another user cannot open the collection")
	w = postForm(r, "/api/collections/add-item", martin, url.Values{"collection_id": {collectionID}, "entity_id": {martinsBook.ID.String()}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "another user cannot add to the collection")

	asUser(t, jana)
	assert.Empty(t, titlesIn(mainContent(getWithCookie(r, "/collections/"+collectionID, nil).Body.String())), "nothing got into the collection")
}
