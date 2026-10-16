package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func newTestRouter(t *testing.T, srv *Server) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return srv.SetupRouter()
}

func postLogin(r http.Handler, email, password string, headers map[string]string) *httptest.ResponseRecorder {
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sessionCookieFrom(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func getWithCookie(r http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSessionLifecycle(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("ATRIA_ENV", "production")
	t.Setenv("SESSION_SECRET", testSecret)
	t.Setenv("PROXY_ALLOWLIST", "")

	srv := NewServer(db)
	r := newTestRouter(t, srv)

	w := postLogin(r, user.Email, "pass", nil)
	require.Equal(t, http.StatusFound, w.Code)
	cookie := sessionCookieFrom(w)
	require.NotNil(t, cookie, "login must set a session cookie")
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)

	t.Run("valid cookie authenticates", func(t *testing.T) {
		w := getWithCookie(r, "/profile", cookie)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("tampered email is rejected", func(t *testing.T) {
		parts := strings.Split(cookie.Value, ".")
		parts[1] = base64.RawURLEncoding.EncodeToString([]byte("someone-else@atria.local"))
		forged := &http.Cookie{Name: sessionCookieName, Value: strings.Join(parts, ".")}
		assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", forged).Code)
	})

	t.Run("legacy email|hmac cookie is rejected", func(t *testing.T) {
		legacy := &http.Cookie{Name: sessionCookieName, Value: user.Email + "|deadbeef"}
		assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", legacy).Code)
	})

	t.Run("expired cookie is rejected", func(t *testing.T) {
		expires := time.Now().Add(-time.Minute).Unix()
		value := strings.Join([]string{
			"v1",
			base64.RawURLEncoding.EncodeToString([]byte(user.Email)),
			strconv.FormatInt(expires, 10),
			signSession([]byte(testSecret), user.Email, expires, user.PasswordHash),
		}, ".")
		expired := &http.Cookie{Name: sessionCookieName, Value: value}
		assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", expired).Code)
	})

	t.Run("cookie signed with another secret is rejected", func(t *testing.T) {
		t.Setenv("SESSION_SECRET", "a-completely-different-secret-value-xx")
		assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", cookie).Code)
	})

	t.Run("password change invalidates existing sessions", func(t *testing.T) {
		_, err := db.ExecContext(context.Background(),
			`UPDATE users SET password_hash = $1 WHERE id = $2`, user.PasswordHash+"x", user.ID)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", cookie).Code)
	})
}

func TestSessionRejectedWithoutSecretInProduction(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("ATRIA_ENV", "development")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("ATRIA_USER", "")

	srv := NewServer(db)
	r := newTestRouter(t, srv)

	// A cookie minted with the development fallback secret...
	w := postLogin(r, user.Email, "pass", nil)
	cookie := sessionCookieFrom(w)
	require.NotNil(t, cookie)

	// ...must not be accepted once the server runs in production.
	t.Setenv("ATRIA_ENV", "production")
	assert.Equal(t, http.StatusUnauthorized, getWithCookie(r, "/profile", cookie).Code)
	assert.Error(t, ValidateSecurityConfig())
}

func TestValidateSecurityConfig(t *testing.T) {
	cases := []struct {
		env, secret string
		wantErr     bool
	}{
		{"development", "", false},
		{"test", "", false},
		{"production", "", true},
		{"", "", true},
		{"production", exampleSessionSecret, true},
		{"production", testSecret, false},
	}
	for _, c := range cases {
		t.Setenv("ATRIA_ENV", c.env)
		t.Setenv("SESSION_SECRET", c.secret)
		err := ValidateSecurityConfig()
		assert.Equal(t, c.wantErr, err != nil, "env=%q secret=%q err=%v", c.env, c.secret, err)
	}
}

func TestLoginHardening(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("ATRIA_ENV", "production")
	t.Setenv("SESSION_SECRET", testSecret)

	srv := NewServer(db)
	r := newTestRouter(t, srv)

	t.Run("cross-site login is refused", func(t *testing.T) {
		w := postLogin(r, user.Email, "pass", map[string]string{"Sec-Fetch-Site": "cross-site"})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Nil(t, sessionCookieFrom(w))
	})

	t.Run("repeated failures lock out the client", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			w := postLogin(r, user.Email, "wrong", nil)
			require.Nil(t, sessionCookieFrom(w))
		}
		w := postLogin(r, user.Email, "pass", nil)
		assert.Nil(t, sessionCookieFrom(w), "correct password must be refused while locked out")
	})
}

func TestSecurityHeaders(t *testing.T) {
	db, _ := testutil.SetupTestDB(t)
	t.Setenv("ATRIA_ENV", "production")
	t.Setenv("SESSION_SECRET", testSecret)

	r := newTestRouter(t, NewServer(db))
	w := getWithCookie(r, "/login", nil)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "object-src 'none'")
}

func TestLocalRedirectTarget(t *testing.T) {
	cases := map[string]string{
		"":                                  "/fallback",
		"https://evil.example/phish":        "/phish",
		"https://atria.local/notes/1?x=2":   "/notes/1?x=2",
		"//evil.example/x":                  "/x",
		"javascript:alert(1)":               "/fallback",
		"https://atria.local//evil.example": "/fallback",
	}
	gin.SetMode(gin.TestMode)
	for referer, want := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Request.Header.Set("Referer", referer)
		assert.Equal(t, want, localRedirectTarget(c, "/fallback"), "referer %q", referer)
	}
}

