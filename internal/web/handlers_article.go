package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"html/template"
	"net/http"
	"strconv"

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

	var title, domain string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, domain FROM articles_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &domain)
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
		"Title":   title,
		"Domain":  domain,
		"Content": template.HTML(htmlContent),
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
