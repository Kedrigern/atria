package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/users"
	"database/sql"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
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

// AuthMiddleware solves identity pass by header from Authelia
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		headerName := os.Getenv("PROXY_AUTH_HEADER")
		if headerName == "" {
			headerName = "Remote-Email"
		}

		email := c.GetHeader(headerName)
		groups := c.GetHeader("Remote-Groups")

		if email == "" && os.Getenv("ATRIA_ENV") == "development" {
			email = os.Getenv("ATRIA_USER")
		}

		if email == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Missing identity header"})
			return
		}

		user, err := core.FindUser(c.Request.Context(), s.db, email)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: User not registered in Atria"})
			return
		}

		newRole := core.RoleUser
		if strings.Contains(groups, "admins") {
			newRole = core.RoleAdmin
		}

		if user.Role != newRole {
			_ = users.UpdateUserRole(c.Request.Context(), s.db, user.Email, newRole)
			user.Role = newRole
		}

		c.Set("currentUser", user)
		c.Next()
	}
}

// getUser
func (s *Server) getUser(c *gin.Context) *core.User {
	val, exists := c.Get("currentUser")
	if !exists {
		return nil
	}
	return val.(*core.User)
}

func (s *Server) SetupRouter() *gin.Engine {
	r := gin.Default()

	trustedProxiesRaw := os.Getenv("TRUSTED_PROXIES")
	if trustedProxiesRaw != "" {
		proxies := strings.Split(trustedProxiesRaw, ",")
		r.SetTrustedProxies(proxies)
	} else {
		r.SetTrustedProxies(nil)
	}

	subFS, err := fs.Sub(StaticFS, "static")
	if err != nil {
		panic(err)
	}
	r.StaticFS("/static", http.FS(subFS))

	auth := r.Group("/")
	auth.Use(s.AuthMiddleware())

	// ==========================================
	// 1. BASIC PAGES
	// ==========================================
	auth.GET("/", s.handleHome)
	auth.GET("/tables", s.makeHandler("table_list.html", nil))
	auth.GET("/settings", s.makeHandler("settings.html", nil))
	auth.GET("/profile", s.handleProfile)
	auth.GET("/attachments", s.handleAttachments)

	// RSS
	rss := auth.Group("/rss")
	{
		rss.GET("", s.handleRSS)
		rss.GET("/feeds", s.handleRSSFeeds)
	}

	// Read (Articles)
	read := auth.Group("/read")
	{
		read.GET("", s.handleRead)
		read.GET("/:id", s.handleReadDetail)
		read.GET("/:id/export/md", s.handleReadExportMD)
		read.GET("/:id/export/epub", s.handleReadExportEPUB)
	}

	// Notes
	notes := auth.Group("/notes")
	{
		notes.GET("", s.handleNotes)
		notes.GET("/new", s.handleNoteAdd)
		notes.GET("/:id", s.handleNoteDetail)
		notes.GET("/:id/export/md", s.handleNoteExportMD)
		notes.GET("/:id/export/epub", s.handleNoteExportEPUB)
	}

	// Tags
	tags := auth.Group("/tags")
	{
		tags.GET("", s.handleTags)
		tags.GET("/:name", s.handleTagDetail)
	}

	// ==========================================
	// 2. API ENDPOINTS (HTMX & Form submits)
	// ==========================================
	api := auth.Group("/api")
	{
		// Cross-Entity
		entity := api.Group("/entity/:id")
		{
			entity.POST("/tags", s.handleTagAttach)
			entity.POST("/attachments", s.handleEntityAttachmentUpload)
			entity.POST("/links", s.handleEntityLinkAdd)
		}

		// API: RSS
		apiRSS := api.Group("/rss")
		{
			apiRSS.POST("/add", s.handleRSSAdd)
			apiRSS.POST("/fetch", s.handleRSSFetch)
			apiRSS.POST("/archive-batch", s.handleRSSArchiveBatch)
			apiRSS.POST("/archive/:id", s.handleRSSArchive)
			apiRSS.POST("/save/:id", s.handleRSSSave)
		}

		// API: Read (Articles)
		apiRead := api.Group("/read")
		{
			apiRead.POST("/add", s.handleReadAdd)
			apiRead.POST("/archive/:id", s.handleReadArchive)
			apiRead.POST("/refetch/:id", s.handleReadRefetch)
			apiRead.POST("/note/:id", s.handleReadUpdateNote)
		}

		// API: Notes
		apiNotes := api.Group("/notes")
		{
			apiNotes.POST("/create", s.handleNoteCreate)
			apiNotes.POST("/update/:id", s.handleNoteUpdate)
			apiNotes.POST("/delete/:id", s.handleNoteDelete)
		}

		// API: Other
		api.POST("/tags/add", s.handleTagAdd)
		api.POST("/profile/preferences", s.handleProfilePreferences)
	}

	return r
}

// render wrap parsing HTMX and adding useful functions (eg formatDate)
func (s *Server) render(c *gin.Context, tmplName string, data gin.H) {
	var t *template.Template
	var err error

	if data == nil {
		data = gin.H{}
	}

	user := s.getUser(c)
	if user == nil {
		return
	}

	data["User"] = user
	data["Flash"] = s.getFlash(c)
	data["Theme"] = user.Preferences.Theme

	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("02.01.2006 15:04")
		},
		"stripHTML": func(s string) string {
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
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"divide": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}

	if c.GetHeader("HX-Request") == "true" {
		t = template.New(tmplName).Funcs(funcMap)
		t, err = t.ParseFS(TemplatesFS, "templates/partials.html", "templates/pages/"+tmplName)
		if err == nil {
			err = t.ExecuteTemplate(c.Writer, "content", data)
		}
	} else {
		t = template.New("base.html").Funcs(funcMap)
		t, err = t.ParseFS(TemplatesFS, "templates/base.html", "templates/partials.html", "templates/pages/"+tmplName)
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
	user := s.getUser(c)
	if user == nil {
		return
	}

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

func (s *Server) handleProfile(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	ctx := c.Request.Context()

	var noteCount, articleCount, tagCount int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notes WHERE owner_id = $1", user.ID).Scan(&noteCount)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM articles WHERE owner_id = $1", user.ID).Scan(&articleCount)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags WHERE owner_id = $1", user.ID).Scan(&tagCount)

	s.render(c, "profile.html", gin.H{
		"User":         user,
		"NoteCount":    noteCount,
		"ArticleCount": articleCount,
		"TagCount":     tagCount,
		"ProxyHeaders": gin.H{
			"Email":  c.GetHeader("Remote-Email"),
			"User":   c.GetHeader("Remote-User"),
			"Groups": c.GetHeader("Remote-Groups"),
		},
	})
}

func (s *Server) handleProfilePreferences(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		c.String(http.StatusUnauthorized, "Unauthorized")
		return
	}

	prefs := user.Preferences

	newTheme := c.PostForm("theme")
	if newTheme == "light" || newTheme == "dark" || newTheme == "system" {
		prefs.Theme = newTheme
	}

	if sizeStr := c.PostForm("pagination_size"); sizeStr != "" {
		if size, err := strconv.Atoi(sizeStr); err == nil && size >= 10 && size <= 100 {
			prefs.PaginationSize = size
		}
	}

	prefs.RSSInlineDetails = c.PostForm("rss_inline_details") == "on"

	err := users.UpdatePreferences(c.Request.Context(), s.db, user.ID, prefs)
	if err != nil {
		s.setFlash(c, "error", "Nepodařilo se uložit preference.")
	} else {
		s.setFlash(c, "success", "Preference uloženy.")
	}

	c.Redirect(http.StatusFound, "/profile")
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
