package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/netutil"
)

var (
	parseFile      string
	parseURL       string
	parseFormat    string
	parseSanitized bool
	parseSaveHTML  string
)

var articleParseCmd = &cobra.Command{
	Use:   "parse [url]",
	Short: "Runs article extraction on a URL or saved HTML file and prints the result (nothing is saved)",
	Long: `Runs the same extraction pipeline as "article add" and prints the result
without touching the database. Useful for checking how a site is parsed and
for creating test fixtures.

Metadata (title, byline, length, ...) is printed to stderr, the content to
stdout, so the output can be redirected to a file.

Examples:
  atria article parse https://example.com/post
  atria article parse https://example.com/post --save-html post.html
  atria article parse --file post.html --url https://example.com/post --format html
  curl -s https://example.com/post | atria article parse --file - --url https://example.com/post`,
	Args: cobra.MaximumNArgs(1),
	// Parsing needs neither the database nor a user.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		parsed, err := runArticleParse(cmd, args)
		if err != nil {
			return err
		}
		return printParsedArticle(cmd.OutOrStdout(), cmd.ErrOrStderr(), parsed)
	},
}

func runArticleParse(cmd *cobra.Command, args []string) (*articles.ParsedArticle, error) {
	switch parseFormat {
	case "md", "html", "text", "json":
	default:
		return nil, fmt.Errorf("unknown --format %q (use md, html, text or json)", parseFormat)
	}

	if parseFile == "" {
		if len(args) != 1 {
			return nil, errors.New("give a URL, or --file with --url")
		}
		// The CLI runs with the user's own privileges, so local addresses
		// (e.g. a dev server) are allowed, as in the other CLI commands.
		ctx := context.WithValue(context.Background(), netutil.AllowLocalKey, true)
		parsed, page, err := articles.ParseURL(ctx, args[0])
		if err != nil {
			return nil, err
		}
		if parseSaveHTML != "" {
			if err := os.WriteFile(parseSaveHTML, []byte(page), 0o644); err != nil {
				return nil, fmt.Errorf("save HTML: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Saved page to %s\n", parseSaveHTML)
		}
		return parsed, nil
	}

	if len(args) > 0 {
		return nil, errors.New("use either a URL argument or --file, not both")
	}
	if parseSaveHTML != "" {
		return nil, errors.New("--save-html only applies when fetching a URL")
	}
	pageURL := parseURL
	if pageURL == "" {
		pageURL = "https://example.com/"
		fmt.Fprintln(cmd.ErrOrStderr(), "Note: no --url given; relative links resolve against https://example.com/ and site-name title detection is off.")
	}
	base, err := url.Parse(pageURL)
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("--url must be an absolute URL, got %q", pageURL)
	}

	var page []byte
	if parseFile == "-" {
		page, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), 32<<20))
	} else {
		page, err = os.ReadFile(parseFile)
	}
	if err != nil {
		return nil, fmt.Errorf("read HTML: %w", err)
	}
	return articles.ParseHTML(string(page), base)
}

func printParsedArticle(out, meta io.Writer, p *articles.ParsedArticle) error {
	if parseSanitized {
		p.HTML = core.SanitizeHTML(p.HTML)
	}

	if parseFormat == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(p)
	}

	fmt.Fprintf(meta, "Title:    %s\n", p.Title)
	for _, field := range []struct{ label, value string }{
		{"Byline", p.Byline}, {"Site", p.SiteName}, {"Language", p.Language}, {"Excerpt", p.Excerpt},
	} {
		if strings.TrimSpace(field.value) != "" {
			fmt.Fprintf(meta, "%-9s %s\n", field.label+":", field.value)
		}
	}
	if p.PublishedTime != nil {
		fmt.Fprintf(meta, "Published: %s\n", p.PublishedTime.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(meta, "Length:   %d characters (≈ %.1f normostran)\n---\n", p.Length, float64(p.Length)/1800)

	switch parseFormat {
	case "html":
		_, err := fmt.Fprintln(out, p.HTML)
		return err
	case "text":
		_, err := fmt.Fprintln(out, strings.TrimSpace(p.Text))
		return err
	default:
		md, err := articles.HTMLToMarkdown(p.HTML)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "# %s\n\n%s\n", p.Title, md)
		return err
	}
}

func init() {
	articleCmd.AddCommand(articleParseCmd)
	articleParseCmd.Flags().StringVar(&parseFile, "file", "", "Parse a saved HTML file instead of fetching ('-' reads stdin)")
	articleParseCmd.Flags().StringVar(&parseURL, "url", "", "Original page URL when using --file (resolves relative links)")
	articleParseCmd.Flags().StringVarP(&parseFormat, "format", "f", "md", "Output format: md, html, text, json")
	articleParseCmd.Flags().BoolVar(&parseSanitized, "sanitized", false, "Apply the web UI's HTML sanitizer (shows what readers actually see)")
	articleParseCmd.Flags().StringVar(&parseSaveHTML, "save-html", "", "Also save the downloaded page to this file (e.g. as a test fixture)")
}
