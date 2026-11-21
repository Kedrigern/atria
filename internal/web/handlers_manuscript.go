package web

import (
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	"github.com/gin-gonic/gin"

	"atria/internal/core"
	"atria/internal/links"
	"atria/internal/manuscripts"
	"atria/internal/mdcheck"
)

// Manuscripts: one Markdown file per book. See docs/architecture/manuscripts.md.

func (s *Server) handleManuscriptList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	list, err := manuscripts.List(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load manuscripts", err)
		return
	}
	s.render(c, "manuscript_list.html", gin.H{"Manuscripts": list})
}

func (s *Server) handleManuscriptDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	html, _, err := core.RenderMarkdown([]byte(m.Markdown))
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to render the manuscript", err)
		return
	}
	ctx := c.Request.Context()
	tags, _ := core.GetEntityTags(ctx, s.db, user.ID, m.ID)
	outgoing, incoming, _ := links.GetEntityLinks(ctx, s.db, user.ID, m.ID)
	s.render(c, "manuscript_detail.html", gin.H{
		"Manuscript":        m,
		"ID":                m.ID.String(),
		"Check":             mdcheck.Check([]byte(m.Markdown)),
		"Formats":           manuscripts.Formats(),
		"Content":           template.HTML(html),
		"Tags":              tags,
		"OutgoingLinks":     outgoing,
		"CollectionSection": s.collectionSection(c, user, m.ID),
		"IncomingLinks":     incoming,
	})
}

func (s *Server) handleManuscriptEdit(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	s.render(c, "manuscript_edit.html", gin.H{"Manuscript": m})
}

func (s *Server) handleManuscriptDownload(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	name := m.Slug + ".md"
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"manuscript.md\"; filename*=UTF-8''%s", url.PathEscape(name)))
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(m.Markdown))
}

// handleManuscriptImport stores an uploaded Markdown file and opens its page,
// which shows the check of the file.
func (s *Server) handleManuscriptImport(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "A Markdown file is required")
		return
	}
	src, err := readUpload(file)
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to read the file", err)
		return
	}
	m, err := manuscripts.Create(c.Request.Context(), s.db, user.ID, c.PostForm("path"), manuscripts.FromFile(file.Filename, src))
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to import the manuscript", err)
		return
	}
	s.setFlash(c, "success", "Manuscript imported.")
	s.redirectTo(c, "/manuscripts/"+m.ID.String())
}

// handleManuscriptUpdate saves the metadata and the folder. A new file, if
// one is uploaded, replaces the content.
func (s *Server) handleManuscriptUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	in := manuscripts.Input{
		Title:       c.PostForm("title"),
		Author:      c.PostForm("author"),
		Language:    c.PostForm("language"),
		Description: c.PostForm("description"),
		Markdown:    m.Markdown,
	}
	if file, err := c.FormFile("file"); err == nil {
		src, err := readUpload(file)
		if err != nil {
			s.respondError(c, http.StatusBadRequest, "Failed to read the file", err)
			return
		}
		in.Markdown = string(src)
	}
	if err := manuscripts.Update(c.Request.Context(), s.db, user.ID, m.ID, c.PostForm("path"), in); err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to save the manuscript", err)
		return
	}
	s.setFlash(c, "success", "Manuscript saved.")
	s.redirectTo(c, "/manuscripts/"+m.ID.String())
}

func (s *Server) handleManuscriptDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	if err := core.SoftDeleteEntity(c.Request.Context(), s.db, user.ID, m.ID); err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to delete the manuscript", err)
		return
	}
	s.setFlash(c, "success", "Manuscript moved to the trash.")
	s.redirectTo(c, "/manuscripts")
}

func (s *Server) loadManuscript(c *gin.Context, user *core.User) (*manuscripts.Manuscript, bool) {
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return nil, false
	}
	m, err := manuscripts.Get(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Manuscript not found")
		return nil, false
	}
	return m, true
}

// readUpload reads an uploaded file up to the size limit of a manuscript.
func readUpload(file *multipart.FileHeader) ([]byte, error) {
	if file.Size > manuscripts.MaxBytes {
		return nil, core.NewUserError("the file is larger than %d MB", manuscripts.MaxBytes>>20)
	}
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, manuscripts.MaxBytes+1))
}

// handleManuscriptExport sends the manuscript as EPUB, or as DOCX or PDF when
// pandoc (and typst for PDF) are installed.
func (s *Server) handleManuscriptExport(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	format := c.Param("format")
	if format != "epub" && !slices.Contains(manuscripts.Formats(), format) {
		s.renderError(c, http.StatusNotFound, "This export format is not available")
		return
	}
	dir, err := os.MkdirTemp("", "atria-export-*")
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to export the manuscript", err)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "book."+format)
	if format == "epub" {
		_, err = manuscripts.WriteEPUB(m, path)
	} else {
		err = manuscripts.Convert(c.Request.Context(), m, format, path)
	}
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to export the manuscript", err)
		return
	}
	c.FileAttachment(path, m.Slug+"."+format)
}

// handleManuscriptPublish makes an e-book from the manuscript and opens it.
func (s *Server) handleManuscriptPublish(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	m, ok := s.loadManuscript(c, user)
	if !ok {
		return
	}
	book, err := manuscripts.Publish(c.Request.Context(), s.db, user.ID, m)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to publish the manuscript", err)
		return
	}
	s.setFlash(c, "success", "E-book created from the manuscript.")
	s.redirectTo(c, "/ebooks/"+book.ID.String())
}
