package web

import (
	"atria/internal/core"
	"atria/internal/notes"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/russross/blackfriday/v2"
)

func (s *Server) handleNotes(c *gin.Context) {
	user := s.getDummyUser(c)
	list, err := notes.ListNotes(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load notes: "+err.Error())
		return
	}
	s.render(c, "note_list.html", gin.H{
		"Notes": list,
	})
}

func (s *Server) handleNoteDetail(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, path string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, path FROM notes_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &path)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	mdContent, err := notes.GetNoteContent(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load note content: "+err.Error())
		return
	}
	htmlContent := blackfriday.Run([]byte(mdContent))

	tags, err := core.GetEntityTags(c.Request.Context(), s.db, id)
	if err != nil {
		tags = []core.Tag{}
	}

	s.render(c, "note_detail.html", gin.H{
		"ID":      id.String(),
		"Title":   title,
		"Path":    path,
		"Content": template.HTML(htmlContent),
		"Tags":    tags,
	})
}
