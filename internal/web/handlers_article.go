package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"database/sql"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleRead(c *gin.Context) {
	user := s.getDummyUser(c)

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 30
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

	var title, domain, originalURL string
	var createdAt time.Time
	var userNote sql.NullString

	err = s.db.QueryRowContext(c.Request.Context(), `
		SELECT title, domain, original_url, created_at, user_note
		FROM articles_full_view
		WHERE id = $1 AND owner_id = $2
	`, id, user.ID).Scan(&title, &domain, &originalURL, &createdAt, &userNote)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Article not found")
		return
	}

	htmlContent, err := articles.GetArticleHTML(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load article content: "+err.Error())
		return
	}

	s.render(c, "read_detail.html", gin.H{
		"ID":          id.String(),
		"Title":       title,
		"Domain":      domain,
		"OriginalURL": originalURL,
		"CreatedAt":   createdAt,
		"UserNote":    userNote.String,
		"Content":     template.HTML(htmlContent),
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

	limit := 30
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

	if urlStr == "" {
		s.renderError(c, http.StatusBadRequest, "URL is required")
		return
	}

	_, err := articles.CreateArticle(c.Request.Context(), s.db, user.ID, urlStr)
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
