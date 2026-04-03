package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/notes"

	"github.com/russross/blackfriday/v2"
)

var (
	notePath   string
	noteFile   string
	exportPath string // used by note export
	recursive  bool
	hardDelete bool
	showFormat string
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Knowledge base and markdown notes management",
}

var noteAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Creates a new note (reads from --file or stdin)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		title := args[0]
		var content []byte
		var err error

		if noteFile != "" {
			content, err = os.ReadFile(noteFile)
			if err != nil {
				log.Fatalf("Failed to read file: %v", err)
			}
		} else {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				content, err = io.ReadAll(os.Stdin)
				if err != nil {
					log.Fatalf("Failed to read from stdin: %v", err)
				}
			} else {
				log.Fatal("ERROR: No content provided. Use --file or pipe content via stdin.")
			}
		}

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		entity, err := notes.CreateNote(ctx, db, owner.ID, title, notePath, string(content))
		if err != nil {
			log.Fatalf("Failed to create note: %v", err)
		}

		fmt.Printf("✅ Note created successfully!\nID: %s\nTitle: %s\n", entity.ID, entity.Title)
	},
}

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists existing notes in the knowledge base",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		noteList, err := notes.ListNotes(ctx, db, owner.ID)
		if err != nil {
			log.Fatalf("Failed to list notes: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tPATH\tTITLE\tCREATED")
		for _, n := range noteList {
			shortID := n.ID.String()[:8]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", shortID, n.Path, n.Title, n.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <uuid|short-uuid|\"Title\">",
	Short: "Displays the content of a specific note",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
		targetNote := resolveEntityOrExit(ctx, db, owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			os.Exit(1)
		}

		content, err := notes.GetNoteContent(ctx, db, targetNote.ID)
		if err != nil {
			log.Fatalf("Failed to fetch content: %v", err)
		}

		switch showFormat {
		case "html":
			htmlOutput := blackfriday.Run([]byte(content))
			fmt.Println(string(htmlOutput))
		case "plain":
			fmt.Println(content)
		case "md":
			fallthrough
		default:
			fmt.Println(content)
		}
	},
}

var noteExportCmd = &cobra.Command{
	Use:   "export <uuid|short-uuid|\"Title\">",
	Short: "Exports a note to the local filesystem as a markdown file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		targetNote := resolveEntityOrExit(ctx, db, owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			os.Exit(1)
		}

		content, err := notes.GetNoteContent(ctx, db, targetNote.ID)
		if err != nil {
			log.Fatalf("Failed to fetch content: %v", err)
		}

		// Sanitize title for filename
		safeTitle := strings.ReplaceAll(targetNote.Title, " ", "_")
		safeTitle = strings.ReplaceAll(safeTitle, "/", "-")
		safeTitle = strings.ReplaceAll(safeTitle, "\\", "-")
		filename := strings.ToLower(safeTitle) + ".md"
		fullPath := filepath.Join(exportPath, filename)

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			log.Fatalf("Failed to write file to disk: %v", err)
		}

		fmt.Printf("✅ Successfully exported to: %s\n", fullPath)
	},
}

var noteRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid|\"Title\">",
	Short: "Deletes a note or folder (soft delete by default)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		targetNote := resolveEntityOrExit(ctx, db, owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			os.Exit(1)
		}

		err = notes.DeleteEntity(ctx, db, owner.ID, targetNote.ID, recursive, hardDelete)
		if err != nil {
			log.Fatalf("Delete failed: %v", err)
		}

		if hardDelete {
			fmt.Printf("🔥 Permanently deleted: %s (%s)\n", targetNote.Title, targetNote.ID)
		} else {
			fmt.Printf("🗑️  Moved to trash (soft deleted): %s (%s)\n", targetNote.Title, targetNote.ID)
		}
	},
}

func init() {
	rootCmd.AddCommand(noteCmd)
	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteShowCmd, noteExportCmd, noteRmCmd)

	noteAddCmd.Flags().StringVar(&notePath, "path", "/", "Virtual path (e.g., /home/solar)")
	noteAddCmd.Flags().StringVar(&noteFile, "file", "", "Path to a local .md file to read content from")
	noteRmCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively delete folders and their contents")
	noteRmCmd.Flags().BoolVar(&hardDelete, "hard", false, "Permanently delete the item from the database (cannot be undone)")

	noteExportCmd.Flags().StringVar(&exportPath, "out", ".", "Local directory to export the markdown file to")
}
