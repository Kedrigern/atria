package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"github.com/russross/blackfriday/v2"
)

//go:embed templates/*
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

type Server struct {
	db *sql.DB
}

type FlashMessage struct {
	Type    string
	Message string
}

func NewServer(db *sql.DB) *Server {
	return &Server{db: db}
}

// getDummyUser first user form DB
// Temporaly solution before we implement auth in web
func (s *Server) getDummyUser(c *gin.Context) *core.User {
	var u core.User
	err := s.db.QueryRowContext(c.Request.Context(), "SELECT id, email, role FROM users LIMIT 1").Scan(&u.ID, &u.Email, &u.Role)
	if err != nil {
		// Fallback for empty DB
		u.ID = core.NewUUID()
	}
	return &u
}

func (s *Server) SetupRouter() *gin.Engine {
	r := gin.Default()

	r.StaticFS("/static", http.FS(StaticFS))

	r.GET("/", s.handleHome)
	r.GET("/rss", s.handleRSS)
	r.GET("/rss/feeds", s.handleRSSFeeds)
	r.POST("/api/rss/archive-batch", s.handleRSSArchiveBatch)
	r.POST("/api/rss/archive/:id", s.handleRSSArchive)
	r.GET("/read", s.handleRead)
	r.GET("/read/:id", s.handleReadDetail)
	r.POST("/api/read/archive/:id", s.handleReadArchive)
	r.POST("/api/rss/add", s.handleRSSAdd)
	r.POST("/api/rss/fetch", s.handleRSSFetch)
	r.GET("/notes", s.handleNotes)
	r.GET("/notes/:id", s.handleNoteDetail)

	// API
	r.POST("/api/rss/save/:id", s.handleRSSSave)

	// Static mocks for WIP pages
	r.GET("/tables", s.makeHandler("table_list.html", nil))
	r.GET("/settings", s.makeHandler("settings.html", nil))
	r.GET("/profile", s.makeHandler("profile.html", nil))

	return r
}

// render wrap parsing HTMX and adding useful functions (eg formatDate)
func (s *Server) render(c *gin.Context, tmplName string, data gin.H) {
	var t *template.Template
	var err error

	if data == nil {
		data = gin.H{}
	}
	data["Flash"] = s.getFlash(c)

	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("02.01.2006 15:04")
		},
		"stripHTML": func(s string) string {
			// Delete all html tags for clean perex
			re := regexp.MustCompile(`<[^>]*>`)
			return re.ReplaceAllString(s, "")
		},
		"truncate": func(s string, l int) string {
			runes := []rune(s)
			if len(runes) > l {
				return string(runes[:l]) + "..."
			}
			return s
		},
	}

	if c.GetHeader("HX-Request") == "true" {
		t = template.New(tmplName).Funcs(funcMap)
		t, err = t.ParseFS(TemplatesFS, "templates/pages/"+tmplName)
		if err == nil {
			err = t.ExecuteTemplate(c.Writer, "content", data)
		}
	} else {
		t = template.New("base.html").Funcs(funcMap)
		t, err = t.ParseFS(TemplatesFS, "templates/base.html", "templates/pages/"+tmplName)
		if err == nil {
			err = t.ExecuteTemplate(c.Writer, "base.html", data)
		}
	}

	if err != nil {
		c.String(http.StatusInternalServerError, "Template error: %v", err)
	}
}

func (s *Server) setFlash(c *gin.Context, flashType, message string) {
	escapedMessage := url.QueryEscape(message)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "atria_flash",
		Value:    flashType + "|" + escapedMessage,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   10,
	})
}

