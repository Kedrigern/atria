package main

import (
	"fmt"
	"os"

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

		attachment, err := attachments.AddAttachment(app.Ctx, app.DB, app.Owner.ID, localPath)
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

		// 1. Vyhledání přílohy (nově s podporou krátkého UUID nebo jména souboru)
		targetAtt, err := resolveAttachment(app.Ctx, app.DB, app.Owner.ID, attIdentifier)
		if err != nil {
			return err
		}

		// 2. Vyhledání cílové entity
		targetEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", entityIdentifier, false)
		if err != nil {
			return err
		}

		// 3. Propojení v databázi
		err = attachments.LinkAttachment(app.Ctx, app.DB, targetEntity.ID, targetAtt.ID)
		if err != nil {
			return fmt.Errorf("failed to link attachment: %w", err)
		}

		fmt.Printf("🔗 Attachment %s (%s) successfully linked to %s '%s'\n", targetAtt.Filename, ShortID(targetAtt.ID), targetEntity.Type, targetEntity.Title)
		return nil
	},
}

// resolveAttachment prohledá přílohy podle plného UUID, krátkého UUID, nebo přesného názvu souboru.
func resolveAttachment(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string) (*core.Attachment, error) {
	var query string
	var args []interface{}

	if parsedID, err := core.ParseUUID(identifier); err == nil {
		query = `SELECT id, filename FROM attachments WHERE id = $1 AND owner_id = $2`
		args = []interface{}{parsedID, ownerID}
	} else {
		// Hledáme podle konce stringu (short-uuid) nebo názvu souboru
		query = `SELECT id, filename FROM attachments WHERE owner_id = $1 AND (id::text LIKE $2 OR filename = $3)`
		args = []interface{}{ownerID, "%" + identifier, identifier}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("attachment search failed: %w", err)
	}
	defer rows.Close()

	var results []core.Attachment
	for rows.Next() {
		var a core.Attachment
		if err := rows.Scan(&a.ID, &a.Filename); err != nil {
			return nil, err
		}
		results = append(results, a)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no attachment found matching '%s'", identifier)
	}
	if len(results) > 1 {
		errMsg := fmt.Sprintf("Ambiguous attachment identifier '%s'. Please be more specific:\n", identifier)
		for _, r := range results {
			errMsg += fmt.Sprintf("  %s  %s\n", ShortID(r.ID), r.Filename)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return &results[0], nil
}

func init() {
	rootCmd.AddCommand(attachmentCmd)
	attachmentCmd.AddCommand(attachmentAddCmd, attachmentListCmd, attachmentLinkCmd)

	attachmentAddCmd.Flags().StringVarP(&attachEntity, "link", "k", "", "Optionally link to an entity (UUID or Title) immediately after upload")

	attachmentListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	attachmentListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")
}
