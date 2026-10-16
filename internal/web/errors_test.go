package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func TestUserFacingError(t *testing.T) {
	internal := fmt.Errorf("query notes: %w", errors.New(`pq: relation "notes" does not exist`))
	assert.Equal(t, "Failed to load notes", userFacingError("Failed to load notes", internal))

	validation := fmt.Errorf("rename: %w", core.NewUserError("entity title is required"))
	assert.Equal(t, "Failed to rename: entity title is required", userFacingError("Failed to rename", validation))

	duplicate := fmt.Errorf("insert: %w", &pq.Error{Code: "23505", Message: `duplicate key value violates unique constraint "tags_owner_id_name_key"`})
	got := userFacingError("Failed to create tag", duplicate)
	assert.Equal(t, "Failed to create tag: an item with this name already exists", got)
	assert.NotContains(t, got, "tags_owner_id_name_key")
}

func TestTagCreateErrors(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	t.Run("validation message is shown", func(t *testing.T) {
		w := postForm(r, "/api/tags/create", user, url.Values{"name": {"Not_Valid!"}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "can only contain lowercase letters")
	})

	t.Run("duplicate is reported without SQL details", func(t *testing.T) {
		w := postForm(r, "/api/tags/create", user, url.Values{"name": {"reading"}})
		assert.Less(t, w.Code, 400, w.Body.String())

		w = postForm(r, "/api/tags/create", user, url.Values{"name": {"reading"}})
		body := w.Body.String()
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, body, "already exists")
		for _, leak := range []string{"pq:", "duplicate key", "constraint"} {
			assert.False(t, strings.Contains(body, leak), "response leaks %q: %s", leak, body)
		}
	})
}
