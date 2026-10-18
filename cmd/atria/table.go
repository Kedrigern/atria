package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"atria/internal/cli"
	"atria/internal/core"
	"atria/internal/tables"
)

var (
	tableName    string
	tableTitle   string
	tableColumns string
	tableOut     string
)

var tableCmd = &cobra.Command{
	Use:               "table",
	Short:             "Data tables: create, import CSV, show and export",
	PersistentPreRunE: RequireUserContext,
}

// resolveTable accepts a title, short ID, full UUID or the table's SQL name.
func resolveTable(identifier string) (*tables.Table, error) {
	if t, err := tables.FindBySQLName(app.Ctx, app.DB, app.Owner.ID, identifier); err == nil {
		return t, nil
	}
	entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeSpreadsheet, identifier, false)
	if err != nil {
		return nil, err
	}
	return tables.Get(app.Ctx, app.DB, app.Owner.ID, entity.ID)
}

var tableListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists your tables",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := tables.List(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return err
		}
		headers := []string{"ID", "NAME", "TITLE", "COLUMNS", "ROWS", "UPDATED"}
		rows := make([][]string, 0, len(list))
		for _, t := range list {
			rows = append(rows, []string{FormatID(t.ID, showLong), t.SQLName, t.Title,
				strconv.Itoa(len(t.Columns)), strconv.Itoa(t.RowCount), t.UpdatedAt.Format("2006-01-02 15:04")})
		}
		return cli.Render(cmd.OutOrStdout(), listFormat, headers, rows, list)
	},
}

var tableCreateCmd = &cobra.Command{
	Use:   "create <title> --columns <spec>",
	Short: "Creates an empty table",
	Long: `Creates an empty table. --columns is a comma-separated list of
name:type, where type is text, number, number:<decimals>, date, checkbox, url
or select=<option>|<option>|...

Example:
  atria table create "Expenses" --columns "item:text,price:number:2,bought:date,category:select=food|home|fun,paid:checkbox"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		columns, err := parseColumnSpec(tableColumns)
		if err != nil {
			return err
		}
		name := tableName
		if name == "" {
			name = tables.Identifier(args[0], "table")
		}
		t, err := tables.Create(app.Ctx, app.DB, app.Owner.ID, args[0], name, columns)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Created table %q (name: %s, ID: %s)\n", t.Title, t.SQLName, ShortID(t.ID))
		return nil
	},
}

// parseColumnSpec parses "item:text,price:number:2,category:select=a|b".
func parseColumnSpec(spec string) ([]tables.Column, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, fmt.Errorf("--columns is required, e.g. \"item:text,price:number:2\"")
	}
	var columns []tables.Column
	for i, part := range strings.Split(spec, ",") {
		name, typ, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			return nil, fmt.Errorf("column %q: use name:type", part)
		}
		c := tables.Column{ID: fmt.Sprintf("c%d", i+1), Name: strings.TrimSpace(name)}
		switch {
		case strings.HasPrefix(typ, "select="):
			c.Type = tables.TypeSelect
			c.Options = strings.Split(strings.TrimPrefix(typ, "select="), "|")
		case strings.HasPrefix(typ, "number:"):
			d, err := strconv.Atoi(strings.TrimPrefix(typ, "number:"))
			if err != nil {
				return nil, fmt.Errorf("column %q: decimals must be a number", name)
			}
			c.Type, c.Decimals = tables.TypeNumber, &d
		default:
			c.Type = tables.ColumnType(typ)
		}
		columns = append(columns, c)
	}
	return columns, nil
}

var tableImportCmd = &cobra.Command{
	Use:   "import <file.csv|->",
	Short: "Creates a table from a CSV file (types detected automatically)",
	Long: `Creates a new table from CSV. The first line is the header. The delimiter
(comma, semicolon or tab) and column types (number, date, checkbox, url, text)
are detected; Czech formats such as "1 234,50" and "31.12.2026" work.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := cmd.InOrStdin()
		title := tableTitle
		if args[0] != "-" {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			r = f
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
			}
		}
		if title == "" {
			return fmt.Errorf("--title is required when reading stdin")
		}
		res, err := tables.ImportCSV(app.Ctx, app.DB, app.Owner.ID, title, tableName, r)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "✅ Imported %d rows into %q (name: %s, ID: %s)\n", res.Rows, res.Table.Title, res.Table.SQLName, ShortID(res.Table.ID))
		for _, c := range res.Table.Columns {
			fmt.Fprintf(out, "   %-24s %s\n", c.Name, describeType(c))
		}
		return nil
	},
}