func (s *Server) getFlash(c *gin.Context) *FlashMessage {
	cookie, err := c.Request.Cookie("atria_flash")
	if err != nil || cookie.Value == "" {
		return nil
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "atria_flash",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	parts := regexp.MustCompile(`\|`).Split(cookie.Value, 2)
	if len(parts) != 2 {
		return nil
	}

	message, err := url.QueryUnescape(parts[1])
	if err != nil {
		message = parts[1]
	}

	return &FlashMessage{
		Type:    parts[0],
		Message: message,
	}
}

func (s *Server) renderError(c *gin.Context, status int, message string) {
	if c.GetHeader("HX-Request") == "true" {
		c.String(status, message)
		return
	}
	c.String(status, message)
}

func (s *Server) handleHome(c *gin.Context) {
	user := s.getDummyUser(c)
	ctx := c.Request.Context()

	rssItems, err := rss.ListItemsToRead(ctx, s.db, user.ID, 100, 0)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load RSS items: "+err.Error())
		return
	}

	articlesList, err := articles.ListArticles(ctx, s.db, user.ID, 100, 0)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load articles: "+err.Error())
		return
	}

	notesList, err := notes.ListNotes(ctx, s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load notes: "+err.Error())
		return
	}

	if len(notesList) > 5 {
		notesList = notesList[:5]
	}

	s.render(c, "home.html", gin.H{
		"UnreadCount": len(rssItems),
		"InboxCount":  len(articlesList),
		"RecentNotes": notesList,
	})
}

func (s *Server) handleRead(c *gin.Context) {
	user := s.getDummyUser(c)

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 30
	offset := (page - 1) * limit

	list, err := articles.ListArticles(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load articles: "+err.Error())
		return
	}

	hasNext := false
	if len(list) > limit {
		hasNext = true
		list = list[:limit]
	}

	s.render(c, "read_list.html", gin.H{
		"Articles": list,
		"Page":     page,
		"HasNext":  hasNext,
		"NextPage": page + 1,
		"PrevPage": page - 1,
	})
}

func (s *Server) handleNotes(c *gin.Context) {
	user := s.getDummyUser(c)
	list, err := notes.ListNotes(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load notes: "+err.Error())
		return
	}
	s.render(c, "note_list.html", gin.H{
		"Notes": list,
	})
}

func (s *Server) handleReadDetail(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, domain string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, domain FROM articles_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &domain)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Article not found")
		return
	}

	htmlContent, err := articles.GetArticleHTML(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load article content: "+err.Error())
		return
	}

	s.render(c, "read_detail.html", gin.H{
		"Title":   title,
		"Domain":  domain,
		"Content": template.HTML(htmlContent),
	})
}

func (s *Server) handleNoteDetail(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var title, path string
	err = s.db.QueryRowContext(c.Request.Context(), "SELECT title, path FROM notes_full_view WHERE id = $1 AND owner_id = $2", id, user.ID).Scan(&title, &path)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Note not found")
		return
	}

	mdContent, err := notes.GetNoteContent(c.Request.Context(), s.db, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load note content: "+err.Error())
		return
	}
	htmlContent := blackfriday.Run([]byte(mdContent))

	s.render(c, "note_detail.html", gin.H{
		"Title":   title,
		"Path":    path,
		"Content": template.HTML(htmlContent),
	})
}

func (s *Server) handleReadArchive(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = articles.ArchiveArticle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to archive article: "+err.Error())
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}

	limit := 30
	offset := (page - 1) * limit
	list, err := articles.ListArticles(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to refresh article list: "+err.Error())
		return
	}
	if len(list) == 0 && page > 1 {
		page--
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<span style="color: var(--text-muted); font-size: 0.9rem;">✓ Archived</span>`))
		return
	}

	s.setFlash(c, "success", "Article archived.")
	c.Redirect(http.StatusSeeOther, "/read?page="+strconv.Itoa(page))
}

func (s *Server) handleRSSSave(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	_, err = rss.SaveItemAsArticle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to save: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// HTMX magic: Instead of entire HTML return just piece of code
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<span style=\"color: #10b981; font-weight: bold;\">✅ Saved to Inbox</span>`))
		return
	}

	s.setFlash(c, "success", "RSS item saved to Inbox.")
	c.Redirect(http.StatusSeeOther, "/rss")
}

func (s *Server) makeHandler(tmplName string, data gin.H) gin.HandlerFunc {
	return func(c *gin.Context) {
		renderData := gin.H{}
		for k, v := range data {
			renderData[k] = v
		}
		s.render(c, tmplName, renderData)
	}
}

