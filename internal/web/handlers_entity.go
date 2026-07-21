package web

import (
	"atria/internal/core"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleUpdateVisibility(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid ID")
		return
	}

	newVisibility := core.Visibility(c.PostForm("visibility"))
	if newVisibility != core.VisibilityPrivate && newVisibility != core.VisibilityUsers && newVisibility != core.VisibilityPublic {
		c.String(http.StatusBadRequest, "Invalid visibility")
		return
	}

	if err := core.UpdateVisibility(c.Request.Context(), s.db, user.ID, entityID, newVisibility); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to update visibility")
		return
	}

	c.Status(http.StatusOK)
}

func (s *Server) handleEntityRename(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	newTitle := c.PostForm("title")
	if newTitle == "" {
		newTitle = c.GetHeader("HX-Prompt")
	}
	newTitle = strings.TrimSpace(newTitle)

	if newTitle == "" {
		c.Status(http.StatusOK)
		return
	}

	if err := core.RenameEntity(c.Request.Context(), s.db, user.ID, entityID, newTitle); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Nepodařilo se přejmenovat entitu")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, c.Request.Referer())
}
