package web

import (
	"atria/internal/articles"
	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/export"
	"atria/internal/links"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleRead(c *gin.Context) {
	user := s.getDummyUser(c)

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := user.Preferences.PaginationSize
	if limit <= 0 {
		limit = 30
	}
	offset := (page - 1) * limit

	list, err := articles.ListArticles(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load articles: "+err.Error())
		return
	}

	hasNext := false
	if len(list) > limit {
		hasNext = true
		list = list[:limit]
	}

	s.render(c, "read_list.html", gin.H{
		"Articles": list,
		"Page":     page,
		"HasNext":  hasNext,
		"NextPage": page + 1,
		"PrevPage": page - 1,
	})
}

func (s *Server) handleReadDetail(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, shortID, domain, originalURL string
	var createdAt time.Time
	var userNote sql.NullString

	query := `SELECT title, domain, original_url, created_at, COALESCE(user_note, ''), short_id
              FROM articles_full_view WHERE id = $1 AND owner_id = $2`
	err = s.db.QueryRowContext(c.Request.Context(), query, id, user.ID).
		Scan(&title, &domain, &originalURL, &createdAt, &userNote, &shortID)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Article not found")
		return
	}

	htmlContent, err := articles.GetArticleHTML(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load article content: "+err.Error())
		return
	}

	tags, _ := core.GetEntityTags(c.Request.Context(), s.db, id)
	if err != nil {
		tags = []core.Tag{}
	}
	atts, _ := attachments.GetEntityAttachments(c.Request.Context(), s.db, id)
	if err != nil {
		atts = []core.Attachment{}
	}
	outgoingLinks, incomingLinks, _ := links.GetEntityLinks(c.Request.Context(), s.db, id)

	s.render(c, "read_detail.html", gin.H{
		"ID":            id.String(),
		"ShortID":       shortID,
		"Title":         title,
		"Domain":        domain,
		"OriginalURL":   originalURL,
		"CreatedAt":     createdAt,
		"UserNote":      userNote.String,
		"Content":       template.HTML(htmlContent),
		"Tags":          tags,
		"Attachments":   atts,
		"OutgoingLinks": outgoingLinks,
		"IncomingLinks": incomingLinks,
	})
}

func (s *Server) handleReadArchive(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = articles.ArchiveArticle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to archive article: "+err.Error())
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}

	limit := user.Preferences.PaginationSize
	if limit <= 0 {
		limit = 30
	}
	offset := (page - 1) * limit
	list, err := articles.ListArticles(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to refresh article list: "+err.Error())
		return
	}
	if len(list) == 0 && page > 1 {
		page--
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<span style="color: var(--text-muted); font-size: 0.9rem;">✓ Archived</span>`))
		return
	}

	s.setFlash(c, "success", "Article archived.")
	c.Redirect(http.StatusSeeOther, "/read?page="+strconv.Itoa(page))
}

func (s *Server) handleReadAdd(c *gin.Context) {
	user := s.getDummyUser(c)
	urlStr := c.PostForm("url")
	userNote := c.PostForm("user_note")

	if urlStr == "" {
		s.renderError(c, http.StatusBadRequest, "URL is required")
		return
	}

	_, err := articles.CreateArticle(c.Request.Context(), s.db, user.ID, urlStr, userNote)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save article: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Article saved to Inbox.")
	c.Redirect(http.StatusSeeOther, "/read")
}

func (s *Server) handleReadRefetch(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = articles.RefetchArticle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Refetch failed: "+err.Error())
		return
	}

	s.setFlash(c, "success", "Article content refreshed from source.")
	c.Redirect(http.StatusSeeOther, "/read/"+id.String())
}

// handleReadUpdateNote processes the update of the user note for the given article
func (s *Server) handleReadUpdateNote(c *gin.Context) {
	user := s.getDummyUser(c)
	articleID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	newNote := c.PostForm("user_note")

	if err := articles.UpdateUserNote(c.Request.Context(), s.db, user.ID, articleID, newNote); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to update note: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	c.Redirect(http.StatusSeeOther, "/read/"+articleID.String())
}

func (s *Server) handleReadExportMD(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, slug, originalURL string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, slug, original_url FROM articles_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &slug, &originalURL)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Article not found")
		return
	}

	// Vezmeme hezký Markdown z článku
	content, err := articles.GetArticleMarkdown(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to process article Markdown")
		return
	}

	fm := map[string]interface{}{
		"id":     id.String(),
		"title":  title,
		"source": originalURL,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to build front matter")
		return
	}
	finalOutput := fmt.Sprintf("---\n%s---\n\n%s", string(fmBytes), content)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.md\"", slug))
	c.Data(http.StatusOK, "text/markdown", []byte(finalOutput))
}

func (s *Server) handleReadExportEPUB(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, slug string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, slug FROM entities WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &slug)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Article not found")
		return
	}

	tempFile, err := os.CreateTemp("", "atria-article-*.epub")
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create temp file")
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	items := []core.EntitySummary{{ID: id, Title: title, Type: core.TypeArticle}}
	if err := export.ExportEPUB(c.Request.Context(), s.db, items, tempPath); err != nil {
		s.renderError(c, http.StatusInternalServerError, "EPUB generation failed")
		return
	}

	c.FileAttachment(tempPath, slug+".epub")
}
