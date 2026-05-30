package web

import (
	"atria/internal/core"
	"net/http"

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
