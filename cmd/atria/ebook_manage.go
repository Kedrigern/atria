package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/ebooks"
)

var (
	ebookEditTitle       string
	ebookEditAuthor      string
	ebookEditDescription string
	ebookEditLanguage    string
)

var ebookEditCmd = &cobra.Command{
	Use:   "edit <identifier>",
	Short: "Edits e-book metadata (only the given fields change)",
	Example: `  atria ebook edit "Weekend reading" --author "Me" --language cs
  atria ebook edit "Weekend reading" --description ""   # clears the description`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeEbook, args[0], false)
		if err != nil {
			return err
		}
		detail, err := ebooks.GetDetail(app.Ctx, app.DB, app.Owner.ID, entity.ID)
		if err != nil {
			return err
		}

		deref := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}
		title, author, description, language := detail.Title, deref(detail.Author), deref(detail.Description), deref(detail.Language)
		changed := false
		for flag, target := range map[string]*string{"title": &title, "author": &author, "description": &description, "language": &language} {
			if cmd.Flags().Changed(flag) {
				value, _ := cmd.Flags().GetString(flag)
				*target = value
				changed = true
			}
		}
		if !changed {
			return fmt.Errorf("nothing to change: use --title, --author, --description or --language")
		}

		if err := ebooks.UpdateMetadata(app.Ctx, app.DB, app.Owner.ID, entity.ID, title, description, author, language); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ E-book %q updated\n", title)
		return nil
	},
}

var ebookRmCmd = &cobra.Command{
	Use:   "rm <identifier>",
	Short: "Moves an e-book to the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeEbook, args[0], false)
		if err != nil {
			return err
		}
		if err := ebooks.Delete(app.Ctx, app.DB, app.Owner.ID, entity.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Moved %q to the trash\n", entity.Title)
		return nil
	},
}

var ebookCoverCmd = &cobra.Command{
	Use:   "cover <identifier> <image-file>",
	Short: "Replaces the cover of an e-book, in Atria and in the EPUB file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeEbook, args[0], false)
		if err != nil {
			return err
		}
		if err := ebooks.ReplaceCover(app.Ctx, app.DB, app.Owner.ID, entity.ID, args[1], filepath.Base(args[1])); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Cover of %q replaced.\n", entity.Title)
		return nil
	},
}

var ebookReplaceCmd = &cobra.Command{
	Use:   "replace <identifier> <epub-file>",
	Short: "Replaces the EPUB file of an e-book (title, author and cover come from the new file)",
	Long: `Replaces the EPUB file of an e-book, for example with a better edition.
Atria takes the title, author, description, language and cover from the new
file. Where the new file has no value, Atria keeps its own value and writes it
into the file. The links to notes and articles stay.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeEbook, args[0], false)
		if err != nil {
			return err
		}
		if err := ebooks.ReplaceFile(app.Ctx, app.DB, app.Owner.ID, entity.ID, args[1], filepath.Base(args[1])); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "EPUB file of %q replaced.\n", entity.Title)
		return nil
	},
}

func init() {
	ebookCmd.AddCommand(ebookEditCmd, ebookRmCmd, ebookCoverCmd, ebookReplaceCmd)
	ebookEditCmd.Flags().StringVar(&ebookEditTitle, "title", "", "New title")
	ebookEditCmd.Flags().StringVar(&ebookEditAuthor, "author", "", "Author (\"\" clears)")
	ebookEditCmd.Flags().StringVar(&ebookEditDescription, "description", "", "Description (\"\" clears)")
	ebookEditCmd.Flags().StringVar(&ebookEditLanguage, "language", "", "Language code, e.g. cs or en (\"\" clears)")
}
