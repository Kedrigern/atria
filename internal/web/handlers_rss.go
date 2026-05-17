package web

import (
	"atria/internal/core"
	"atria/internal/rss"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

func (s *Server) handleRSS(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := user.Preferences.PaginationSize
	if limit <= 0 {
		limit = 30
	}
	offset := (page - 1) * limit
	activeTag := c.Query("tag")

	var items []core.RSSItem
	var err error
	if activeTag != "" {
		items, err = rss.ListItemsToReadByTag(c.Request.Context(), s.db, user.ID, activeTag, limit+1, offset)
	} else {
		items, err = rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
	}
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load RSS items: "+err.Error())
		return
	}

	tags, err := rss.ListFeedTags(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load feed tags: "+err.Error())
		return
	}

	hasNext := false
	if len(items) > limit {
		hasNext = true
		items = items[:limit]
	}

	s.render(c, "rss.html", gin.H{
		"Items":     items,
		"Page":      page,
		"HasNext":   hasNext,
		"NextPage":  page + 1,
		"PrevPage":  page - 1,
		"Tags":      tags,
		"ActiveTag": activeTag,
	})
}

func (s *Server) handleRSSAdd(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	title := c.PostForm("title")
	urlStr := c.PostForm("url")

	if urlStr == "" {
		s.renderError(c, http.StatusBadRequest, "URL is required")
		return
	}
	if title == "" {
		title = urlStr // Fallback, pokud uživatel nevyplní název
	}

	_, err := rss.CreateFeed(c.Request.Context(), s.db, user.ID, title, urlStr)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to add feed: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// HX-Refresh triggers a full page reload in HTMX to show the updated list and clear the form
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Feed added.")
	c.Redirect(http.StatusSeeOther, "/rss/feeds")
}

func (s *Server) handleRSSSave(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	_, err = rss.SaveItemAsArticle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		s.renderSnippet(c, "badge_saved", nil)
		return
	}

	s.setFlash(c, "success", "RSS item saved to Inbox.")
	c.Redirect(http.StatusSeeOther, "/rss")
}

func (s *Server) handleRSSFetch(c *gin.Context) {
	// Triggers the parallel worker pool to fetch all feeds
	err := rss.FetchAllActiveFeeds(c.Request.Context(), s.db)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Fetch failed: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// Reload the page to display newly fetched items
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Feeds fetched.")
	c.Redirect(http.StatusSeeOther, "/rss/feeds")
}

func (s *Server) handleRSSFeeds(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	feeds, err := rss.ListFeeds(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to list feeds: "+err.Error())
		return
	}
	s.render(c, "rss_feeds.html", gin.H{
		"Feeds": feeds,
	})
}

func (s *Server) handleRSSArchive(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = rss.MarkAsRead(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to archive RSS item: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		s.renderSnippet(c, "badge_archived", nil)
		return
	}

	s.setFlash(c, "success", "RSS item archived.")
	c.Redirect(http.StatusSeeOther, "/rss")
}

func (s *Server) handleRSSFeedDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	feedID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid feed ID")
		return
	}

	includeRead := c.Query("archived") == "1"

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := user.Preferences.PaginationSize
	if limit <= 0 {
		limit = 30
	}
	offset := (page - 1) * limit

	detail, err := rss.GetFeedDetail(c.Request.Context(), s.db, user.ID, feedID, includeRead, limit, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load feed: "+err.Error())
		return
	}

	unreadItems := detail.TotalItems - detail.ReadItems

	s.render(c, "rss_feed_detail.html", gin.H{
		"Feed":            detail,
		"Items":           detail.Items,
		"TotalItems":      detail.TotalItems,
		"ReadItems":       detail.ReadItems,
		"UnreadItems":     unreadItems,
		"IncludeArchived": includeRead,
		"Page":            page,
		"HasNext":         detail.HasMore,
		"NextPage":        page + 1,
		"PrevPage":        page - 1,
	})
}

func (s *Server) handleRSSArchiveBatch(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	// HTMX will send the IDs as an array from the hidden inputs
	idStrs := c.PostFormArray("ids")
	var ids []uuid.UUID

	for _, idStr := range idStrs {
		if id, err := core.ParseUUID(idStr); err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) > 0 {
		err := rss.MarkBatchAsRead(c.Request.Context(), s.db, user.ID, ids)
		if err != nil {
			s.renderError(c, http.StatusInternalServerError, "Failed to archive batch: "+err.Error())
			return
		}
	}

	page := 1
	if p, err := strconv.Atoi(c.DefaultPostForm("page", "1")); err == nil && p > 0 {
		page = p
	}

	limit := user.Preferences.PaginationSize
	if limit <= 0 {
		limit = 30
	}
	offset := (page - 1) * limit

	items, err := rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to refresh RSS items: "+err.Error())
		return
	}

	if len(items) == 0 && page > 1 {
		page--
		offset = (page - 1) * limit
		items, err = rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
		if err != nil {
			s.renderError(c, http.StatusInternalServerError, "Failed to refresh RSS items: "+err.Error())
			return
		}
	}

	if c.GetHeader("HX-Request") != "true" {
		s.setFlash(c, "success", "Page archived.")
		c.Redirect(http.StatusSeeOther, "/rss?page="+strconv.Itoa(page))
		return
	}

	hasNext := false
	if len(items) > limit {
		hasNext = true
		items = items[:limit]
	}

	s.render(c, "rss.html", gin.H{
		"Items":    items,
		"Page":     page,
		"HasNext":  hasNext,
		"NextPage": page + 1,
		"PrevPage": page - 1,
	})
}
