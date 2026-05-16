package web

// White-box tests for unexported security helper functions.
// This file lives in package "web" (not "web_test") so it can access
// hasExactGroup and isProxyAllowed directly.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasExactGroup(t *testing.T) {
	tests := []struct {
		name        string
		groupHeader string
		group       string
		want        bool
	}{
		{
			name:        "single matching group",
			groupHeader: "admins",
			group:       "admins",
			want:        true,
		},
		{
			name:        "group present among many",
			groupHeader: "editors,admins,family",
			group:       "admins",
			want:        true,
		},
		{
			name:        "substring match must not match",
			groupHeader: "not-admins,super-admins",
			group:       "admins",
			want:        false,
		},
		{
			name:        "empty header",
			groupHeader: "",
			group:       "admins",
			want:        false,
		},
		{
			name:        "case sensitive – uppercase does not match",
			groupHeader: "ADMINS",
			group:       "admins",
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasExactGroup(tc.groupHeader, tc.group)
			assert.Equal(t, tc.want, got)
		})
	}
}

// proxyAllowedResult is the JSON shape returned by the helper route below.
type proxyAllowedResult struct {
	Allowed bool `json:"allowed"`
}

// buildProxyRouter creates a minimal gin router with a single GET /check route
// that calls isProxyAllowed and returns the result as JSON.
// This lets us exercise isProxyAllowed while controlling the request's RemoteAddr.
func buildProxyRouter(t *testing.T, allowlist string) *gin.Engine {
	t.Helper()
	t.Setenv("PROXY_ALLOWLIST", allowlist)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Disable gin's trusted-proxy magic so RemoteAddr is used directly.
	r.SetTrustedProxies(nil)

	r.GET("/check", func(c *gin.Context) {
		c.JSON(http.StatusOK, proxyAllowedResult{Allowed: isProxyAllowed(c)})
	})
	return r
}

// fireProxyCheck sends a GET /check with the given remoteAddr and returns the result.
func fireProxyCheck(t *testing.T, r *gin.Engine, remoteAddr string) bool {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/check", nil)
	require.NoError(t, err)
	req.RemoteAddr = remoteAddr

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var result proxyAllowedResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	return result.Allowed
}

func TestIsProxyAllowed(t *testing.T) {
	t.Run("empty allowlist always returns false", func(t *testing.T) {
		r := buildProxyRouter(t, "")
		assert.False(t, fireProxyCheck(t, r, "10.0.0.1:1234"))
	})

	t.Run("IP in single-entry allowlist is allowed", func(t *testing.T) {
		r := buildProxyRouter(t, "10.0.0.1")
		assert.True(t, fireProxyCheck(t, r, "10.0.0.1:1234"))
	})

	t.Run("different IP is not allowed", func(t *testing.T) {
		r := buildProxyRouter(t, "10.0.0.1")
		assert.False(t, fireProxyCheck(t, r, "10.0.0.2:1234"))
	})

	t.Run("IP in multi-entry allowlist is allowed", func(t *testing.T) {
		r := buildProxyRouter(t, "10.0.0.1,192.168.1.1")
		assert.True(t, fireProxyCheck(t, r, "192.168.1.1:9999"))
	})

	t.Run("first IP in multi-entry allowlist is allowed", func(t *testing.T) {
		r := buildProxyRouter(t, "10.0.0.1,192.168.1.1")
		assert.True(t, fireProxyCheck(t, r, "10.0.0.1:9999"))
	})

	t.Run("IP not in multi-entry allowlist is blocked", func(t *testing.T) {
		r := buildProxyRouter(t, "10.0.0.1,192.168.1.1")
		assert.False(t, fireProxyCheck(t, r, "172.16.0.1:1234"))
	})
}
