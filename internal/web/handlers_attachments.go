package web

import (
	"atria/internal/attachments"
	"atria/internal/core"
	"net/http"
	"os"

	"database/sql"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
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

	s.render(c, "settings_attachments.html", gin.H{
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

	att, err := attachments.AddAttachment(c.Request.Context(), s.db, user.ID, tempPath, file.Filename)
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

func (s *Server) handleProtectedAttachment(c *gin.Context) {
	year := c.Param("year")
	month := c.Param("month")
	filename := c.Param("filename")

	relDiskPath := filepath.Join(year, month, filename)

	// --- RUČNÍ EXTRAKCE UŽIVATELE (kvůli podpoře public sdílení mimo auth group) ---
	var user *core.User
	headerName := os.Getenv("PROXY_AUTH_HEADER")
	if headerName == "" {
		headerName = "Remote-Email"
	}

	var email string
	proxyEmail := c.GetHeader(headerName)

	// Ověříme proxy auth nebo session cookie stejně jako v middleware
	if proxyEmail != "" && isProxyAllowed(c) {
		email = proxyEmail
	} else {
		email = s.verifySessionCookie(c)
	}

	// Dev fallback
	if email == "" && os.Getenv("ATRIA_ENV") == "development" {
		email = os.Getenv("ATRIA_USER")
	}

	if email != "" {
		user, _ = core.FindUser(c.Request.Context(), s.db, email)
	}
	// -------------------------------------------------------------------------------

	var ownerID uuid.UUID
	var visibility string
	var actualFilename string

	query := `SELECT owner_id, visibility, filename FROM attachments WHERE disk_path = $1 AND deleted_at IS NULL`
	err := s.db.QueryRowContext(c.Request.Context(), query, relDiskPath).Scan(&ownerID, &visibility, &actualFilename)

	if err == sql.ErrNoRows {
		c.String(http.StatusNotFound, "Příloha nebyla nalezena")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "Chyba databáze při ověřování přílohy")
		return
	}

	// Access Control Logic
	allowed := false
	if visibility == "public" {
		allowed = true
	} else if user != nil {
		if visibility == "users" {
			allowed = true
		} else if visibility == "private" && ownerID == user.ID {
			allowed = true
		}
	}

	if !allowed {
		c.String(http.StatusForbidden, "Nemáte oprávnění k zobrazení tohoto souboru")
		return
	}

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}
	absPath := filepath.Join(storagePath, relDiskPath)

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "Soubor chybí na úložném disku serveru")
		return
	}

	c.File(absPath)
}
