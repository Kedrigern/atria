package web

import (
	"atria/internal/attachments"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAttachments(c *gin.Context) {
	user := s.getDummyUser(c)
	list, err := attachments.ListAttachments(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při načítání příloh: "+err.Error())
		return
	}

	s.render(c, "attachment_list.html", gin.H{
		"Attachments": list,
	})
}
