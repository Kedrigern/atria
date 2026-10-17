package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"atria/internal/core"
	"atria/internal/tables"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// --- Pages -------------------------------------------------------------

func (s *Server) handleTableList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	list, err := tables.List(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load tables", err)
		return
	}
	s.render(c, "table_list.html", gin.H{
		"Tables":    list,
		"MaxRows":   tables.MaxRows,
		"MaxCols":   tables.MaxColumns,
		"PageTitle": "Tables",
	})
}

func (s *Server) handleTableDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	t, ok := s.loadTable(c, user)
	if !ok {
		return
	}
	s.render(c, "table_detail.html", gin.H{
		"Table":       t,
		"ColumnTypes": []tables.ColumnType{tables.TypeText, tables.TypeNumber, tables.TypeDate, tables.TypeCheckbox, tables.TypeSelect, tables.TypeURL},
	})
}

func (s *Server) handleTableCreate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	title := strings.TrimSpace(c.PostForm("title"))
	sqlName := strings.TrimSpace(c.PostForm("sql_name"))
	if sqlName == "" {
		sqlName = tables.Identifier(title, "table")
	}
	columns := []tables.Column{{ID: "c1", Name: "name", Type: tables.TypeText}}
	t, err := tables.Create(c.Request.Context(), s.db, user.ID, title, sqlName, columns)
	if err != nil {
		s.flashError(c, "Failed to create table", err)
		c.Redirect(http.StatusSeeOther, "/tables")
		return
	}
	s.redirectTo(c, "/tables/"+t.ID.String())
}

func (s *Server) handleTableImport(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	file, err := c.FormFile("csv")
	if err != nil {
		s.flashError(c, "Choose a CSV file", err)
		c.Redirect(http.StatusSeeOther, "/tables")
		return
	}
	f, err := file.Open()
	if err != nil {
		s.flashError(c, "Failed to read the upload", err)
		c.Redirect(http.StatusSeeOther, "/tables")
		return
	}
	defer f.Close()

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = strings.TrimSuffix(file.Filename, ".csv")
	}
	res, err := tables.ImportCSV(c.Request.Context(), s.db, user.ID, title, strings.TrimSpace(c.PostForm("sql_name")), f)
	if err != nil {
		s.flashError(c, "Import failed", err)
		c.Redirect(http.StatusSeeOther, "/tables")
		return
	}
	s.setFlash(c, "success", "Imported rows: "+strconv.Itoa(res.Rows))
	s.redirectTo(c, "/tables/"+res.Table.ID.String())
}

func (s *Server) handleTableExportCSV(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	t, ok := s.loadTable(c, user)
	if !ok {
		return
	}
	rows, err := tables.Rows(c.Request.Context(), s.db, user.ID, t.ID, 0, 0)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Export failed", err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", contentDispositionAttachment(t.SQLName+".csv"))
	if err := tables.WriteCSV(c.Writer, t, rows); err != nil {
		logRequestError(c, "CSV export failed", err)
	}
}

func (s *Server) handleTableDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	t, ok := s.loadTable(c, user)
	if !ok {
		return
	}
	if err := core.SoftDeleteEntity(c.Request.Context(), s.db, user.ID, t.ID); err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to delete table", err)
		return
	}
	s.setFlash(c, "success", "Table moved to the trash.")
	s.redirectTo(c, "/tables")
}

// --- JSON API used by static/tables.js ----------------------------------

func (s *Server) handleTableData(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	t, ok := s.loadTable(c, user)
	if !ok {
		return
	}
	rows, err := tables.Rows(c.Request.Context(), s.db, user.ID, t.ID, 0, 0)
	if err != nil {
		s.jsonError(c, http.StatusInternalServerError, "Failed to load rows", err)
		return
	}
	if rows == nil {
		rows = []tables.Row{}
	}
	c.JSON(http.StatusOK, gin.H{"table": t, "rows": rows})
}

func (s *Server) handleTableRowCreate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	var body struct {
		Values map[string]any `json:"values"`
	}
	if !s.bindJSON(c, &body) {
		return
	}
	tableID, ok := s.paramUUID(c, "id")
	if !ok {
		return
	}
	if body.Values == nil {
		body.Values = map[string]any{}
	}
	rows, err := tables.InsertRows(c.Request.Context(), s.db, user.ID, tableID, []map[string]any{body.Values})
	if err != nil {
		s.jsonError(c, http.StatusBadRequest, "Failed to add row", err)
		return
	}
	c.JSON(http.StatusCreated, rows[0])
}

func (s *Server) handleTableRowUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	var body struct {
		Version int            `json:"version"`
		Values  map[string]any `json:"values"`
	}
	if !s.bindJSON(c, &body) {
		return
	}
	tableID, ok := s.paramUUID(c, "id")
	if !ok {
		return
	}
	rowID, ok := s.paramUUID(c, "row")
	if !ok {
		return
	}
	version, err := tables.UpdateRow(c.Request.Context(), s.db, user.ID, tableID, rowID, body.Version, body.Values)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tables.ErrConflict) {
			status = http.StatusConflict
		}
		s.jsonError(c, status, "Failed to save", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": version})
}

func (s *Server) handleTableRowDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	tableID, ok := s.paramUUID(c, "id")
	if !ok {
		return
	}
	rowID, ok := s.paramUUID(c, "row")
	if !ok {
		return
	}
	if err := tables.DeleteRow(c.Request.Context(), s.db, user.ID, tableID, rowID); err != nil {
		s.jsonError(c, http.StatusNotFound, "Failed to delete row", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleTableColumnsUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	var body struct {
		SchemaVersion int             `json:"schema_version"`
		Columns       []tables.Column `json:"columns"`
	}
	if !s.bindJSON(c, &body) {
		return
	}
	tableID, ok := s.paramUUID(c, "id")
	if !ok {
		return
	}
	cleared, err := tables.SetColumns(c.Request.Context(), s.db, user.ID, tableID, body.SchemaVersion, body.Columns)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, tables.ErrConflict) {
			status = http.StatusConflict
		}
		s.jsonError(c, status, "Failed to save columns", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cleared": cleared})
}

// --- helpers ------------------------------------------------------------

func (s *Server) loadTable(c *gin.Context, user *core.User) (*tables.Table, bool) {
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Table not found")
		return nil, false
	}
	t, err := tables.Get(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.respondError(c, http.StatusNotFound, "Table not found", err)
		return nil, false
	}
	return t, true
}

func (s *Server) paramUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := core.ParseUUID(c.Param(name))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) bindJSON(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(c.Request.Body)
	dec.UseNumber() // keep numbers exact
	if err := dec.Decode(dst); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return false
	}
	return true
}

// jsonError logs err and responds with {"error": safe message}.
func (s *Server) jsonError(c *gin.Context, status int, message string, err error) {
	logRequestError(c, message, err)
	c.JSON(status, gin.H{"error": userFacingError(message, err)})
}

// redirectTo navigates htmx requests with HX-Redirect and plain ones with 303.
func (s *Server) redirectTo(c *gin.Context, url string) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", url)
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}
