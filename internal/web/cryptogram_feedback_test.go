package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A wrong guess must give feedback at once: the JSON answer has the reason,
// and without JavaScript the page after the redirect shows it.
func TestWrongCryptogramGuessGivesFeedback(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	const puzzle = "01900000-0000-7000-8000-000000000001"
	require.Equal(t, http.StatusOK, getWithCookie(r, "/games/cryptograms/"+puzzle, nil).Code)

	var mappingJSON, guessesJSON []byte
	require.NoError(t, db.QueryRow(`SELECT cipher_mapping, guesses FROM game_cryptogram_progress WHERE user_id = $1 AND cryptogram_id = $2`,
		user.ID, puzzle).Scan(&mappingJSON, &guessesJSON))
	var mapping, guesses map[string]string
	require.NoError(t, json.Unmarshal(mappingJSON, &mapping))
	require.NoError(t, json.Unmarshal(guessesJSON, &guesses))
	code := ""
	for c := range mapping {
		if guesses[c] == "" {
			code = c
			break
		}
	}
	require.NotEmpty(t, code)
	used := map[string]bool{}
	for _, l := range guesses {
		used[l] = true
	}
	wrong := ""
	for l := 'A'; l <= 'Z'; l++ {
		if s := string(l); s != mapping[code] && !used[s] {
			wrong = s
			break
		}
	}

	form := url.Values{"code": {code}, "letter": {wrong}, "_csrf": {csrfTokenFor(user.Email)}}
	req := httptest.NewRequest(http.MethodPost, "/api/games/cryptograms/"+puzzle+"/guesses", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.JSONEq(t, `{"success": false, "error": "`+wrong+` is not the letter for number `+code+`."}`, w.Body.String())

	// Without JavaScript: a redirect, and the page shows the reason.
	plain := postForm(r, "/api/games/cryptograms/"+puzzle+"/guesses", user, url.Values{"code": {code}, "letter": {wrong}})
	require.Equal(t, http.StatusSeeOther, plain.Code)
	var flash *http.Cookie
	for _, c := range plain.Result().Cookies() {
		if c.Name == "atria_flash" {
			flash = c
		}
	}
	require.NotNil(t, flash)
	page := getWithCookie(r, "/games/cryptograms/"+puzzle, flash).Body.String()
	assert.Contains(t, page, wrong+" is not the letter for number "+code+".")
	assert.Contains(t, page, "Mistakes: 2")
}