func (s *Server) handleRSSAdd(c *gin.Context) {
	user := s.getDummyUser(c)
	title := c.PostForm("title")
	urlStr := c.PostForm("url")

	if urlStr == "" {
		s.renderError(c, http.StatusBadRequest, "URL is required")
		return
	}
	if title == "" {
		title = urlStr // Fallback, pokud uživatel nevyplní název
	}

	_, err := rss.CreateFeed(c.Request.Context(), s.db, user.ID, title, urlStr)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to add feed: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// HX-Refresh triggers a full page reload in HTMX to show the updated list and clear the form
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Feed added.")
	c.Redirect(http.StatusSeeOther, "/rss/feeds")
}

func (s *Server) handleRSSFetch(c *gin.Context) {
	// Triggers the parallel worker pool to fetch all feeds
	err := rss.FetchAllActiveFeeds(c.Request.Context(), s.db)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Fetch failed: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// Reload the page to display newly fetched items
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Feeds fetched.")
	c.Redirect(http.StatusSeeOther, "/rss/feeds")
}

func (s *Server) handleRSSFeeds(c *gin.Context) {
	user := s.getDummyUser(c)
	feeds, err := rss.ListFeeds(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to list feeds: "+err.Error())
		return
	}
	s.render(c, "rss_feeds.html", gin.H{
		"Feeds": feeds,
	})
}

func (s *Server) handleRSSArchiveBatch(c *gin.Context) {
	user := s.getDummyUser(c)

	// HTMX will send the IDs as an array from the hidden inputs
	idStrs := c.PostFormArray("ids")
	var ids []uuid.UUID

	for _, idStr := range idStrs {
		if id, err := core.ParseUUID(idStr); err == nil {
			ids = append(ids, id)
		}
	}

	if len(ids) > 0 {
		err := rss.MarkBatchAsRead(c.Request.Context(), s.db, user.ID, ids)
		if err != nil {
			s.renderError(c, http.StatusInternalServerError, "Failed to archive batch: "+err.Error())
			return
		}
	}

	page := 1
	if p, err := strconv.Atoi(c.DefaultPostForm("page", "1")); err == nil && p > 0 {
		page = p
	}

	limit := 30
	offset := (page - 1) * limit

	items, err := rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to refresh RSS items: "+err.Error())
		return
	}

	if len(items) == 0 && page > 1 {
		page--
		offset = (page - 1) * limit
		items, err = rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
		if err != nil {
			s.renderError(c, http.StatusInternalServerError, "Failed to refresh RSS items: "+err.Error())
			return
		}
	}

	if c.GetHeader("HX-Request") != "true" {
		s.setFlash(c, "success", "Page archived.")
		c.Redirect(http.StatusSeeOther, "/rss?page="+strconv.Itoa(page))
		return
	}

	hasNext := false
	if len(items) > limit {
		hasNext = true
		items = items[:limit]
	}

	s.render(c, "rss.html", gin.H{
		"Items":    items,
		"Page":     page,
		"HasNext":  hasNext,
		"NextPage": page + 1,
		"PrevPage": page - 1,
	})
}
func (s *Server) handleRSS(c *gin.Context) {
	user := s.getDummyUser(c)

	// Paginace: defaultně stránka 1
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 30 // Zobrazíme 30 položek na stránku
	offset := (page - 1) * limit

	// Trik pro paginaci: řekneme si o limit + 1. Pokud se jich tolik vrátí, víme, že existuje další stránka!
	items, err := rss.ListItemsToRead(c.Request.Context(), s.db, user.ID, limit+1, offset)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load RSS items: "+err.Error())
		return
	}

	hasNext := false
	if len(items) > limit {
		hasNext = true
		items = items[:limit] // Odřízneme ten jeden navíc
	}

	s.render(c, "rss.html", gin.H{
		"Items":    items,
		"Page":     page,
		"HasNext":  hasNext,
		"NextPage": page + 1,
		"PrevPage": page - 1,
	})
}

func (s *Server) handleRSSArchive(c *gin.Context) {
	user := s.getDummyUser(c)
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	err = rss.MarkAsRead(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to archive RSS item: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// Vrátíme ikonu odškrtnutí
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<span style=\"color: var(--text-muted); font-size: 0.9rem;\">✓ Archived</span>`))
		return
	}

	s.setFlash(c, "success", "RSS item archived.")
	c.Redirect(http.StatusSeeOther, "/rss")
}