func describeType(c tables.Column) string {
	switch {
	case c.Type == tables.TypeNumber && c.Decimals != nil:
		return fmt.Sprintf("number (%d decimals)", *c.Decimals)
	case c.Type == tables.TypeSelect:
		return "select: " + strings.Join(c.Options, ", ")
	}
	return string(c.Type)
}

var tableShowCmd = &cobra.Command{
	Use:   "show <table>",
	Short: "Prints a table (table, csv, md or json)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := resolveTable(args[0])
		if err != nil {
			return err
		}
		rows, err := tables.Rows(app.Ctx, app.DB, app.Owner.ID, t.ID, 0, 0)
		if err != nil {
			return err
		}
		return writeTable(cmd.OutOrStdout(), listFormat, t, rows)
	},
}

func writeTable(w io.Writer, format string, t *tables.Table, rows []tables.Row) error {
	switch format {
	case "csv":
		return tables.WriteCSV(w, t, rows)
	case "md", "markdown":
		return tables.WriteMarkdown(w, t, rows)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			*tables.Table
			Rows []tables.Row `json:"rows"`
		}{t, rows})
	case "table", "":
		headers := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			headers[i] = strings.ToUpper(c.Header())
		}
		out := make([][]string, len(rows))
		for i, r := range rows {
			out[i] = make([]string, len(t.Columns))
			for j, c := range t.Columns {
				out[i][j] = tables.FormatValue(c, r.Data[c.ID])
			}
		}
		return cli.Render(w, "table", headers, out, nil)
	}
	return fmt.Errorf("unknown format %q (use table, csv, md or json)", format)
}

var tableExportCmd = &cobra.Command{
	Use:   "export <table> -o <file.csv|file.md|file.json>",
	Short: "Exports a table to a file (format from the extension)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if tableOut == "" {
			return fmt.Errorf("-o <file> is required")
		}
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(tableOut)), ".")
		if format != "csv" && format != "md" && format != "json" {
			return fmt.Errorf("unsupported extension %q (use .csv, .md or .json)", filepath.Ext(tableOut))
		}
		t, err := resolveTable(args[0])
		if err != nil {
			return err
		}
		rows, err := tables.Rows(app.Ctx, app.DB, app.Owner.ID, t.ID, 0, 0)
		if err != nil {
			return err
		}
		f, err := os.Create(tableOut)
		if err != nil {
			return err
		}
		if err := writeTable(f, format, t, rows); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Exported %d rows to %s\n", len(rows), tableOut)
		return nil
	},
}

var tableAddCmd = &cobra.Command{
	Use:   "add <table> <column=value>...",
	Short: "Appends one row, e.g. atria table add expenses item=milk price=24,90",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := resolveTable(args[0])
		if err != nil {
			return err
		}
		byName := map[string]tables.Column{}
		for _, c := range t.Columns {
			byName[c.Name] = c
		}
		values := map[string]any{}
		for _, kv := range args[1:] {
			name, value, ok := strings.Cut(kv, "=")
			c, known := byName[name]
			if !ok || !known {
				return fmt.Errorf("%q: use <column>=<value> with one of the column names", kv)
			}
			values[c.ID] = value
		}
		if _, err := tables.InsertRows(app.Ctx, app.DB, app.Owner.ID, t.ID, []map[string]any{values}); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Added a row to %q (%d rows)\n", t.Title, t.RowCount+1)
		return nil
	},
}

var tableRmCmd = &cobra.Command{
	Use:   "rm <table>",
	Short: "Moves a table to the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := resolveTable(args[0])
		if err != nil {
			return err
		}
		if err := core.SoftDeleteEntity(app.Ctx, app.DB, app.Owner.ID, t.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Moved %q to the trash\n", t.Title)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tableCmd)
	tableCmd.AddCommand(tableListCmd, tableCreateCmd, tableImportCmd, tableShowCmd, tableExportCmd,
		tableAddCmd, tableRmCmd, newRenameCmd(core.TypeSpreadsheet))

	tableListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	tableListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")
	tableCreateCmd.Flags().StringVar(&tableColumns, "columns", "", "Column spec, e.g. \"item:text,price:number:2\"")
	tableCreateCmd.Flags().StringVar(&tableName, "name", "", "SQL name used in views (default: from the title)")
	tableImportCmd.Flags().StringVar(&tableTitle, "title", "", "Table title (default: file name)")
	tableImportCmd.Flags().StringVar(&tableName, "name", "", "SQL name used in views (default: from the title)")
	tableShowCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, csv, md, json)")
	tableExportCmd.Flags().StringVarP(&tableOut, "output", "o", "", "Output file (.csv, .md or .json)")
}
