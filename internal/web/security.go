package web

import (
	"atria/internal/core"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "atria_session"
	sessionTTL        = 30 * 24 * time.Hour

	// devSessionSecret is used only when ATRIA_ENV is development or test and
	// SESSION_SECRET is unset. It is never used in any other environment.
	devSessionSecret = "atria-insecure-development-session-secret"

	// exampleSessionSecret is the placeholder shipped in .env.example.
	exampleSessionSecret = "super-secret-dev-key-change-me"

	defaultMaxUploadMB = 100
	maxLoginBodyBytes  = 64 << 10
)

// contentSecurityPolicy limits where scripts, styles and other resources may be
// loaded from. Scripts may only come from our own /static files and the exact
// CDN paths of the rendering libraries: no inline scripts, no on* handlers, no
// javascript: URLs and no eval (htmx runs with allowEval=false). Page
// behaviour therefore lives in static/app.js and is wired up via data-*
// attributes. Inline styles stay allowed; they cannot execute code.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' https://cdn.jsdelivr.net/npm/mermaid@10/ https://cdn.jsdelivr.net/npm/mathjax@3/ https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/; " +
	"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; " +
	"img-src * data: blob:; " +
	"media-src *; " +
	"font-src 'self' data: https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; " +
	"connect-src 'self' https://cdn.jsdelivr.net; " +
	"worker-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

func isDevelopmentEnv() bool {
	env := os.Getenv("ATRIA_ENV")
	return env == "development" || env == "test"
}

// sessionSecret returns the key used to sign session cookies and CSRF tokens.
// It returns nil outside development/test when SESSION_SECRET is unset, which
// makes every signature check fail instead of falling back to a known key.
func sessionSecret() []byte {
	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		return []byte(secret)
	}
	if isDevelopmentEnv() {
		return []byte(devSessionSecret)
	}
	return nil
}

// ValidateSecurityConfig must be called before serving requests. It refuses to
// run outside development/test without a real SESSION_SECRET, because a known
// secret lets anyone forge a session for any user.
func ValidateSecurityConfig() error {
	if isDevelopmentEnv() {
		return nil
	}
	secret := os.Getenv("SESSION_SECRET")
	switch {
	case secret == "":
		return errors.New("SESSION_SECRET must be set when ATRIA_ENV is not development (generate one with: openssl rand -hex 32)")
	case secret == exampleSessionSecret:
		return errors.New("SESSION_SECRET still has the placeholder value from .env.example; generate one with: openssl rand -hex 32")
	case len(secret) < 32:
		log.Printf("WARNING: SESSION_SECRET is shorter than 32 characters; use: openssl rand -hex 32")
	}
	return nil
}

func hmacHex(secret []byte, parts ...string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(m.Sum(nil))
}

// signSession binds the cookie to the email, its expiry and the user's current
// password hash, so changing the password invalidates all existing sessions.
func signSession(secret []byte, email string, expires int64, passwordHash string) string {
	return hmacHex(secret, "session:v1", email, strconv.FormatInt(expires, 10), passwordHash)
}

