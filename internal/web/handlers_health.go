package web

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleHealth reports whether the server can reach its database.
// It is unauthenticated and returns no details, so it is safe behind the proxy.
func (s *Server) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	c.Header("Cache-Control", "no-store")
	if err := s.db.PingContext(ctx); err != nil {
		c.String(http.StatusServiceUnavailable, "db unavailable\n")
		return
	}
	c.String(http.StatusOK, "ok\n")
}
