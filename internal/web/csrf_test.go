package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/users"
	"atria/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCSRFTest builds an HTTP request and runs it through both AuthMiddleware and CSRFMiddleware.
// env and atriaUser configure the dev-mode auth fallback.
// csrfToken is the value placed in the X-CSRF-Token header (empty string = omit header).
// method is the HTTP method (GET, POST, …).
// The router exposes:
//
//	POST /test  → 200 OK (guarded by CSRF)
//	GET  /token → plain-text CSRF token via srv.GetCSRFToken
func runCSRFTest(t *testing.T, srv *web.Server, env, atriaUser, csrfToken, method string) *httptest.ResponseRecorder {
	t.Helper()

	t.Setenv("ATRIA_ENV", env)
	t.Setenv("ATRIA_USER", atriaUser)
	t.Setenv("PROXY_AUTH_HEADER", "Remote-Email")
	t.Setenv("PROXY_ALLOWLIST", "10.0.0.1")

	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.Use(srv.AuthMiddleware())
	r.Use(srv.CSRFMiddleware())

	r.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	r.GET("/token", func(c *gin.Context) {
		token := srv.GetCSRFToken(c)
		c.String(http.StatusOK, token)
	})

	var target string
	if method == http.MethodGet {
		target = "/token"
	} else {
		target = "/test"
	}

	req, err := http.NewRequest(method, target, nil)
	require.NoError(t, err)
	req.RemoteAddr = proxyClientIP // reuse constant from middleware_test.go

	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// runCSRFTestWithBody is like runCSRFTest but lets the caller supply a custom
// request body and content-type (used for form-encoded CSRF submission).
func runCSRFTestWithBody(t *testing.T, srv *web.Server, env, atriaUser string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()

	t.Setenv("ATRIA_ENV", env)
	t.Setenv("ATRIA_USER", atriaUser)
	t.Setenv("PROXY_AUTH_HEADER", "Remote-Email")
	t.Setenv("PROXY_ALLOWLIST", "10.0.0.1")

	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.Use(srv.AuthMiddleware())
	r.Use(srv.CSRFMiddleware())

	r.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	r.GET("/token", func(c *gin.Context) {
		token := srv.GetCSRFToken(c)
		c.String(http.StatusOK, token)
	})

	req, err := http.NewRequest(http.MethodPost, "/test", body)
	require.NoError(t, err)
	req.RemoteAddr = proxyClientIP
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// fetchCSRFToken performs a GET /token request and returns the token string.
func fetchCSRFToken(t *testing.T, srv *web.Server, env, atriaUser string) string {
	t.Helper()
	w := runCSRFTest(t, srv, env, atriaUser, "", http.MethodGet)
	require.Equal(t, http.StatusOK, w.Code, "expected 200 when fetching CSRF token")
	token := w.Body.String()
	require.NotEmpty(t, token, "CSRF token must not be empty")
	return token
}

func TestCSRFMiddleware(t *testing.T) {
	db, testUser := testutil.SetupTestDB(t)
	defer db.Close()

	srv := web.NewServer(db)

	t.Run("GET request is always allowed", func(t *testing.T) {
		// A GET to /token carries no CSRF token and must still return 200.
		w := runCSRFTest(t, srv, "development", testUser.Email, "", http.MethodGet)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST without CSRF token is blocked", func(t *testing.T) {
		w := runCSRFTest(t, srv, "development", testUser.Email, "", http.MethodPost)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("POST with wrong CSRF token is blocked", func(t *testing.T) {
		w := runCSRFTest(t, srv, "development", testUser.Email, "wrongtoken", http.MethodPost)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("POST with correct CSRF token via header is allowed", func(t *testing.T) {
		// Step 1: obtain the real token via GET /token.
		token := fetchCSRFToken(t, srv, "development", testUser.Email)

		// Step 2: POST with the token in X-CSRF-Token.
		w := runCSRFTest(t, srv, "development", testUser.Email, token, http.MethodPost)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST with correct CSRF token via form field is allowed", func(t *testing.T) {
		token := fetchCSRFToken(t, srv, "development", testUser.Email)

		// Submit token as _csrf form field.
		formBody := strings.NewReader("_csrf=" + token)
		w := runCSRFTestWithBody(t, srv, "development", testUser.Email, formBody, "application/x-www-form-urlencoded")
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("CSRF token is deterministic for same user", func(t *testing.T) {
		token1 := fetchCSRFToken(t, srv, "development", testUser.Email)
		token2 := fetchCSRFToken(t, srv, "development", testUser.Email)
		assert.Equal(t, token1, token2, "CSRF token must be stable across calls for the same user")
	})

	t.Run("CSRF token differs between users", func(t *testing.T) {
		// Create a second user directly in the same database so we don't reset it.
		secondUser, err := users.CreateUser(
			context.Background(), db,
			"second_user@atria.local", "Second User", "pass2", core.RoleUser,
		)
		require.NoError(t, err)

		token1 := fetchCSRFToken(t, srv, "development", testUser.Email)
		token2 := fetchCSRFToken(t, srv, "development", secondUser.Email)
		assert.NotEqual(t, token1, token2, "CSRF tokens for different users must differ")
	})
}
