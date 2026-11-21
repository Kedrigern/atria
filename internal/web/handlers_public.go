package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

func (s *Server) handlePublicArticle(c *gin.Context) {
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Public article not found")
		return
	}

	article, err := articles.GetPublicArticle(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Public article not found")
		return
	}

	var content template.HTML
	if article.HTMLContent != nil {
		content = template.HTML(core.SanitizeHTML(*article.HTMLContent))
	}

	s.render(c, "public_detail.html", gin.H{
		"Title":     article.Title,
		"Content":   content,
		"SourceURL": article.OriginalURL,
	})
}

func (s *Server) handlePublicNote(c *gin.Context) {
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Public note not found")
		return
	}

	note, err := notes.GetPublicNote(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Public note not found")
		return
	}

	htmlContent, _, err := core.RenderMarkdown([]byte(note.MarkdownContent))
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to render public note")
		return
	}

	s.render(c, "public_detail.html", gin.H{
		"Title":   note.Title,
		"Content": template.HTML(htmlContent),
	})
}

// renderShared shows a note or an article of another user, which its owner
// made visible to all users, read-only: no editing, tags, links or notes of
// the owner.
func (s *Server) renderShared(c *gin.Context, ownerID uuid.UUID, title string, content template.HTML, sourceURL string, renderMath bool) {
	ownerName := "another user"
	if owner, err := core.FindUser(c.Request.Context(), s.db, ownerID.String()); err == nil && owner.DisplayName != "" {
		ownerName = owner.DisplayName
	}
	s.render(c, "shared_detail.html", gin.H{
		"Title":      title,
		"Owner":      ownerName,
		"Content":    content,
		"SourceURL":  sourceURL,
		"RenderMath": renderMath,
	})
}