func (s *Server) setSessionCookie(c *gin.Context, user *core.User) {
	secret := sessionSecret()
	if secret == nil {
		return
	}
	expires := time.Now().Add(sessionTTL).Unix()
	value := strings.Join([]string{
		"v1",
		base64.RawURLEncoding.EncodeToString([]byte(user.Email)),
		strconv.FormatInt(expires, 10),
		signSession(secret, user.Email, expires, user.PasswordHash),
	}, ".")

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureContext(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureContext(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// userFromSessionCookie returns the user identified by a valid, unexpired
// session cookie, or nil.
func (s *Server) userFromSessionCookie(c *gin.Context) *core.User {
	secret := sessionSecret()
	if secret == nil {
		return nil
	}
	cookie, err := c.Request.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return nil
	}
	emailBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	expires, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return nil
	}

	user, err := core.FindUser(c.Request.Context(), s.db, string(emailBytes))
	if err != nil {
		return nil
	}
	expected := signSession(secret, user.Email, expires, user.PasswordHash)
	if !hmac.Equal([]byte(parts[3]), []byte(expected)) {
		return nil
	}
	return user
}

func proxyAuthHeader() string {
	if name := os.Getenv("PROXY_AUTH_HEADER"); name != "" {
		return name
	}
	return "Remote-Email"
}

// resolveRequestUser identifies the caller from trusted proxy headers, the
// session cookie or, in development only, ATRIA_USER. identity is the email
// that was presented; it is non-empty with a nil user when that email is not
// registered in Atria.
func (s *Server) resolveRequestUser(c *gin.Context) (user *core.User, source core.AuthSource, identity string) {
	ctx := c.Request.Context()

	if email := c.GetHeader(proxyAuthHeader()); email != "" && isProxyAllowed(c) {
		user, _ := core.FindUser(ctx, s.db, email)
		return user, core.AuthSourceProxy, email
	}

	if user := s.userFromSessionCookie(c); user != nil {
		return user, core.AuthSourceLocal, user.Email
	}

	if os.Getenv("ATRIA_ENV") == "development" {
		if email := os.Getenv("ATRIA_USER"); email != "" {
			user, _ := core.FindUser(ctx, s.db, email)
			return user, core.AuthSourceLocal, email
		}
	}

	return nil, "", ""
}

func csrfTokenFor(email string) string {
	secret := sessionSecret()
	if secret == nil {
		return ""
	}
	return hmacHex(secret, "csrf", email)
}

// GetCSRFToken derives a per-user CSRF token from the authenticated user in
// context: HMAC-SHA256(SESSION_SECRET, "csrf"+email). It is tied to the user
// without requiring a separate DB lookup.
func (s *Server) GetCSRFToken(c *gin.Context) string {
	user := s.getUser(c)
	if user == nil {
		return ""
	}
	return csrfTokenFor(user.Email)
}

func (s *Server) CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		// Use the already-authenticated user from context (set by AuthMiddleware).
		// This works for all auth sources: proxy, session cookie, and dev fallback.
		user := s.getUser(c)
		expected := ""
		if user != nil {
			expected = csrfTokenFor(user.Email)
		}

		submitted := c.GetHeader("X-CSRF-Token")
		if submitted == "" {
			submitted = c.PostForm("_csrf")
		}

		if expected == "" || !hmac.Equal([]byte(submitted), []byte(expected)) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Invalid CSRF token"})
			return
		}

		c.Next()
	}
}

// SecurityHeadersMiddleware sets defensive headers on every response.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		c.Next()
	}
}

func maxUploadBytes() int64 {
	if s := os.Getenv("MAX_UPLOAD_MB"); s != "" {
		if mb, err := strconv.ParseInt(s, 10, 64); err == nil && mb > 0 {
			return mb << 20
		}
	}
	return defaultMaxUploadMB << 20
}

// BodyLimitMiddleware caps request bodies so a single upload cannot exhaust
// memory or disk.
func BodyLimitMiddleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

// localRedirectTarget returns the path (and query) of the Referer so handlers
// can send the user back where they came from without ever redirecting to a
// foreign host.
func localRedirectTarget(c *gin.Context, fallback string) string {
	ref, err := url.Parse(c.Request.Referer())
	if err != nil || ref.Path == "" || !strings.HasPrefix(ref.Path, "/") || strings.HasPrefix(ref.Path, "//") {
		return fallback
	}
	target := ref.EscapedPath()
	if ref.RawQuery != "" {
		target += "?" + ref.RawQuery
	}
	return target
}

// loginLimiter throttles failed password logins per client IP.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailures
	max      int
	window   time.Duration
}

type loginFailures struct {
	count int
	since time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{failures: map[string]*loginFailures{}, max: max, window: window}
}

func (l *loginLimiter) allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.failures[key]
	if !ok {
		return true
	}
	if time.Since(f.since) > l.window {
		delete(l.failures, key)
		return true
	}
	return f.count < l.max
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.failures) > 10000 {
		for k, f := range l.failures {
			if now.Sub(f.since) > l.window {
				delete(l.failures, k)
			}
		}
	}
	f, ok := l.failures[key]
	if !ok || now.Sub(f.since) > l.window {
		l.failures[key] = &loginFailures{count: 1, since: now}
		return
	}
	f.count++
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// compareDummyPassword spends the same time as a real bcrypt check so login
// response times do not reveal whether an email is registered.
func compareDummyPassword(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("atria-dummy-password"), bcrypt.DefaultCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// isCrossSiteRequest reports whether the browser marked the request as coming
// from another site (Fetch Metadata), which blocks login CSRF.
func isCrossSiteRequest(c *gin.Context) bool {
	return c.GetHeader("Sec-Fetch-Site") == "cross-site"
}

func contentDispositionAttachment(filename string) string {
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s",
		strings.Map(func(r rune) rune {
			if r < 0x20 || r == '"' || r == '\\' || r > 0x7e {
				return '_'
			}
			return r
		}, filename),
		url.PathEscape(filename))
}
