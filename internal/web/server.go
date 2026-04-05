package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"database/sql"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
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

	subFS, err := fs.Sub(StaticFS, "static")
	if err != nil {
		panic(err)
	}
	r.StaticFS("/static", http.FS(subFS))

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

func (s *Server) makeHandler(tmplName string, data gin.H) gin.HandlerFunc {
	return func(c *gin.Context) {
		renderData := gin.H{}
		for k, v := range data {
			renderData[k] = v
		}
		s.render(c, tmplName, renderData)
	}
}
