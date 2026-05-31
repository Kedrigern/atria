package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"atria/internal/attachments"
	"atria/internal/cli"
	"atria/internal/core"
	"context"
	"database/sql"
)

var attachEntity string

var attachmentCmd = &cobra.Command{
	Use:               "attachment",
	Aliases:           []string{"att"},
	Short:             "File uploads and attachment management",
	PersistentPreRunE: RequireUserContext,
}

var attachmentAddCmd = &cobra.Command{
	Use:   "add <local-file-path>",
	Short: "Uploads a file, deduplicates it, and stores it in Atria",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		localPath := args[0]

		attachment, err := attachments.AddAttachment(app.Ctx, app.DB, app.Owner.ID, localPath, filepath.Base(localPath))
		if err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}

		fmt.Printf("✅ Attachment stored successfully!\nID: %s\nFile: %s\nPath: %s\n", attachment.ID, attachment.Filename, attachment.DiskPath)

		if attachEntity != "" {
			targetEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", attachEntity, false)
			if err != nil {
				return fmt.Errorf("attachment stored, but linking failed: %w", err)
			}

			err = attachments.LinkAttachment(app.Ctx, app.DB, targetEntity.ID, attachment.ID)
			if err != nil {
				return fmt.Errorf("attachment stored, but linking failed: %w", err)
			}
			fmt.Printf("🔗 Linked to entity: %s\n", targetEntity.Title)
		}

		return nil
	},
}

var attachmentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all uploaded attachments",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := attachments.ListAttachments(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to list attachments: %w", err)
		}

		headers := []string{"ID", "FILENAME", "MIME TYPE", "SIZE (KB)", "CREATED"}
		var rows [][]string
		for _, a := range list {
			rows = append(rows, []string{
				FormatID(a.ID, showLong),
				a.Filename,
				a.MimeType,
				fmt.Sprintf("%d", a.SizeBytes/1024),
				a.CreatedAt.Format("2006-01-02 15:04"),
			})
		}

		return cli.Render(os.Stdout, listFormat, headers, rows, list)
	},
}

var attachmentLinkCmd = &cobra.Command{
	Use:   "link <attachment-uuid> <entity-uuid|short-uuid|\"Title\">",
	Short: "Links an existing attachment to an entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		attIdentifier, entityIdentifier := args[0], args[1]

		// 1. Resolve the attachment (supports short UUID or filename).
		targetAtt, err := resolveAttachment(app.Ctx, app.DB, app.Owner.ID, attIdentifier)
		if err != nil {
			return err
		}

		// 2. Resolve the target entity.
		targetEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", entityIdentifier, false)
		if err != nil {
			return err
		}

		// 3. Create the link in the database.
		err = attachments.LinkAttachment(app.Ctx, app.DB, targetEntity.ID, targetAtt.ID)
		if err != nil {
			return fmt.Errorf("failed to link attachment: %w", err)
		}

		fmt.Printf("🔗 Attachment %s (%s) successfully linked to %s '%s'\n", targetAtt.Filename, ShortID(targetAtt.ID), targetEntity.Type, targetEntity.Title)
		return nil
	},
}

// resolveAttachment search attachments by UUID, short UUID, or filename
func resolveAttachment(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string) (*core.Attachment, error) {

	items, err := attachments.FindAttachments(ctx, db, ownerID, identifier)

	return resolveSingle(identifier, items, err, "attachment", func(a core.Attachment) string {
		return fmt.Sprintf("%s  %s", ShortID(a.ID), a.Filename)
	})
}

func init() {
	rootCmd.AddCommand(attachmentCmd)
	attachmentCmd.AddCommand(attachmentAddCmd, attachmentListCmd, attachmentLinkCmd)

	attachmentAddCmd.Flags().StringVarP(&attachEntity, "link", "k", "", "Optionally link to an entity (UUID or Title) immediately after upload")

	attachmentListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	attachmentListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")
}
