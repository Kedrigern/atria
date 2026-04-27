package web

import (
	"atria/internal/attachments"
	"atria/internal/core"
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

	entityID, _ := core.ParseUUID(c.Param("id"))

	file, err := c.FormFile("file")
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "No file uploaded")
		return
	}

	tempPath := "./data/temp_" + file.Filename
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save temp file")
		return
	}
	defer os.Remove(tempPath)

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
