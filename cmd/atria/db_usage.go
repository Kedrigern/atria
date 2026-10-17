package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"atria/internal/cli"
	"atria/internal/core"
	"atria/internal/storage"
)

var usageUser string

var dbUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Shows how much storage each user's data takes",
	Long: `Shows attachments, articles, notes and RSS items per user, largest first,
plus instance totals and the overall database size.

Text sizes are PostgreSQL's stored (compressed) sizes; attachment sizes are the
stored file sizes. Items in the trash are included.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDatabase()
		if err != nil {
			return err
		}
		defer db.Close()
		ctx := context.Background()

		var list []storage.Usage
		if usageUser != "" {
			user, err := core.FindUser(ctx, db, usageUser)
			if err != nil {
				return err
			}
			u, err := storage.ForUser(ctx, db, user.ID)
			if err != nil {
				return err
			}
			list = []storage.Usage{*u}
		} else if list, err = storage.ForAllUsers(ctx, db); err != nil {
			return err
		}

		var total storage.Usage
		for _, u := range list {
			total.Add(u)
		}
		dbSize, err := storage.DatabaseSize(ctx, db)
		if err != nil {
			return err
		}

		return renderUsage(cmd, list, total, dbSize)
	},
}

func renderUsage(cmd *cobra.Command, list []storage.Usage, total storage.Usage, dbSize int64) error {
	out := cmd.OutOrStdout()
	if listFormat == "json" {
		return cli.Render(out, "json", nil, nil, struct {
			Users         []storage.Usage `json:"users"`
			Total         storage.Usage   `json:"total"`
			DatabaseBytes int64           `json:"database_bytes"`
		}{list, total, dbSize})
	}

	var headers []string
	var row func(name string, u storage.Usage) []string
	if listFormat == "csv" {
		// Machine-readable: separate columns with exact counts and bytes.
		headers = []string{"user", "attachment_count", "attachment_bytes", "article_count", "article_bytes",
			"note_count", "note_bytes", "rss_item_count", "rss_bytes", "total_bytes"}
		row = func(name string, u storage.Usage) []string {
			n := func(v int64) string { return strconv.FormatInt(v, 10) }
			return []string{name, n(u.AttachmentCount), n(u.AttachmentBytes), n(u.ArticleCount), n(u.ArticleBytes),
				n(u.NoteCount), n(u.NoteBytes), n(u.RSSItemCount), n(u.RSSBytes), n(u.TotalBytes())}
		}
	} else {
		headers = []string{"USER", "ATTACHMENTS", "ARTICLES", "NOTES", "RSS", "TOTAL"}
		row = func(name string, u storage.Usage) []string {
			cell := func(bytes, count int64) string { return fmt.Sprintf("%s (%d)", core.FormatBytes(bytes), count) }
			return []string{name,
				cell(u.AttachmentBytes, u.AttachmentCount),
				cell(u.ArticleBytes, u.ArticleCount),
				cell(u.NoteBytes, u.NoteCount),
				cell(u.RSSBytes, u.RSSItemCount),
				core.FormatBytes(u.TotalBytes())}
		}
	}
	var rows [][]string
	for _, u := range list {
		rows = append(rows, row(u.Email, u))
	}
	if len(list) > 1 && listFormat != "csv" {
		rows = append(rows, row("TOTAL", total))
	}
	if err := cli.Render(out, listFormat, headers, rows, list); err != nil {
		return err
	}
	if listFormat == "table" || listFormat == "" {
		fmt.Fprintf(out, "\nDatabase size (incl. indexes and search data): %s\n", core.FormatBytes(dbSize))
		fmt.Fprintln(out, "Sizes are stored (compressed) sizes; counts in parentheses. Trash included.")
	}
	return nil
}

func init() {
	dbCmd.AddCommand(dbUsageCmd)
	dbUsageCmd.Flags().StringVarP(&usageUser, "user", "u", "", "Only this user (email or UUID)")
	dbUsageCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")
}
