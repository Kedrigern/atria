package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/tables"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonRequest(r http.Handler, user *core.User, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfTokenFor(user.Email))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTablesWeb(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	// Import a CSV through the upload form.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("_csrf", csrfTokenFor(user.Email)))
	fw, err := mw.CreateFormFile("csv", "vydaje.csv")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("Položka;Cena\nMléko;24,90\nChleba;35,50\n"))
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/tables/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	location := w.Header().Get("Location")
	require.True(t, strings.HasPrefix(location, "/tables/"), location)
	tableID := strings.TrimPrefix(location, "/tables/")
	api := "/api/tables/" + tableID

	t.Run("pages render", func(t *testing.T) {
		assert.Contains(t, getWithCookie(r, "/tables", nil).Body.String(), "vydaje")
		page := getWithCookie(r, location, nil)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Contains(t, page.Body.String(), `data-table-id="`+tableID+`"`)
		assert.Contains(t, page.Body.String(), "/static/tables.js")
	})

	var data struct {
		Table tables.Table `json:"table"`
		Rows  []tables.Row `json:"rows"`
	}
	w = jsonRequest(r, user, http.MethodGet, api, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	require.Len(t, data.Rows, 2)
	first := data.Rows[0]

	t.Run("update, conflict, validation", func(t *testing.T) {
		w := jsonRequest(r, user, http.MethodPatch, api+"/rows/"+first.ID.String(), map[string]any{
			"version": first.Version, "values": map[string]any{"c2": "31,5"}})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		w = jsonRequest(r, user, http.MethodPatch, api+"/rows/"+first.ID.String(), map[string]any{
			"version": first.Version, "values": map[string]any{"c2": "1"}})
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "changed elsewhere")

		w = jsonRequest(r, user, http.MethodPatch, api+"/rows/"+first.ID.String(), map[string]any{
			"version": first.Version + 1, "values": map[string]any{"c2": "abc"}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "is not a number")
		assert.NotContains(t, w.Body.String(), "pq:")
	})

	t.Run("add and delete row", func(t *testing.T) {
		w := jsonRequest(r, user, http.MethodPost, api+"/rows", map[string]any{"values": map[string]any{"c1": "Káva"}})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var row tables.Row
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &row))
		w = jsonRequest(r, user, http.MethodDelete, api+"/rows/"+row.ID.String(), nil)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("columns", func(t *testing.T) {
		cols := append(data.Table.Columns, tables.Column{ID: "c3", Name: "qty", Type: tables.TypeNumber})
		w := jsonRequest(r, user, http.MethodPut, api+"/columns", map[string]any{"schema_version": data.Table.SchemaVersion, "columns": cols})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = jsonRequest(r, user, http.MethodPut, api+"/columns", map[string]any{"schema_version": data.Table.SchemaVersion, "columns": cols})
		assert.Equal(t, http.StatusConflict, w.Code)
		w = jsonRequest(r, user, http.MethodPut, api+"/columns", map[string]any{"schema_version": data.Table.SchemaVersion + 1,
			"columns": []tables.Column{{ID: "c1", Name: "Bad Name", Type: tables.TypeText}}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("csv export", func(t *testing.T) {
		w := getWithCookie(r, "/tables/"+tableID+"/export.csv", nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Disposition"), "vydaje.csv")
		assert.Contains(t, w.Body.String(), "Mléko,31.50")
	})

	t.Run("other user cannot read or write", func(t *testing.T) {
		other, err := users.CreateUser(ctx, db, "intruder@atria.local", "Intruder", "pass", core.RoleUser)
		require.NoError(t, err)
		asUser(t, other)
		assert.Equal(t, http.StatusNotFound, jsonRequest(r, other, http.MethodGet, api, nil).Code)
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/tables/"+tableID, nil).Code)
		w := jsonRequest(r, other, http.MethodPatch, api+"/rows/"+first.ID.String(), map[string]any{"version": 2, "values": map[string]any{"c1": "x"}})
		assert.NotEqual(t, http.StatusOK, w.Code)
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/tables/"+tableID+"/export.csv", nil).Code)
		asUser(t, user)
	})

	t.Run("delete table", func(t *testing.T) {
		w := postForm(r, api+"/delete", user, map[string][]string{})
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/tables/"+tableID, nil).Code)
	})
}