func TestIsInlineAttachmentType(t *testing.T) {
	assert.True(t, isInlineAttachmentType("image/png"))
	assert.True(t, isInlineAttachmentType("text/plain; charset=utf-8"))
	assert.True(t, isInlineAttachmentType("application/pdf"))
	assert.False(t, isInlineAttachmentType("image/svg+xml"))
	assert.False(t, isInlineAttachmentType("text/html; charset=utf-8"))
	assert.False(t, isInlineAttachmentType("application/javascript"))
	assert.False(t, isInlineAttachmentType(""))
}

// asUser routes requests through the development auto-login as the given user.
func asUser(t *testing.T, user *core.User) {
	t.Setenv("ATRIA_ENV", "development")
	t.Setenv("SESSION_SECRET", testSecret)
	t.Setenv("ATRIA_USER", user.Email)
}

func postForm(r http.Handler, path string, user *core.User, form url.Values) *httptest.ResponseRecorder {
	form.Set("_csrf", csrfTokenFor(user.Email))
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCrossUserWritesAreRejected(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	ctx := context.Background()
	t.Setenv("STORAGE_PATH", t.TempDir())

	attacker, err := users.CreateUser(ctx, db, "attacker@atria.local", "Attacker", "pass", core.RoleUser)
	require.NoError(t, err)
	note, err := notes.CreateNote(ctx, db, owner.ID, "Owner note", "", "secret")
	require.NoError(t, err)
	other, err := notes.CreateNote(ctx, db, attacker.ID, "Attacker note", "", "x")
	require.NoError(t, err)

	r := newTestRouter(t, NewServer(db))
	asUser(t, attacker)
	noteURL := "/api/entity/" + note.ID.String()

	t.Run("tag attach", func(t *testing.T) {
		w := postForm(r, noteURL+"/tags", attacker, url.Values{"tag_name": {"pwned"}})
		assert.Equal(t, http.StatusNotFound, w.Code)
		tags, err := core.GetEntityTags(ctx, db, owner.ID, note.ID)
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("link add", func(t *testing.T) {
		w := postForm(r, noteURL+"/links", attacker, url.Values{"target_identifier": {other.ID.String()}})
		assert.Equal(t, http.StatusNotFound, w.Code)
		var count int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rel_entity_links WHERE source_id = $1`, note.ID).Scan(&count))
		assert.Zero(t, count)
	})

	t.Run("attachment upload", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		require.NoError(t, mw.WriteField("_csrf", csrfTokenFor(attacker.Email)))
		fw, err := mw.CreateFormFile("file", "x.txt")
		require.NoError(t, err)
		_, _ = fw.Write([]byte("hello"))
		require.NoError(t, mw.Close())

		req := httptest.NewRequest(http.MethodPost, noteURL+"/attachments", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAttachmentServing(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	ctx := context.Background()
	t.Setenv("STORAGE_PATH", t.TempDir())

	note, err := notes.CreateNote(ctx, db, owner.ID, "With attachment", "", "x")
	require.NoError(t, err)

	r := newTestRouter(t, NewServer(db))
	asUser(t, owner)

	upload := func(filename, content string) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		require.NoError(t, mw.WriteField("_csrf", csrfTokenFor(owner.Email)))
		fw, err := mw.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, _ = fw.Write([]byte(content))
		require.NoError(t, mw.Close())
		req := httptest.NewRequest(http.MethodPost, "/api/entity/"+note.ID.String()+"/attachments", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	upload("evil.html", "<html><body><script>alert(1)</script></body></html>")

	var diskPath string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT disk_path FROM attachments WHERE owner_id = $1`, owner.ID).Scan(&diskPath))

	t.Run("html is forced to download in a sandbox", func(t *testing.T) {
		w := getWithCookie(r, "/data/attachments/"+diskPath, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
		assert.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox")
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	})

	t.Run("private attachment is hidden from other users", func(t *testing.T) {
		intruder, err := users.CreateUser(ctx, db, "intruder@atria.local", "Intruder", "pass", core.RoleUser)
		require.NoError(t, err)
		asUser(t, intruder)
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/data/attachments/"+diskPath, nil).Code)
	})

	t.Run("private attachment is hidden from anonymous visitors", func(t *testing.T) {
		t.Setenv("ATRIA_ENV", "production")
		assert.Equal(t, http.StatusNotFound, getWithCookie(r, "/data/attachments/"+diskPath, nil).Code)
	})
}
