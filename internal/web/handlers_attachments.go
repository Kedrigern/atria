package web

import (
	"atria/internal/attachments"
	"atria/internal/core"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strings"

	"database/sql"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAttachments(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	list, err := attachments.ListAttachments(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Could not load the attachments", err)
		return
	}

	var totalBytes int64
	for _, a := range list {
		totalBytes += int64(a.SizeBytes)
	}

	var totalFormatted string
	if totalBytes > 1024*1024*1024 {
		totalFormatted = fmt.Sprintf("%.2f GB", float64(totalBytes)/(1024*1024*1024))
	} else {
		totalFormatted = fmt.Sprintf("%.2f MB", float64(totalBytes)/(1024*1024))
	}

	s.render(c, "settings_attachments.html", gin.H{
		"Attachments": list,
		"TotalSize":   totalFormatted,
		"SettingsTab": "attachments",
	})
}

func (s *Server) handleEntityAttachmentUpload(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}
	if !s.requireOwnedEntity(c, user, entityID) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "No file uploaded (or file too large)")
		return
	}

	tempPath, cleanup, err := saveUploadToTemp(file, "", "atria-upload-*")
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save temp file")
		return
	}
	defer cleanup()

	att, err := attachments.AddAttachment(c.Request.Context(), s.db, user.ID, tempPath, file.Filename)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Upload failed", err)
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
	c.Redirect(http.StatusSeeOther, localRedirectTarget(c, "/"))
}

// inlineAttachmentTypes are served inline; every other type is forced to
// download so uploaded HTML/SVG/JS can never execute on the Atria origin.
var inlineAttachmentTypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"image/webp":      true,
	"image/avif":      true,
	"image/bmp":       true,
	"application/pdf": true,
	"text/plain":      true,
}

func isInlineAttachmentType(mimeType string) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	return inlineAttachmentTypes[mediaType] ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "video/")
}

func (s *Server) handleProtectedAttachment(c *gin.Context) {
	relDiskPath := filepath.Join(c.Param("year"), c.Param("month"), c.Param("filename"))

	// This route is outside the auth group so public attachments work for
	// anonymous visitors; identify the caller with the same rules as AuthMiddleware.
	user, _, _ := s.resolveRequestUser(c)

	// Several users can own the same file (dedup is per owner, the disk path
	// is content-addressed), so check access across all matching rows.
	query := `SELECT mime_type, filename FROM attachments
		WHERE disk_path = $1 AND deleted_at IS NULL AND visibility = 'public'
		LIMIT 1`
	args := []any{relDiskPath}
	if user != nil {
		query = `SELECT mime_type, filename FROM attachments
			WHERE disk_path = $1 AND deleted_at IS NULL
			  AND (owner_id = $2 OR visibility IN ('users', 'public'))
			ORDER BY (owner_id = $2) DESC
			LIMIT 1`
		args = append(args, user.ID)
	}

	var mimeType, filename string
	err := s.db.QueryRowContext(c.Request.Context(), query, args...).Scan(&mimeType, &filename)
	if err == sql.ErrNoRows {
		c.String(http.StatusNotFound, "Attachment not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "Database error while reading the attachment")
		return
	}

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}
	absPath := filepath.Join(storagePath, relDiskPath)

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "The file is missing from the server disk")
		return
	}

	// Content-Type comes from the sniffed MIME type stored at upload time,
	// never from the user-controlled file extension.
	h := c.Writer.Header()
	h.Set("Content-Type", mimeType)
	h.Set("X-Content-Type-Options", "nosniff")
	if isInlineAttachmentType(mimeType) {
		// Built-in viewers (e.g. PDF) can break under the page CSP; these
		// types cannot run script on our origin anyway.
		h.Del("Content-Security-Policy")
	} else {
		h.Set("Content-Disposition", contentDispositionAttachment(filename))
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	c.File(absPath)
}

func (s *Server) handleAttachmentRename(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	attID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	// HTMX sends the hx-prompt value in this header.
	newName := c.GetHeader("HX-Prompt")
	if newName != "" {
		err = attachments.RenameAttachment(c.Request.Context(), s.db, user.ID, attID, newName)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Failed to rename", err)
			return
		}
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/attachments")
}

func (s *Server) handleAttachmentDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	attID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = attachments.DeleteAttachment(c.Request.Context(), s.db, user.ID, attID)
	if err != nil {
		s.flashError(c, "Could not delete the attachment", err)
	} else {
		s.setFlash(c, "success", "The attachment is deleted permanently.")
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/attachments")
}
