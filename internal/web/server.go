package web

import (
	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/search"
	"atria/internal/users"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*
var TemplatesFS embed.FS

//go:embed static/*
var StaticFS embed.FS

type Server struct {
	db           *sql.DB
	loginLimiter *loginLimiter
}

type FlashMessage struct {
	Type     string
	Message  string
	LinkURL  string
	LinkText string
}

func isSecureContext() bool {
	return os.Getenv("ATRIA_ENV") != "development"
}

func debugLog(format string, args ...any) {
	if os.Getenv("DEBUG") == "true" {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func NewServer(db *sql.DB) *Server {
	return &Server{db: db, loginLimiter: newLoginLimiter(10, 15*time.Minute)}
}

// isProxyAllowed checks whether the direct TCP connection (RemoteAddr) is in the PROXY_ALLOWLIST.
// We intentionally use RemoteAddr instead of c.ClientIP() because ClientIP() unwraps
// X-Forwarded-For — giving us the end-user's IP, not the proxy's IP.
func isProxyAllowed(c *gin.Context) bool {
	allowlistRaw := os.Getenv("PROXY_ALLOWLIST")
	if allowlistRaw == "" {
		debugLog("isProxyAllowed: PROXY_ALLOWLIST not set, rejecting proxy auth")
		return false
	}
	// Strip the port from host:port
	remoteIP, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		remoteIP = c.Request.RemoteAddr
	}
	debugLog("isProxyAllowed: remoteIP=%q allowlist=%q", remoteIP, allowlistRaw)
	for _, allowed := range strings.Split(allowlistRaw, ",") {
		if strings.TrimSpace(allowed) == remoteIP {
			return true
		}
	}
	debugLog("isProxyAllowed: remoteIP %q not in allowlist, rejecting proxy auth", remoteIP)
	return false
}

// hasExactGroup checks whether groupHeader (a comma-separated list of group names)
// contains the given group as an exact match.
func hasExactGroup(groupHeader, group string) bool {
	for _, g := range strings.Split(groupHeader, ",") {
		if strings.TrimSpace(g) == group {
			return true
		}
	}
	return false
}

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Proxy headers are trusted only from allowlisted IPs; see resolveRequestUser.
		user, authSource, identity := s.resolveRequestUser(c)

		if identity == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		if user == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: User not registered in Atria"})
			return
		}

		user.AuthSource = authSource

		if authSource == core.AuthSourceProxy {
			groups := c.GetHeader("Remote-Groups")
			newRole := core.RoleUser
			if hasExactGroup(groups, "admins") {
				newRole = core.RoleAdmin
			}

			if user.Role != newRole {
				if err := users.UpdateUserRole(c.Request.Context(), s.db, user.Email, newRole); err != nil {
					log.Printf("ERROR: failed to update role for user %s: %v", user.Email, err)
				}
				user.Role = newRole
			}
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
	r.Use(SecurityHeadersMiddleware())
	r.Use(BodyLimitMiddleware(maxUploadBytes()))

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
	r.GET("/manifest.webmanifest", s.handleWebAppManifest)
	r.GET("/sw.js", s.handleServiceWorker)

	r.GET("/data/attachments/:year/:month/:filename", s.handleProtectedAttachment)
	r.GET("/public/articles/:id", s.handlePublicArticle)
	r.GET("/public/notes/:id", s.handlePublicNote)

	auth := r.Group("/")
	auth.Use(s.AuthMiddleware())
	auth.Use(s.CSRFMiddleware())

	// ==========================================
	// 1. BASIC PAGES
	// ==========================================
	auth.GET("/", s.handleHome)
	auth.GET("/tables", s.makeHandler("table_list.html", nil))
	auth.GET("/settings", s.makeHandler("settings.html", nil))
	auth.GET("/settings/users", s.handleUserList)
	auth.GET("/settings/rss", s.handleRSSFeedList)
	auth.GET("/settings/attachments", s.handleAttachments)
	auth.GET("/profile", s.handleProfile)
	auth.GET("/search", s.handleSearch)

	// Login / logout are outside the auth+CSRF group:
	// - /login has no authenticated user yet, so CSRF middleware cannot derive a token
	r.GET("/login", s.handleLoginGet)
	r.POST("/login", s.handleLoginPost)
	// Logout stays in the auth group so AuthMiddleware populates the user context
	// (needed for the Authelia SSO logout redirect).
	auth.GET("/logout", s.handleLogout)

	// RSS
	rss := auth.Group("/rss")
	{
		rss.GET("", s.handleRSSItemList)
		rss.GET("/feeds", s.handleRSSFeedList)
		rss.GET("/:id", s.handleRSSFeedDetail)
	}

	// Read (Articles)
	read := auth.Group("/read")
	{
		read.GET("", s.handleArticleList)
		read.GET("/:id", s.handleArticleDetail)
		read.GET("/:id/export/md", s.handleArticleExportMD)
		read.GET("/:id/export/epub", s.handleArticleExportEPUB)
	}

	// Notes
	notes := auth.Group("/notes")
	{
		notes.GET("", s.handleNoteList)
		notes.GET("/new", s.handleNoteAdd)
		notes.GET("/:id", s.handleNoteDetail)
		notes.GET("/:id/export/md", s.handleNoteExportMD)
		notes.GET("/:id/export/epub", s.handleNoteExportEPUB)
	}

	// Games
	games := auth.Group("/games")
	{
		games.GET("", s.handleGameList)
		games.GET("/cryptograms/:id", s.handleCryptogramPlay)
	}

	// E-books
	ebooks := auth.Group("/ebooks")
	{
		ebooks.GET("", s.handleEbookList)
		ebooks.GET("/new", s.handleEbookNew)
		ebooks.GET("/import", s.handleEbookImportForm)
		ebooks.GET("/:id", s.handleEbookDetail)
		ebooks.GET("/:id/edit", s.handleEbookEdit)
		ebooks.GET("/:id/download", s.handleEbookDownload)
	}

	// Tag details are content navigation, not settings.
	auth.GET("/tags/:name", s.handleTagDetail)

	// Tag management settings.
	tags := auth.Group("/settings/tags")
	{
		tags.GET("", s.handleTagList)
		tags.GET("/:name", func(c *gin.Context) {
			c.Redirect(http.StatusPermanentRedirect, "/tags/"+c.Param("name"))
		})
	}

	// ==========================================
	// 2. API ENDPOINTS (HTMX & Form submits)
	// ==========================================
	api := auth.Group("/api")
	{
		// Cross-Entity
		entity := api.Group("/entity/:id")
		{
			entity.PUT("/visibility", s.handleUpdateVisibility)
			entity.POST("/tags", s.handleTagAttach)
			entity.POST("/attachments", s.handleEntityAttachmentUpload)
			entity.POST("/links", s.handleEntityLinkAdd)
			entity.POST("/rename", s.handleEntityRename)
		}

		// API: RSS
		apiRSS := api.Group("/rss")
		{
			apiRSS.POST("/add", s.handleRSSFeedAdd)
			apiRSS.POST("/fetch", s.handleRSSFetchAll)
			apiRSS.POST("/:id/fetch", s.handleRSSFeedFetch)
			apiRSS.POST("/:id/update", s.handleRSSFeedUpdate)
			apiRSS.POST("/:id/delete", s.handleRSSFeedDelete)
			apiRSS.POST("/:id/archive", s.handleRSSItemArchive)
			apiRSS.POST("/:id/archive-all", s.handleRSSFeedArchiveAll)
			apiRSS.POST("/archive-batch", s.handleRSSItemArchiveBatch)
		}

		// API: Articles
		apiArticles := api.Group("/articles")
		{
			apiArticles.POST("/add", s.handleArticleAdd)
			apiArticles.POST("/:id/archive", s.handleArticleArchive)
			apiArticles.POST("/:id/delete", s.handleArticleDelete)
			apiArticles.POST("/:id/refetch", s.handleArticleRefetch)
			apiArticles.POST("/:id/note", s.handleArticleNoteUpdate)
		}

		// API: Games
		apiGames := api.Group("/games/cryptograms/:id")
		{
			apiGames.POST("/guesses", s.handleCryptogramGuess)
			apiGames.POST("/hint", s.handleCryptogramHint)
			apiGames.POST("/reset", s.handleCryptogramReset)
		}

		// API: E-books
		apiEbooks := api.Group("/ebooks")
		{
			apiEbooks.POST("/create", s.handleEbookCreate)
			apiEbooks.POST("/import", s.handleEbookImport)
			apiEbooks.POST("/:id/update", s.handleEbookUpdate)
			apiEbooks.POST("/:id/delete", s.handleEbookDelete)
		}

		// API: Notes
		apiNotes := api.Group("/notes")
		{
			apiNotes.POST("/create", s.handleNoteCreate)
			apiNotes.POST("/:id/update", s.handleNoteUpdate)
			apiNotes.POST("/:id/delete", s.handleNoteDelete)
		}

		// API: Tags
		apiTags := api.Group("/tags")
		{
			apiTags.POST("/create", s.handleTagCreate)
			apiTags.POST("/attach", s.handleTagAttachUniversal)
			apiTags.GET("/options", s.handleTagOptions)
		}

		// API: Attachments
		apiAttachments := api.Group("/attachments")
		{
			apiAttachments.POST("/:id/rename", s.handleAttachmentRename)
			apiAttachments.POST("/:id/delete", s.handleAttachmentDelete)
		}

		// API: Users
		apiUsers := api.Group("/users")
		{
			apiUsers.POST("/create", s.handleUserCreate)
			apiUsers.POST("/role", s.handleUserRoleUpdate)
			apiUsers.POST("/delete", s.handleUserDelete)
		}

		// API: Profile
		api.POST("/profile/preferences", s.handleProfilePreferencesUpdate)
		api.POST("/profile/font-size", s.handleFontSizeAdjust)
	}

	return r
}

func (s *Server) getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("02.01.2006 15:04")
		},
		"formatDateOnly": func(t time.Time) string {
			return t.Format("02.01.2006")
		},
		"formatDatePtr": func(t *time.Time) string {
			if t == nil {
				return "Never"
			}
			return t.Format("02.01.2006 15:04")
		},
		"stripHTML": func(s string) string {
			re := regexp.MustCompile(`<[^>]*>`)
			stripped := re.ReplaceAllString(s, "")

			return html.UnescapeString(stripped)
		},
		"truncate": func(s string, l int) string {
			runes := []rune(s)
			if len(runes) > l {
				return string(runes[:l]) + "..."
			}
			return s
		},
		// sanitizeHTML renders untrusted third-party HTML (e.g. RSS content)
		// through the allowlist sanitizer. There is deliberately no raw
		// "safeHTML" helper: trusted HTML must be produced in Go code.
		"sanitizeHTML": func(s string) template.HTML {
			return template.HTML(core.SanitizeHTML(s))
		},
		"searchHeadline": search.HeadlineHTML,

		"normoPages": func(charCount int) string {
			// 1 normopage = 1800 characters (Czech standard ČSN).
			if charCount <= 0 {
				return ""
			}
			pages := float64(charCount) / 1800.0
			if pages < 0.1 {
				return ""
			}
			if pages < 1.0 {
				return fmt.Sprintf("%.1f NS", pages)
			}
			return fmt.Sprintf("%.0f NS", pages)
		},
		"divide": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"isImage": func(mimeType string) bool {
			return strings.HasPrefix(mimeType, "image/")
		},
	}
}

