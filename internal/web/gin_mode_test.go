package web

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGinMode(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	t.Setenv("ATRIA_ENV", "production")
	assert.Equal(t, gin.ReleaseMode, ginMode())

	t.Setenv("ATRIA_ENV", "")
	assert.Equal(t, gin.ReleaseMode, ginMode(), "an unset ATRIA_ENV is not development")

	t.Setenv("ATRIA_ENV", "development")
	assert.Equal(t, gin.DebugMode, ginMode())

	t.Setenv("GIN_MODE", gin.TestMode)
	assert.Equal(t, gin.TestMode, ginMode(), "an explicit GIN_MODE wins")
}
