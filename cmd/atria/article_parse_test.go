package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runParse(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	// Flag variables are package globals; reset them between runs.
	parseFile, parseURL, parseFormat, parseSanitized, parseSaveHTML = "", "", "md", false, ""
	t.Setenv("DATABASE_URL", "postgres://invalid-host-should-not-be-used/none")

	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(append([]string{"article", "parse"}, args...))
	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func TestArticleParseCommand(t *testing.T) {
	page := `<html><head><title>CLI story</title></head><body><article><h1>CLI story</h1>` +
		strings.Repeat(`<p>Enough paragraph text for readability to choose this block as the article.</p>`, 8) +
		`<img src="/pic.png" onerror="alert(1)"></article></body></html>`
	file := filepath.Join(t.TempDir(), "page.html")
	require.NoError(t, os.WriteFile(file, []byte(page), 0o600))

	t.Run("markdown to stdout, metadata to stderr, no database needed", func(t *testing.T) {
		out, meta, err := runParse(t, "--file", file, "--url", "https://news.example/a")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out, "# CLI story\n"), out)
		assert.Contains(t, out, "https://news.example/pic.png", "relative URLs resolve against --url")
		assert.Contains(t, meta, "Title:    CLI story")
		assert.NotContains(t, out, "Title:")
	})

	t.Run("json", func(t *testing.T) {
		out, _, err := runParse(t, "--file", file, "--url", "https://news.example/a", "-f", "json")
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "CLI story", got["title"])
		assert.Equal(t, "https://news.example/a", got["url"])
		assert.Contains(t, got["html"], "<p>")
	})

	t.Run("sanitized html", func(t *testing.T) {
		raw, _, err := runParse(t, "--file", file, "--url", "https://news.example/a", "-f", "html")
		require.NoError(t, err)
		clean, _, err := runParse(t, "--file", file, "--url", "https://news.example/a", "-f", "html", "--sanitized")
		require.NoError(t, err)
		assert.Contains(t, raw, "onerror")
		assert.NotContains(t, clean, "onerror")
	})

	t.Run("usage errors", func(t *testing.T) {
		_, _, err := runParse(t)
		assert.ErrorContains(t, err, "give a URL")
		_, _, err = runParse(t, "--file", file, "--format", "pdf")
		assert.ErrorContains(t, err, "unknown --format")
		_, _, err = runParse(t, "--file", file, "--url", "not-a-url")
		assert.ErrorContains(t, err, "absolute URL")
	})
}
