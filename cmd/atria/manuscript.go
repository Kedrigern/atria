package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"atria/internal/cli"
	"atria/internal/core"
	"atria/internal/manuscripts"
	"atria/internal/mdcheck"
)

var (
	manuscriptPath    string
	manuscriptReplace bool
	manuscriptFormat  string
	manuscriptOut     string
)

var manuscriptCmd = &cobra.Command{
	Use:               "manuscript",
	Short:             "Store and manage books as Markdown files",
	PersistentPreRunE: RequireUserContext,
}

var manuscriptImportCmd = &cobra.Command{
	Use:   "import FILE...",
	Short: "Imports Markdown files as manuscripts and prints the check of each file",
	Long: `Imports Markdown files as manuscripts, one book per file. The file is kept
exactly as it is. The title comes from the front matter, then from the first
# heading, then from the file name. The author and the language come from the
front matter (author, lang).

Each file gets the same check as 'atria check'. Warnings do not stop the
import. A manuscript with the same title in the same folder stops the import
of that file; with --replace, the new file replaces its content and keeps its
ID, tags and links. The other files are imported.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		failed := 0
		for _, file := range args {
			src, err := os.ReadFile(file)
			if err != nil {
				fmt.Fprintf(out, "failed   %s: %v\n", file, err)
				failed++
				continue
			}
			m, state, err := manuscripts.Import(app.Ctx, app.DB, app.Owner.ID, manuscriptPath, manuscripts.FromFile(file, src), manuscriptReplace)
			if err != nil {
				fmt.Fprintf(out, "failed   %s: %v\n", file, err)
				failed++
				continue
			}
			fmt.Fprintf(out, "%-8s %s  %s\n", state, ShortID(m.ID), m.Title)
			printCheckReport(out, file, mdcheck.Check(src))
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d files were not imported", failed, len(args))
		}
		return nil
	},
}

var manuscriptListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists manuscripts",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := manuscripts.List(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return err
		}
		headers := []string{"ID", "TITLE", "PATH", "AUTHOR", "LANGUAGE", "STANDARD PAGES"}
		rows := make([][]string, 0, len(list))
		for _, m := range list {
			rows = append(rows, []string{
				FormatID(m.ID, showLong), m.Title, m.Path, m.Author, m.Language,
				fmt.Sprintf("%.1f", float64(m.Characters)/mdcheck.StandardPageChars),
			})
		}
		return cli.Render(cmd.OutOrStdout(), listFormat, headers, rows, list)
	},
}

var manuscriptShowCmd = &cobra.Command{
	Use:   "show <uuid|short-uuid|\"Title\">",
	Short: "Prints the Markdown file of a manuscript",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeManuscript, args[0], false)
		if err != nil {
			return err
		}
		m, err := manuscripts.Get(app.Ctx, app.DB, app.Owner.ID, entity.ID)
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), m.Markdown)
		return nil
	},
}

var manuscriptRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid|\"Title\">",
	Short: "Moves a manuscript to the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeManuscript, args[0], false)
		if err != nil {
			return err
		}
		if err := core.SoftDeleteEntity(app.Ctx, app.DB, app.Owner.ID, entity.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Moved %q to the trash\n", entity.Title)
		return nil
	},
}

var manuscriptExportCmd = &cobra.Command{
	Use:   "export <uuid|short-uuid|\"Title\"> [--format=epub|docx|pdf|md] [--out=FILE]",
	Short: "Writes a manuscript as EPUB, DOCX, PDF or Markdown",
	Long: `Writes a manuscript to a file. EPUB has the page numbers of the original
as a page list for e-book readers. DOCX and PDF need pandoc (PDF also typst)
and show the page numbers of the original in brackets, for example [15].
The default file name is the slug of the title with the format extension.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeManuscript, args[0], false)
		if err != nil {
			return err
		}
		m, err := manuscripts.Get(app.Ctx, app.DB, app.Owner.ID, entity.ID)
		if err != nil {
			return err
		}
		out := manuscriptOut
		if out == "" {
			out = m.Slug + "." + manuscriptFormat
		}
		switch manuscriptFormat {
		case "md":
			err = os.WriteFile(out, []byte(m.Markdown), 0o644)
		case "epub":
			var pages int
			pages, err = manuscripts.WriteEPUB(m, out)
			if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%d pages of the original in the page list\n", pages)
			}
		case manuscripts.FormatDOCX, manuscripts.FormatPDF:
			if !slices.Contains(manuscripts.Formats(), manuscriptFormat) {
				return fmt.Errorf("%s needs pandoc on this computer, and PDF also typst", manuscriptFormat)
			}
			err = manuscripts.Convert(app.Ctx, m, manuscriptFormat, out)
		default:
			return fmt.Errorf("unknown format %q (use epub, docx, pdf or md)", manuscriptFormat)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", out)
		return nil
	},
}

var manuscriptPublishCmd = &cobra.Command{
	Use:   "publish <uuid|short-uuid|\"Title\">",
	Short: "Makes an e-book from a manuscript",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeManuscript, args[0], false)
		if err != nil {
			return err
		}
		m, err := manuscripts.Get(app.Ctx, app.DB, app.Owner.ID, entity.ID)
		if err != nil {
			return err
		}
		book, err := manuscripts.Publish(app.Ctx, app.DB, app.Owner.ID, m)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ E-book %s  %s\n", ShortID(book.ID), book.Title)
		return nil
	},
}

func init() {
	manuscriptImportCmd.Flags().StringVarP(&manuscriptPath, "path", "p", "/", "folder of the manuscripts, for example /books")
	manuscriptImportCmd.Flags().BoolVar(&manuscriptReplace, "replace", false, "replace a manuscript with the same title in the folder")
	manuscriptListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "output format: table, csv or json")
	manuscriptExportCmd.Flags().StringVarP(&manuscriptFormat, "format", "f", "epub", "epub, docx, pdf or md")
	manuscriptExportCmd.Flags().StringVarP(&manuscriptOut, "out", "o", "", "output file (default: <slug>.<format>)")
	manuscriptCmd.AddCommand(manuscriptImportCmd, manuscriptListCmd, manuscriptShowCmd, manuscriptRmCmd, newRenameCmd(core.TypeManuscript),
		manuscriptExportCmd, manuscriptPublishCmd)
	rootCmd.AddCommand(manuscriptCmd)
}
