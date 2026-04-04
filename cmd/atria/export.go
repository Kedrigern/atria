package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/export"
)

var exportOutPath string
var exportSaveAsAttachment bool

var exportCmd = &cobra.Command{
	Use:               "export",
	Short:             "Bulk export functionalities",
	PersistentPreRunE: RequireUserContext,
}

var exportEpubCmd = &cobra.Command{
	Use:   "epub <identifier1> [identifier2...]",
	Short: "Bundles specified articles and notes into an EPUB file",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var itemsToExport []core.EntitySummary

		// 1. Resolve all requested entities
		for _, arg := range args {
			entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", arg, false)
			if err != nil {
				fmt.Printf("⚠️ Could not resolve '%s': %v\n", arg, err)
				continue
			}

			if entity.Type != core.TypeArticle && entity.Type != core.TypeNote {
				fmt.Printf("⏭️  Skipping '%s': EPUB export only supports articles and notes (got %s)\n", entity.Title, entity.Type)
				continue
			}

			itemsToExport = append(itemsToExport, *entity)
		}

		if len(itemsToExport) == 0 {
			return fmt.Errorf("no valid articles or notes found to export")
		}

		// 2. Prepare temporary path if saving as attachment
		finalOutPath := exportOutPath
		if exportSaveAsAttachment {
			tempDir, err := os.MkdirTemp("", "atria-export-*")
			if err != nil {
				return fmt.Errorf("failed to create temp dir: %w", err)
			}
			defer os.RemoveAll(tempDir)
			safeName := "atria-export.epub"
			if len(itemsToExport) > 0 {
				safeName = fmt.Sprintf("Atria_Export_%s.epub", itemsToExport[0].Title)
			}
			finalOutPath = filepath.Join(tempDir, safeName)
		}

		// 3. Generate EPUB
		fmt.Printf("📚 Generating EPUB with %d items...\n", len(itemsToExport))
		err := export.ExportEPUB(app.Ctx, app.DB, itemsToExport, finalOutPath)
		if err != nil {
			return fmt.Errorf("EPUB generation failed: %w", err)
		}

		// 4. Handle attachment storage
		if exportSaveAsAttachment {
			// Use our existing attachment service to hash, move and store the record
			att, err := attachments.AddAttachment(app.Ctx, app.DB, app.Owner.ID, finalOutPath)
			if err != nil {
				return fmt.Errorf("failed to store export as attachment: %w", err)
			}

			// Override original filename in DB to be more descriptive than "export.epub"
			// (Optional: could be based on date or first item title)
			fmt.Printf("✅ Export stored as attachment!\nID:   %s\nFile: %s\nHash: %s\n",
				FormatID(att.ID, showLong), att.Filename, att.FileHash)
		} else {
			fmt.Printf("✅ Successfully exported to: %s\n", finalOutPath)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.AddCommand(exportEpubCmd)

	exportEpubCmd.Flags().StringVarP(&exportOutPath, "out", "o", "atria-export.epub", "Path to save the generated EPUB file (ignored if --attach is used)")
	exportEpubCmd.Flags().BoolVarP(&exportSaveAsAttachment, "attach", "a", true, "Automatically store the generated EPUB as a system attachment")
}
