package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleUpdateVisibility(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID := c.Param("id")
	newVisibility := c.PostForm("visibility")

	if newVisibility != "private" && newVisibility != "users" && newVisibility != "public" {
		c.String(http.StatusBadRequest, "Invalid visibility")
		return
	}

	query := `UPDATE entities SET visibility = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND owner_id = $3`
	_, err := s.db.ExecContext(c.Request.Context(), query, newVisibility, entityID, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to update visibility")
		return
	}

	c.Status(http.StatusOK)
}
