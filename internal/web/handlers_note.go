package web

import (
	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/export"
	"atria/internal/links"
	"atria/internal/notes"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleNotes(c *gin.Context) {
	user := s.getDummyUser(c)
	list, err := notes.ListNotes(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load notes: "+err.Error())
		return
	}

	groupedNotes := make(map[string][]core.Entity)
	for _, n := range list {
		groupedNotes[n.Path] = append(groupedNotes[n.Path], n)
	}

	s.render(c, "note_list.html", gin.H{
		"GroupedNotes": groupedNotes,
	})
}

func (s *Server) handleNoteDetail(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, path, shortID string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, path, short_id FROM notes_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &path, &shortID)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	mdContent, err := notes.GetNoteContent(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load note content")
		return
	}

	htmlStr, _, err := core.RenderMarkdown([]byte(mdContent))
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to render markdown")
		return
	}
	htmlContent := template.HTML(htmlStr)

	tags, _ := core.GetEntityTags(c.Request.Context(), s.db, id)
	atts, _ := attachments.GetEntityAttachments(c.Request.Context(), s.db, id)
	outgoingLinks, incomingLinks, _ := links.GetEntityLinks(c.Request.Context(), s.db, id)

	s.render(c, "note_detail.html", gin.H{
		"ID":            id.String(),
		"ShortID":       shortID,
		"Title":         title,
		"Path":          path,
		"RawContent":    mdContent,
		"Content":       template.HTML(htmlContent),
		"Tags":          tags,
		"Attachments":   atts,
		"OutgoingLinks": outgoingLinks,
		"IncomingLinks": incomingLinks,
	})
}

func (s *Server) handleNoteUpdate(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	newContent := c.PostForm("markdown_content")
	err = notes.UpdateNoteContent(c.Request.Context(), s.db, user.ID, id, newContent)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save note: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/notes/"+id.String())
}

func (s *Server) handleNoteAdd(c *gin.Context) {
	s.render(c, "note_form.html", gin.H{})
}

func (s *Server) handleNoteCreate(c *gin.Context) {
	user := s.getDummyUser(c)
	title := c.PostForm("title")
	path := c.PostForm("path")
	content := c.PostForm("content")

	if title == "" {
		s.renderError(c, http.StatusBadRequest, "Title is required")
		return
	}

	note, err := notes.CreateNote(c.Request.Context(), s.db, user.ID, title, path, content)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create note: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/notes/"+note.ID.String())
		c.Status(http.StatusOK)
		return
	}

	c.Redirect(http.StatusSeeOther, "/notes/"+note.ID.String())
}

// handleEntityLinkAdd
func (s *Server) handleEntityLinkAdd(c *gin.Context) {
	user := s.getDummyUser(c)
	sourceID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid source ID")
		return
	}

	targetStr := c.PostForm("target_identifier")
	if targetStr == "" {
		s.renderError(c, http.StatusBadRequest, "Target is required")
		return
	}

	var targetID uuid.UUID

	if parsedID, err := core.ParseUUID(targetStr); err == nil {
		query := `SELECT id FROM entities WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL LIMIT 1`
		err = s.db.QueryRowContext(c.Request.Context(), query, parsedID, user.ID).Scan(&targetID)
	} else {
		query := `SELECT id FROM entities WHERE owner_id = $1 AND deleted_at IS NULL AND (short_id = $2 OR title = $3) LIMIT 1`
		err = s.db.QueryRowContext(c.Request.Context(), query, user.ID, targetStr, targetStr).Scan(&targetID)
	}

	if err == sql.ErrNoRows {
		s.renderError(c, http.StatusNotFound, "Entita nenalezena. Zkontrolujte název, Short ID nebo UUID.")
		return
	} else if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Database error: "+err.Error())
		return
	}

	if sourceID == targetID {
		s.renderError(c, http.StatusBadRequest, "Nemůžete prolinkovat entitu samu na sebe.")
		return
	}

	if err := links.AddLink(c.Request.Context(), s.db, sourceID, targetID, "user_defined"); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create link")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, c.Request.Referer())
}

func (s *Server) handleNoteDelete(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := core.SoftDeleteEntity(c.Request.Context(), s.db, user.ID, id); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to delete note")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/notes")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/notes")
}

func (s *Server) handleNoteExportMD(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, slug, path string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, slug, path FROM notes_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &slug, &path)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	content, err := notes.GetNoteContent(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load note content")
		return
	}

	tags, _ := core.GetEntityTags(c.Request.Context(), s.db, id)
	tagValues := make([]string, 0, len(tags))
	for _, t := range tags {
		tagValues = append(tagValues, t.Name)
	}

	frontMatterData := map[string]interface{}{
		"id":    id.String(),
		"title": title,
		"path":  path,
		"tags":  tagValues,
	}
	frontMatterBytes, err := yaml.Marshal(frontMatterData)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to generate export metadata")
		return
	}

	finalOutput := "---\n" + string(frontMatterBytes) + "---\n\n" + content

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.md\"", slug))
	c.Data(http.StatusOK, "text/markdown", []byte(finalOutput))
}

func (s *Server) handleNoteExportEPUB(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, slug string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, slug FROM entities WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &slug)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	// Vytvoříme dočasný soubor pro EPUB
	tempFile, err := os.CreateTemp("", "atria-note-*.epub")
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create temp file")
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath) // Soubor se smaže, jakmile skončí request

	items := []core.EntitySummary{{ID: id, Title: title, Type: core.TypeNote}}
	if err := export.ExportEPUB(c.Request.Context(), s.db, items, tempPath); err != nil {
		s.renderError(c, http.StatusInternalServerError, "EPUB generation failed")
		return
	}

	c.FileAttachment(tempPath, slug+".epub")
}
