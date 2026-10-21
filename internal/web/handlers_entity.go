package web

import (
	"atria/internal/core"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
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

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
	}
	c.Status(http.StatusOK)
}

func decodePromptTitle(title string) string {
	decodedTitle, err := url.QueryUnescape(title)
	if err == nil {
		title = decodedTitle
	}
	return strings.TrimSpace(title)
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

	newTitle := decodePromptTitle(c.GetHeader("HX-Prompt"))

	if newTitle == "" {
		c.Status(http.StatusOK)
		return
	}

	if err := core.RenameEntity(c.Request.Context(), s.db, user.ID, entityID, newTitle); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Could not rename the item")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, localRedirectTarget(c, "/"))
}

// requireOwnedEntity responds 404 and returns false unless entityID is an
// active entity owned by user. Use it before any write keyed by a URL ID.
func (s *Server) requireOwnedEntity(c *gin.Context, user *core.User, entityID uuid.UUID) bool {
	ownerID, err := core.VerifyOwner(c.Request.Context(), s.db, entityID)
	if err != nil || ownerID != user.ID {
		s.renderError(c, http.StatusNotFound, "Entity not found")
		return false
	}
	return true
}