func (s *Server) renderSnippet(c *gin.Context, snippetName string, data any) {
	c.Header("Content-Type", "text/html; charset=utf-8")

	t := template.New("").Funcs(s.getTemplateFuncs())
	t, err := t.ParseFS(TemplatesFS, "templates/partials.html")
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to render page", err)
		return
	}

	if err := t.ExecuteTemplate(c.Writer, snippetName, data); err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to render page", err)
	}
}

// render wrap parsing HTMX and adding useful functions (eg formatDate)
func (s *Server) render(c *gin.Context, tmplName string, data gin.H) {
	var t *template.Template
	var err error

	if data == nil {
		data = gin.H{}
	}

	user := s.getUser(c)
	if user == nil && tmplName != "login.html" && tmplName != "public_detail.html" {
		return
	}

	if user != nil {
		data["User"] = user
		data["Theme"] = user.Preferences.Theme

		fontSize := user.Preferences.FontSize
		if fontSize == 0 {
			fontSize = 100
		}
		data["FontSize"] = fontSize
		// ContentFontScale is a multiplier (e.g. "1.30") applied via CSS calc()
		// only to the article/note title and body, so the rest of the UI
		// (toolbar, sidebar, nav) keeps its fixed sizing.
		data["ContentFontScale"] = strconv.FormatFloat(float64(fontSize)/100, 'f', 2, 64)
	}

	data["Flash"] = s.getFlash(c)
	data["CSRFToken"] = s.GetCSRFToken(c)

	funcMap := s.getTemplateFuncs()

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
		s.respondError(c, http.StatusInternalServerError, "Failed to render page", err)
	}
}

