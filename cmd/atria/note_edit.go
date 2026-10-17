package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/notes"
)

var (
	noteEditFile  string
	noteEditTitle string
	noteEditPath  string
)

var noteEditCmd = &cobra.Command{
	Use:   "edit <uuid|short-uuid|\"Title\">",
	Short: "Edits a note's content, title or folder",
	Long: `Edits a note. The new content comes from --file (or stdin with --file -);
with neither --file, --title nor --path the note opens in $EDITOR.

Examples:
  atria note edit "Shopping"                       # opens $EDITOR
  atria note edit "Shopping" --file shopping.md
  atria note edit "Shopping" --title "Groceries" --path /home`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeNote, args[0], false)
		if err != nil {
			return err
		}
		note, err := notes.GetNote(app.Ctx, app.DB, entity.ID, app.Owner.ID)
		if err != nil {
			return err
		}

		title, path, content := note.Title, note.Path, note.MarkdownContent
		if cmd.Flags().Changed("title") {
			title = noteEditTitle
		}
		if cmd.Flags().Changed("path") {
			path = noteEditPath
		}

		switch {
		case noteEditFile == "-":
			b, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			content = string(b)
		case noteEditFile != "":
			b, err := os.ReadFile(noteEditFile)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}
			content = string(b)
		case !cmd.Flags().Changed("title") && !cmd.Flags().Changed("path"):
			if content, err = editInEditor(content); err != nil {
				return err
			}
		}

		if title == note.Title && path == note.Path && content == note.MarkdownContent {
			fmt.Fprintln(cmd.OutOrStdout(), "No changes.")
			return nil
		}
		if err := notes.UpdateNote(app.Ctx, app.DB, app.Owner.ID, note.ID, title, path, content); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Note %q updated\n", title)
		return nil
	},
}

// editInEditor opens content in $EDITOR (default vi) and returns the result.
func editInEditor(content string) (string, error) {
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) == 0 {
		editor = []string{"vi"}
	}

	f, err := os.CreateTemp("", "atria-note-*.md")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	c := exec.Command(editor[0], append(editor[1:], f.Name())...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("editor %q failed: %w", editor[0], err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(b)) == "" && strings.TrimSpace(content) != "" {
		return "", errors.New("the edited note is empty; nothing saved (use --file to really clear it)")
	}
	return string(b), nil
}

func init() {
	noteCmd.AddCommand(noteEditCmd, newRenameCmd(core.TypeNote), newVisibilityCmd(core.TypeNote, "/public/notes"))
	noteEditCmd.Flags().StringVar(&noteEditFile, "file", "", "Read the new content from a file ('-' for stdin)")
	noteEditCmd.Flags().StringVar(&noteEditTitle, "title", "", "New title")
	noteEditCmd.Flags().StringVar(&noteEditPath, "path", "", "New folder path, e.g. /home/projects")
}
