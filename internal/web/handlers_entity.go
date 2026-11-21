package web

import (
<<<<<<< Updated upstream
	"atria/internal/core"
	"net/http"
	"net/url"
	"strings"
=======
	"net/http"
>>>>>>> Stashed changes

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

<<<<<<< Updated upstream
=======
// handleUpdateVisibility zpracovává změnu viditelnosti jakékoliv entity
>>>>>>> Stashed changes
func (s *Server) handleUpdateVisibility(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

<<<<<<< Updated upstream
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
=======
	entityIDStr := c.Param("id")
	newVisibility := c.PostForm("visibility")

	// 1. Validace ID a hodnoty
	entityID, err := uuid.FromString(entityIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Neplatné ID entity")
		return
	}

	if newVisibility != "private" && newVisibility != "users" && newVisibility != "public" {
		c.String(http.StatusBadRequest, "Neplatná hodnota viditelnosti")
		return
	}

	// 2. Provedení updatu (ideálně by to bylo volání např. entities.UpdateVisibility)
	query := `
		UPDATE entities
		SET visibility = $1, updated_at = CURRENT_TIMESTAMP, updated_by = $2
		WHERE id = $3 AND owner_id = $4
	`
	res, err := s.db.ExecContext(c.Request.Context(), query, newVisibility, user.ID, entityID, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při aktualizaci databáze")
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		c.String(http.StatusNotFound, "Entita nenalezena nebo nemáš oprávnění k úpravě")
		return
	}

	// 3. Odpověď pro HTMX (hx-swap="none" na frontendu ignoruje obsah, takže 200 OK stačí)
	c.Status(http.StatusOK)
}

// STUB: handleSoftDeleteEntity
func (s *Server) handleSoftDeleteEntity(c *gin.Context) {
	// UPDATE entities SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?
}

// STUB: handleRenameEntity
func (s *Server) handleRenameEntity(c *gin.Context) {
	// UPDATE entities SET title = ?, slug = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
}

// STUB: handleMoveEntity
func (s *Server) handleMoveEntity(c *gin.Context) {
	// UPDATE entities SET parent_id = ? WHERE id = ?
>>>>>>> Stashed changes
}
