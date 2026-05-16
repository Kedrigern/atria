package web

import (
	"atria/internal/search"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleSearch handles GET /search?q=...&in=...
// - q: search query string
// - in: filter — "notes", "articles", "rss", or "" for all
// Works for both full page and HTMX partial requests (render handles that).
func (s *Server) handleSearch(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	q := c.Query("q")
	filter := c.Query("in")

	results, err := search.Search(c.Request.Context(), s.db, user.ID, q, filter)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Search failed: "+err.Error())
		return
	}

	data := gin.H{
		"Results": results,
		"Query":   q,
		"Filter":  filter,
	}

	// Only return the results fragment when the request comes from the search
	// form itself (target is #search-results). Menu navigation also sends
	// HX-Request but targets #main-content and needs the full page.
	if c.GetHeader("HX-Request") == "true" && c.GetHeader("HX-Target") == "search-results" {
		s.render(c, "search_results.html", data)
		return
	}

	s.render(c, "search.html", data)
}
