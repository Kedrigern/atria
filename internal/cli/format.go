package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Render outputs the given data in the specified format (table, json, csv, html).
// - headers: Column names for table, csv, and html formats.
// - rows: String matrix containing the formatted data for table, csv, and html formats.
// - rawData: The original struct slice used for JSON marshaling.
func Render(w io.Writer, format string, headers []string, rows [][]string, rawData any) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(rawData)

	case "csv":
		writer := csv.NewWriter(w)
		if err := writer.Write(headers); err != nil {
			return fmt.Errorf("failed to write csv headers: %w", err)
		}
		if err := writer.WriteAll(rows); err != nil {
			return fmt.Errorf("failed to write csv rows: %w", err)
		}
		writer.Flush()
		return writer.Error()

	case "html":
		fmt.Fprintln(w, "<table>")
		fmt.Fprintln(w, "  <thead>")
		fmt.Fprintln(w, "    <tr>")
		for _, h := range headers {
			fmt.Fprintf(w, "      <th>%s</th>", h)
		}
		fmt.Fprintln(w, "\n    </tr>")
		fmt.Fprintln(w, "  </thead>")
		fmt.Fprintln(w, "  <tbody>")
		for _, row := range rows {
			fmt.Fprintln(w, "    <tr>")
			for _, col := range row {
				// Basic HTML escaping could be added here, but for simple CLI output this is sufficient
				fmt.Fprintf(w, "      <td>%s</td>\n", col)
			}
			fmt.Fprintln(w, "    </tr>")
		}
		fmt.Fprintln(w, "  </tbody>")
		fmt.Fprintln(w, "</table>")
		return nil

	case "table":
		fallthrough
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
		for _, row := range rows {
			fmt.Fprintln(tw, strings.Join(row, "\t"))
		}
		tw.Flush()
		return nil
	}
}
