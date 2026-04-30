package web

import (
	"atria/internal/attachments"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAttachments(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	list, err := attachments.ListAttachments(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při načítání příloh: "+err.Error())
		return
	}

	s.render(c, "attachment_list.html", gin.H{
		"Attachments": list,
	})
}

func (s *Server) handleEntityAttachmentUpload(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, ok := s.getUUIDParam(c, "id")
	if !ok {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Nebyl nahrán žádný soubor")
		return
	}

	tempFile, err := os.CreateTemp("./data", "upload-*.tmp")
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to create temp file")
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save uploaded file")
		return
	}

	att, err := attachments.AddAttachment(c.Request.Context(), s.db, user.ID, tempPath)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Upload failed: "+err.Error())
		return
	}

	if err := attachments.LinkAttachment(c.Request.Context(), s.db, entityID, att.ID); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Linking failed")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, c.Request.Referer())
}
