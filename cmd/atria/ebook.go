package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"atria/internal/cli"
	"atria/internal/core"
	"atria/internal/ebooks"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"
)

var (
	ebookTitle       string
	ebookDescription string
	ebookAuthor      string
	ebookLanguage    string
	ebookCoverPath   string
	ebookFilename    string
	ebookDownloadOut string
)

var ebookCmd = &cobra.Command{
	Use:               "ebook",
	Short:             "Create and manage immutable e-book snapshots",
	PersistentPreRunE: RequireUserContext,
}

var ebookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List generated e-books",
	RunE: func(cmd *cobra.Command, args []string) error {
		ebooks, err := ebooks.List(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return err
		}
		if len(ebooks) == 0 {
			fmt.Println("No e-books found. Use 'atria ebook create' to create one.")
			return nil
		}

		headers := []string{"ID", "TITLE", "AUTHOR", "LANGUAGE", "CHAPTERS", "FILE", "GENERATED"}
		rows := make([][]string, 0, len(ebooks))
		for _, ebook := range ebooks {
			rows = append(rows, []string{
				FormatID(ebook.ID, showLong),
				ebook.Title,
				optionalString(ebook.Author),
				optionalString(ebook.Language),
				fmt.Sprintf("%d", ebook.SourceCount),
				ebook.Filename,
				ebook.GeneratedAt.Format("2006-01-02 15:04"),
			})
		}
		return cli.Render(os.Stdout, listFormat, headers, rows, ebooks)
	},
}

var ebookDownloadCmd = &cobra.Command{
	Use:   "download <identifier>",
	Short: "Download an e-book EPUB to a local file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeEbook, args[0], false)
		if err != nil {
			return err
		}
		download, err := ebooks.GetDownload(app.Ctx, app.DB, app.Owner.ID, entity.ID)
		if err != nil {
			return err
		}
		if err := copyEbookFile(download.DiskPath, ebookDownloadOut); err != nil {
			return err
		}
		fmt.Printf("✅ Downloaded %s to %s\n", download.Filename, ebookDownloadOut)
		return nil
	},
}

var ebookCreateCmd = &cobra.Command{
	Use:   "create <identifier1> [identifier2...]",
	Short: "Create an EPUB snapshot from articles and notes",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		items, _ := resolveEbookSources(args)
		if len(items) == 0 {
			return fmt.Errorf("no valid articles or notes found for the e-book")
		}

		fmt.Printf("📚 Generating EPUB with %d chapters...\n", len(items))
		ebook, err := ebooks.Generate(app.Ctx, app.DB, app.Owner.ID, ebooks.GenerateInput{
			Title:       ebookTitle,
			Description: ebookDescription,
			Author:      ebookAuthor,
			Language:    ebookLanguage,
			CoverPath:   ebookCoverPath,
			CoverName:   filepath.Base(ebookCoverPath),
			Filename:    ebookFilename,
			Sources:     items,
		})
		if err != nil {
			return err
		}

		fmt.Printf("✅ E-book created\nID:   %s\n", FormatID(ebook.ID, showLong))
		return nil
	},
}

func resolveEbookSources(args []string) ([]core.EntitySummary, []uuid.UUID) {
	items := make([]core.EntitySummary, 0, len(args))
	sourceIDs := make([]uuid.UUID, 0, len(args))
	for _, identifier := range args {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", identifier, false)
		if err != nil {
			fmt.Printf("⚠️ Skipping %q: %v\n", identifier, err)
			continue
		}
		if entity.Type != core.TypeArticle && entity.Type != core.TypeNote {
			fmt.Printf("⏭️ Skipping %q: only articles and notes are supported\n", entity.Title)
			continue
		}
		items = append(items, *entity)
		sourceIDs = append(sourceIDs, entity.ID)
	}
	return items, sourceIDs
}

func copyEbookFile(diskPath, destination string) error {
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./data/attachments"
	}

	source, err := os.Open(filepath.Join(storagePath, diskPath))
	if err != nil {
		return fmt.Errorf("open stored EPUB: %w", err)
	}
	defer source.Close()

	target, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("copy EPUB: %w", err)
	}
	return nil
}

func optionalString(value *string) string {
	if value == nil || *value == "" {
		return "-"
	}
	return *value
}

func ebookOutputFilename(title, filename string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." {
		filename = strings.ToLower(strings.Join(strings.Fields(title), "-"))
		filename = strings.NewReplacer("/", "-", "\\", "-").Replace(filename)
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".epub") {
		filename += ".epub"
	}
	return filename
}

func init() {
	rootCmd.AddCommand(ebookCmd)
	ebookCmd.AddCommand(ebookCreateCmd, ebookListCmd, ebookDownloadCmd)

	ebookCreateCmd.Flags().StringVar(&ebookTitle, "title", "", "E-book title")
	ebookCreateCmd.Flags().StringVar(&ebookDescription, "description", "", "E-book description")
	ebookCreateCmd.Flags().StringVar(&ebookAuthor, "author", "", "E-book author or publisher")
	ebookCreateCmd.Flags().StringVar(&ebookLanguage, "language", "", "EPUB language code, for example cs or en")
	ebookCreateCmd.Flags().StringVar(&ebookCoverPath, "cover", "", "Path to a local cover image")
	ebookCreateCmd.Flags().StringVar(&ebookFilename, "filename", "", "Filename of the stored EPUB attachment")
	ebookDownloadCmd.Flags().StringVarP(&ebookDownloadOut, "out", "o", "", "Destination EPUB path")
	_ = ebookCreateCmd.MarkFlagRequired("title")
	_ = ebookDownloadCmd.MarkFlagRequired("out")
}
