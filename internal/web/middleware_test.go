package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type authResponse struct {
	Email string    `json:"email"`
	Role  core.Role `json:"role"`
}

// runAuthTest builds an HTTP request and runs it through the auth middleware.
func runAuthTest(t *testing.T, srv *web.Server, env, atriaUser, headerEmail, headerGroups string) *httptest.ResponseRecorder {
	t.Helper()

	// Isolate environment variables per test.
	t.Setenv("ATRIA_ENV", env)
	t.Setenv("ATRIA_USER", atriaUser)
	t.Setenv("PROXY_AUTH_HEADER", "Remote-Email")

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Apply middleware under test.
	r.Use(srv.AuthMiddleware())

	// Test route returns currentUser values from context.
	r.GET("/test", func(c *gin.Context) {
		val, exists := c.Get("currentUser")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "User missing in context"})
			return
		}
		user, ok := val.(*core.User)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user type in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"email": user.Email, "role": user.Role})
	})

	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	require.NoError(t, err)

	if headerEmail != "" {
		req.Header.Set("Remote-Email", headerEmail)
	}
	if headerGroups != "" {
		req.Header.Set("Remote-Groups", headerGroups)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeAuthResponse(t *testing.T, w *httptest.ResponseRecorder) authResponse {
	t.Helper()

	var resp authResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	return resp
}

func TestAuthMiddleware(t *testing.T) {
	// Connect to real test database prepared by test helpers.
	db, testUser := testutil.SetupTestDB(t)
	defer db.Close()

	srv := web.NewServer(db)

	t.Run("Production: Valid header allows access", func(t *testing.T) {
		w := runAuthTest(t, srv, "production", "hacker@atria.local", testUser.Email, "")
		require.Equal(t, http.StatusOK, w.Code)

		resp := decodeAuthResponse(t, w)
		assert.Equal(t, testUser.Email, resp.Email)
	})

	t.Run("Production: Missing header blocks access (ignores ATRIA_USER)", func(t *testing.T) {
		// Even if ATRIA_USER is valid, production middleware must ignore it.
		w := runAuthTest(t, srv, "production", testUser.Email, "", "")
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Production: Unknown user in header blocks access", func(t *testing.T) {
		w := runAuthTest(t, srv, "production", "", "unknown_person@atria.local", "")
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Development: Missing header falls back to ATRIA_USER", func(t *testing.T) {
		// In dev mode, ATRIA_USER fallback should allow access when header is absent.
		w := runAuthTest(t, srv, "development", testUser.Email, "", "")
		require.Equal(t, http.StatusOK, w.Code)

		resp := decodeAuthResponse(t, w)
		assert.Equal(t, testUser.Email, resp.Email)
	})

	t.Run("Development: Header takes precedence over ATRIA_USER", func(t *testing.T) {
		// A real proxy header should win even in development.
		w := runAuthTest(t, srv, "development", "hacker@atria.local", testUser.Email, "")
		require.Equal(t, http.StatusOK, w.Code)

		resp := decodeAuthResponse(t, w)
		assert.Equal(t, testUser.Email, resp.Email) // Must be testUser, not hacker.
	})

	t.Run("Role synchronization: Adds admin role based on groups", func(t *testing.T) {
		// Verify initial role.
		assert.Equal(t, core.RoleUser, testUser.Role)

		// Send admin group membership via proxy header.
		w := runAuthTest(t, srv, "production", "", testUser.Email, "editors,admins,family")
		require.Equal(t, http.StatusOK, w.Code)

		resp := decodeAuthResponse(t, w)
		assert.Equal(t, core.RoleAdmin, resp.Role)

		// Verify role was persisted to database.
		updatedUser, err := core.FindUser(t.Context(), db, testUser.Email)
		require.NoError(t, err)
		assert.Equal(t, core.RoleAdmin, updatedUser.Role)
	})
}
