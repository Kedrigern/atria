package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/notes"

	"github.com/russross/blackfriday/v2"
)

var (
	notePath   string
	noteFile   string
	exportPath string
	recursive  bool
	hardDelete bool
	showFormat string
)

var noteCmd = &cobra.Command{
	Use:               "note",
	Short:             "Knowledge base and markdown notes management",
	PersistentPreRunE: RequireUserContext,
}

var noteAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Creates a new note (reads from --file or stdin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]
		var content []byte
		var err error

		if noteFile != "" {
			content, err = os.ReadFile(noteFile)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
		} else {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				content, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
			} else {
				return fmt.Errorf("no content provided. Use --file or pipe content via stdin")
			}
		}

		entity, err := notes.CreateNote(app.Ctx, app.DB, app.Owner.ID, title, notePath, string(content))
		if err != nil {
			return fmt.Errorf("failed to create note: %w", err)
		}

		fmt.Printf("✅ Note created successfully!\nID: %s\nTitle: %s\n", entity.ID, entity.Title)
		return nil
	},
}

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists existing notes in the knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		noteList, err := notes.ListNotes(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to list notes: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tPATH\tTITLE\tCREATED")
		for _, n := range noteList {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", FormatID(n.ID, showLong), n.Path, n.Title, n.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <uuid|short-uuid|\"Title\">",
	Short: "Displays the content of a specific note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			return err
		}

		content, err := notes.GetNoteContent(app.Ctx, app.DB, targetNote.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch content: %w", err)
		}

		switch showFormat {
		case "html":
			htmlOutput := blackfriday.Run([]byte(content))
			fmt.Println(string(htmlOutput))
		case "plain":
			fallthrough
		case "md":
			fallthrough
		default:
			fmt.Println(content)
		}
		return nil
	},
}

var noteExportCmd = &cobra.Command{
	Use:   "export <uuid|short-uuid|\"Title\">",
	Short: "Exports a note to the local filesystem as a markdown file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			return err
		}

		content, err := notes.GetNoteContent(app.Ctx, app.DB, targetNote.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch content: %w", err)
		}

		// Sanitize title for filename
		safeTitle := strings.ReplaceAll(targetNote.Title, " ", "_")
		safeTitle = strings.ReplaceAll(safeTitle, "/", "-")
		safeTitle = strings.ReplaceAll(safeTitle, "\\", "-")
		filename := strings.ToLower(safeTitle) + ".md"
		fullPath := filepath.Join(exportPath, filename)

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file to disk: %w", err)
		}

		fmt.Printf("✅ Successfully exported to: %s\n", fullPath)
		return nil
	},
}

var noteRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid|\"Title\">",
	Short: "Deletes a note or folder (soft delete by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			return err
		}

		err = notes.DeleteEntity(app.Ctx, app.DB, app.Owner.ID, targetNote.ID, recursive, hardDelete)
		if err != nil {
			return fmt.Errorf("delete failed: %w", err)
		}

		if hardDelete {
			fmt.Printf("🔥 Permanently deleted: %s (%s)\n", targetNote.Title, targetNote.ID)
		} else {
			fmt.Printf("🗑️  Moved to trash (soft deleted): %s (%s)\n", targetNote.Title, targetNote.ID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(noteCmd)
	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteShowCmd, noteExportCmd, noteRmCmd)

	noteAddCmd.Flags().StringVar(&notePath, "path", "/", "Virtual path (e.g., /home/solar)")
	noteAddCmd.Flags().StringVar(&noteFile, "file", "", "Path to a local .md file to read content from")
	noteShowCmd.Flags().StringVar(&showFormat, "format", "md", "Output format (md, html, plain)")
	noteRmCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively delete folders and their contents")
	noteRmCmd.Flags().BoolVar(&hardDelete, "hard", false, "Permanently delete the item from the database (cannot be undone)")
	noteExportCmd.Flags().StringVar(&exportPath, "out", ".", "Local directory to export the markdown file to")
	noteListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
}
