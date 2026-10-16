package web

import (
	"atria/internal/core"
	"atria/internal/ebooks"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

func (s *Server) handleEbookList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	author := c.Query("author")
	books, err := ebooks.List(c.Request.Context(), s.db, user.ID, author)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load e-books", err)
		return
	}
	authors, err := ebooks.Authors(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load e-book authors", err)
		return
	}
	s.render(c, "ebook_list.html", gin.H{"Ebooks": books, "Authors": authors, "SelectedAuthor": author})
}

func (s *Server) handleEbookNew(c *gin.Context) {
	s.render(c, "ebook_form.html", gin.H{})
}

func (s *Server) handleEbookImportForm(c *gin.Context) {
	s.render(c, "ebook_import.html", gin.H{})
}

func (s *Server) handleEbookImport(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	file, err := c.FormFile("epub")
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "An EPUB file is required")
		return
	}
	tempPath, cleanup, err := saveEbookImportUpload(file)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to save imported EPUB", err)
		return
	}
	defer cleanup()

	book, err := ebooks.Import(c.Request.Context(), s.db, user.ID, ebooks.ImportInput{
		Title:       c.PostForm("title"),
		Description: c.PostForm("description"),
		Author:      c.PostForm("author"),
		Language:    c.PostForm("language"),
		Path:        tempPath,
		Filename:    file.Filename,
	})
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to import EPUB", err)
		return
	}

	s.setFlash(c, "success", "E-book imported.")
	url := "/ebooks/" + book.ID.String()
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", url)
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

func saveEbookImportUpload(file *multipart.FileHeader) (string, func(), error) {
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}
	tempDir := filepath.Join(storagePath, ".tmp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", func() {}, fmt.Errorf("create import temporary directory: %w", err)
	}

	tempPath, cleanup, err := saveUploadToTemp(file, tempDir, "ebook-import-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("save uploaded EPUB: %w", err)
	}
	return tempPath, cleanup, nil
}

// saveUploadToTemp copies an uploaded file into a new, uniquely named temp
// file in dir ("" = system temp dir). Unlike gin's SaveUploadedFile it never
// touches the permissions of dir.
func saveUploadToTemp(file *multipart.FileHeader, dir, pattern string) (string, func(), error) {
	source, err := file.Open()
	if err != nil {
		return "", func() {}, err
	}
	defer source.Close()

	temp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", func() {}, err
	}
	tempPath := temp.Name()
	cleanup := func() { _ = os.Remove(tempPath) }

	if _, err := io.Copy(temp, source); err != nil {
		_ = temp.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tempPath, cleanup, nil
}

func (s *Server) handleEbookCreate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		s.renderError(c, http.StatusBadRequest, "Title is required")
		return
	}

	sources, skipped, err := s.resolveEbookSources(c, user.ID, c.PostForm("source_ids"))
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to resolve sources", err)
		return
	}
	if len(sources) == 0 {
		s.renderError(c, http.StatusBadRequest, "Provide at least one valid article or note ID")
		return
	}

	coverPath, coverName, cleanup, err := saveEbookCover(c)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to save cover image", err)
		return
	}
	defer cleanup()

	book, err := ebooks.Generate(c.Request.Context(), s.db, user.ID, ebooks.GenerateInput{
		Title:       title,
		Description: c.PostForm("description"),
		Author:      c.PostForm("author"),
		Language:    c.PostForm("language"),
		CoverPath:   coverPath,
		CoverName:   coverName,
		Filename:    c.PostForm("filename"),
		Sources:     sources,
	})
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to generate e-book", err)
		return
	}

	message := "E-book created."
	if skipped > 0 {
		message += " Some invalid or unsupported source IDs were skipped."
	}
	s.setFlash(c, "success", message)
	url := "/ebooks/" + book.ID.String()
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", url)
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

func (s *Server) handleEbookDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	book, err := ebooks.GetDetail(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	s.render(c, "ebook_detail.html", gin.H{"Ebook": book})
}

func (s *Server) handleEbookEdit(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	book, err := ebooks.GetDetail(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	s.render(c, "ebook_edit.html", gin.H{"Ebook": book})
}

func (s *Server) handleEbookUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	err = ebooks.UpdateMetadata(c.Request.Context(), s.db, user.ID, id, c.PostForm("title"), c.PostForm("description"), c.PostForm("author"), c.PostForm("language"))
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to update e-book", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/ebooks/"+id.String())
}

func (s *Server) handleEbookDownload(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	download, err := ebooks.GetDownload(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}

	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}
	path := filepath.Join(storagePath, download.DiskPath)
	if _, err := os.Stat(path); err != nil {
		s.renderError(c, http.StatusNotFound, "E-book file not found")
		return
	}
	c.FileAttachment(path, download.Filename)
}

func (s *Server) handleEbookDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}
	if err := ebooks.Delete(c.Request.Context(), s.db, user.ID, id); err != nil {
		s.renderError(c, http.StatusNotFound, "E-book not found")
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/ebooks")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, "/ebooks")
}

func (s *Server) resolveEbookSources(c *gin.Context, ownerID uuid.UUID, rawIDs string) ([]core.EntitySummary, int, error) {
	identifiers := strings.Fields(rawIDs)
	sources := make([]core.EntitySummary, 0, len(identifiers))
	skipped := 0
	for _, identifier := range identifiers {
		entities, err := core.FindEntities(c.Request.Context(), s.db, ownerID, "", identifier, false)
		if err != nil {
			return nil, skipped, err
		}
		if len(entities) != 1 || (entities[0].Type != core.TypeArticle && entities[0].Type != core.TypeNote) {
			skipped++
			continue
		}
		sources = append(sources, entities[0])
	}
	return sources, skipped, nil
}

func saveEbookCover(c *gin.Context) (string, string, func(), error) {
	file, err := c.FormFile("cover")
	if err != nil {
		return "", "", func() {}, nil
	}

	tempPath, cleanup, err := saveUploadToTemp(file, "", "atria-ebook-cover-*")
	if err != nil {
		return "", "", func() {}, err
	}
	return tempPath, file.Filename, cleanup, nil
}
