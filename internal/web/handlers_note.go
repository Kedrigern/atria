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
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"gopkg.in/yaml.v3"
)

// TreeNode represents a single folder in the notes tree structure
type TreeNode struct {
	Name     string
	Level    int
	Notes    []notes.NoteSummary
	Children map[string]*TreeNode
}

func (s *Server) handleNoteList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	// Fetch flat list of notes optimized for tree building
	list, err := notes.ListNotes(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load notes: "+err.Error())
		return
	}

	root := &TreeNode{
		Name:     "Root",
		Level:    0,
		Children: make(map[string]*TreeNode),
	}

	for _, n := range list {
		current := root
		cleanPath := strings.Trim(n.Path, "/")

		if cleanPath != "" {
			parts := strings.Split(cleanPath, "/")
			for _, part := range parts {
				if _, exists := current.Children[part]; !exists {
					current.Children[part] = &TreeNode{
						Name:     part,
						Level:    current.Level + 1,
						Children: make(map[string]*TreeNode),
					}
				}
				current = current.Children[part]
			}
		}
		// Append note to the final resolved folder
		current.Notes = append(current.Notes, n)
	}

	s.render(c, "note_list.html", gin.H{
		"TreeRoot": root,
	})
}

func (s *Server) handleNoteDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	// 1. Získáme KOMPLETNÍ model poznámky (místo tahání jednotlivých sloupců)
	note, err := notes.GetNote(c.Request.Context(), s.db, id, user.ID)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	// 2. Vyrenderujeme Markdown
	htmlStr, _, err := core.RenderMarkdown([]byte(note.MarkdownContent))
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to render markdown")
		return
	}

	tags, _ := core.GetEntityTags(c.Request.Context(), s.db, id)
	atts, _ := attachments.GetEntityAttachments(c.Request.Context(), s.db, id)
	outgoingLinks, incomingLinks, _ := links.GetEntityLinks(c.Request.Context(), s.db, id)

	s.render(c, "note_detail.html", gin.H{
		"Note":          note,
		"ID":            note.ID.String(),
		"ShortID":       note.ShortID,
		"Title":         note.Title,
		"Path":          note.Path,
		"RawContent":    note.MarkdownContent,
		"Content":       template.HTML(htmlStr),
		"Tags":          tags,
		"Attachments":   atts,
		"OutgoingLinks": outgoingLinks,
		"IncomingLinks": incomingLinks,
	})
}

func (s *Server) handleNoteUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

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
	user := s.getUser(c)
	if user == nil {
		return
	}

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
	user := s.getUser(c)
	if user == nil {
		return
	}

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
	user := s.getUser(c)
	if user == nil {
		return
	}

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
	user := s.getUser(c)
	if user == nil {
		return
	}

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
	user := s.getUser(c)
	if user == nil {
		return
	}

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

	// Create temporaly file for EPUB
	tempFile, err := os.CreateTemp("", "atria-note-*.epub")
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create temp file")
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath) // Delete after request is closed

	items := []core.EntitySummary{{ID: id, Title: title, Type: core.TypeNote}}
	if err := export.ExportEPUB(c.Request.Context(), s.db, items, tempPath); err != nil {
		s.renderError(c, http.StatusInternalServerError, "EPUB generation failed")
		return
	}

	c.FileAttachment(tempPath, slug+".epub")
}
