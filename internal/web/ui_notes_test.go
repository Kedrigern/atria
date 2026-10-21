package web

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	"atria/internal/notes"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// czechLetters matches letters with Czech diacritics. The interface is in
// English only (docs/architecture/ui.md, section 3).
var czechLetters = regexp.MustCompile(`[ěščřžýáíéůúňťďĚŠČŘŽÝÁÍÉŮÚŇŤĎ]`)

// inlineStyle matches a style attribute that is not only CSS variables
// (docs/architecture/ui.md, section 11).
var inlineStyle = regexp.MustCompile(`style="\s*[^-\s"]`)

func TestNotePagesUseComponentsInEnglish(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	note, err := notes.CreateNote(context.Background(), db, user.ID, "Reading list", "/projects", "Some text.")
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	for _, path := range []string{"/notes", "/notes/new", "/notes/" + note.ID.String()} {
		w := getWithCookie(r, path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		main := mainContent(w.Body.String())
		assert.NotRegexp(t, czechLetters, main, "%s: Czech text in the page", path)
		assert.NotRegexp(t, inlineStyle, main, "%s: inline style in the page", path)
	}

	detail := getWithCookie(r, "/notes/"+note.ID.String(), nil).Body.String()
	assert.Contains(t, detail, "Move to trash")
	assert.Contains(t, detail, `data-confirm="Move this note to the trash?"`)
}

// mainContent returns the part of the page inside the main column, without the
// application frame, which is converted in its own step.
func mainContent(page string) string {
	re := regexp.MustCompile(`(?s)<main[^>]*>(.*)</main>`)
	if m := re.FindStringSubmatch(page); m != nil {
		return m[1]
	}
	return page
}