func (s *Server) setFlash(c *gin.Context, flashType, message string) {
	s.setFlashLink(c, flashType, message, "", "")
}

// setFlashLink stores a flash message along with an optional link (e.g. to
// point the user at an existing entity that caused a conflict).
func (s *Server) setFlashLink(c *gin.Context, flashType, message, linkURL, linkText string) {
	value := flashType + "|" + url.QueryEscape(message) + "|" + url.QueryEscape(linkURL) + "|" + url.QueryEscape(linkText)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "atria_flash",
		Value:    value,
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

	parts := strings.SplitN(cookie.Value, "|", 4)
	if len(parts) < 2 {
		return nil
	}

	message, err := url.QueryUnescape(parts[1])
	if err != nil {
		message = parts[1]
	}

	flash := &FlashMessage{
		Type:    parts[0],
		Message: message,
	}

	if len(parts) == 4 {
		if linkURL, err := url.QueryUnescape(parts[2]); err == nil {
			flash.LinkURL = linkURL
		}
		if linkText, err := url.QueryUnescape(parts[3]); err == nil {
			flash.LinkText = linkText
		}
	}

	return flash
}

// renderError responds with a plain-text message. Callers with an underlying
// error should use respondError, which logs it and keeps details private.
func (s *Server) renderError(c *gin.Context, status int, message string) {
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
		s.respondError(c, http.StatusInternalServerError, "Failed to load RSS items", err)
		return
	}

	articlesList, err := articles.ListArticles(ctx, s.db, user.ID, 100, 0)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load articles", err)
		return
	}

	notesList, err := notes.ListNotes(ctx, s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load notes", err)
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
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE owner_id = $1`, user.ID).Scan(&noteCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles WHERE owner_id = $1`, user.ID).Scan(&articleCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE owner_id = $1`, user.ID).Scan(&tagCount)

	s.render(c, "profile.html", gin.H{
		"User":             user,
		"SSOManagementURL": os.Getenv("SSO_MANAGEMENT_URL"),
		"NoteCount":        noteCount,
		"ArticleCount":     articleCount,
		"TagCount":         tagCount,
		"ProxyHeaders": gin.H{
			"Email":  c.GetHeader("Remote-Email"),
			"User":   c.GetHeader("Remote-User"),
			"Groups": c.GetHeader("Remote-Groups"),
		},
	})
}

func (s *Server) handleProfilePreferencesUpdate(c *gin.Context) {
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

	if fontSizeStr := c.PostForm("font_size"); fontSizeStr != "" {
		if fontSize, err := strconv.Atoi(fontSizeStr); err == nil && fontSize >= 80 && fontSize <= 160 {
			prefs.FontSize = fontSize
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

// handleFontSizeAdjust nudges the user's font-size preference up or down by
// one step (10%), clamped to the 80-160 range, and redirects back to the
// page the request came from. Intended for the quick +/- toolbar buttons.
func (s *Server) handleFontSizeAdjust(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		c.String(http.StatusUnauthorized, "Unauthorized")
		return
	}

	prefs := user.Preferences

	fontSize := prefs.FontSize
	if fontSize == 0 {
		fontSize = 100
	}

	delta, _ := strconv.Atoi(c.PostForm("delta"))
	fontSize += delta * 10
	if fontSize < 80 {
		fontSize = 80
	}
	if fontSize > 160 {
		fontSize = 160
	}
	prefs.FontSize = fontSize

	if err := users.UpdatePreferences(c.Request.Context(), s.db, user.ID, prefs); err != nil {
		s.setFlash(c, "error", "Nepodařilo se uložit velikost písma.")
	}

	c.Redirect(http.StatusFound, localRedirectTarget(c, "/"))
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

func (s *Server) handleLoginGet(c *gin.Context) {
	if user := s.getUser(c); user != nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	s.render(c, "login.html", nil)
}

func (s *Server) handleLoginPost(c *gin.Context) {
	if isCrossSiteRequest(c) {
		c.String(http.StatusForbidden, "Cross-site login is not allowed")
		return
	}

	limiterKey := c.ClientIP()
	if !s.loginLimiter.allowed(limiterKey) {
		s.setFlash(c, "error", "Příliš mnoho neúspěšných pokusů. Zkuste to znovu později.")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxLoginBodyBytes)
	email := c.PostForm("email")
	password := c.PostForm("password")

	user, err := core.FindUser(c.Request.Context(), s.db, email)
	if err == nil && user.PasswordHash != "" {
		err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	} else {
		compareDummyPassword(password)
		if err == nil {
			err = errors.New("user has no local password")
		}
	}
	if err != nil {
		s.loginLimiter.fail(limiterKey)
		s.setFlash(c, "error", "Neplatný e-mail nebo heslo.")
		c.Redirect(http.StatusFound, "/login")
		return
	}

	s.loginLimiter.reset(limiterKey)
	if !s.setSessionCookie(c, user) {
		s.setFlash(c, "error", "Přihlášení se nezdařilo, zkuste to znovu.")
		c.Redirect(http.StatusFound, "/login")
		return
	}
	c.Redirect(http.StatusFound, "/")
}

func (s *Server) handleLogout(c *gin.Context) {
	user := s.getUser(c)

	s.endSession(c)

	if user != nil && user.AuthSource == core.AuthSourceProxy {
		logoutURL := os.Getenv("SSO_LOGOUT_URL")
		if logoutURL != "" {
			c.Redirect(http.StatusFound, logoutURL)
			return
		}
	}

	c.Redirect(http.StatusFound, "/login")
}
