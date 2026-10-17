package tables

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"unicode"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
	"golang.org/x/text/unicode/norm"
)

// maxImportBytes caps a CSV upload.
const maxImportBytes = MaxTableBytes

// ImportResult describes a finished CSV import.
type ImportResult struct {
	Table *Table
	Rows  int
}

// ImportCSV creates a new table from CSV data. The first record is the
// header; the delimiter (comma, semicolon or tab) and column types are
// detected from the data. sqlName may be empty to derive it from the title.
func ImportCSV(ctx context.Context, db *sql.DB, ownerID uuid.UUID, title, sqlName string, r io.Reader) (*ImportResult, error) {
	columns, records, err := parseCSV(r)
	if err != nil {
		return nil, err
	}
	if sqlName == "" {
		sqlName = Identifier(title, "table")
	}
	if err := ValidateIdentifier("table", sqlName); err != nil {
		return nil, err
	}
	if len(records) > MaxRows {
		return nil, core.NewUserError("the CSV has %d rows; a table can have at most %d", len(records), MaxRows)
	}

	values := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		row := map[string]any{}
		for i, c := range columns {
			if i < len(rec) {
				row[c.ID] = rec[i]
			}
		}
		values = append(values, row)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := createTx(ctx, tx, ownerID, title, sqlName, columns)
	if err != nil {
		return nil, err
	}
	if _, err := insertRowsTx(ctx, tx, t, values); err != nil {
		return nil, err
	}
	if err := checkTableSize(ctx, tx, t.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t.RowCount = len(values)
	return &ImportResult{Table: t, Rows: len(values)}, nil
}

// parseCSV reads the header and records and infers column definitions.
func parseCSV(r io.Reader) ([]Column, [][]string, error) {
	br := bufio.NewReader(io.LimitReader(r, maxImportBytes+1))
	first, err := br.Peek(4096)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, nil, err
	}
	header := strings.SplitN(string(first), "\n", 2)[0]

	cr := csv.NewReader(br)
	cr.Comma = detectDelimiter(header)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	all, err := cr.ReadAll()
	if err != nil {
		return nil, nil, core.NewUserError("could not read CSV: %v", err)
	}
	if len(all) == 0 || len(all[0]) == 0 {
		return nil, nil, core.NewUserError("the CSV is empty")
	}
	headers := all[0]
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff") // Excel BOM
	}
	if len(headers) > MaxColumns {
		return nil, nil, core.NewUserError("the CSV has %d columns; a table can have at most %d", len(headers), MaxColumns)
	}
	records := all[1:]

	used := map[string]bool{}
	columns := make([]Column, len(headers))
	for i, h := range headers {
		name := Identifier(h, fmt.Sprintf("column_%d", i+1))
		for base, n := name, 2; used[name]; n++ {
			name = fmt.Sprintf("%s_%d", base, n)
		}
		used[name] = true

		label := strings.TrimSpace(h)
		if label == name {
			label = ""
		}
		var sample []string
		for _, rec := range records {
			if i < len(rec) && strings.TrimSpace(rec[i]) != "" {
				sample = append(sample, rec[i])
			}
		}
		col := inferColumn(sample)
		col.ID = fmt.Sprintf("c%d", i+1)
		col.Name = name
		col.Label = label
		columns[i] = col
	}
	return columns, records, nil
}

func detectDelimiter(header string) rune {
	best, bestCount := ',', -1
	for _, d := range []rune{',', ';', '\t'} {
		if n := strings.Count(header, string(d)); n > bestCount {
			best, bestCount = d, n
		}
	}
	return best
}

// inferColumn picks the most specific type that every non-empty value fits.
func inferColumn(values []string) Column {
	if len(values) == 0 {
		return Column{Type: TypeText}
	}
	all := func(ok func(string) bool) bool {
		for _, v := range values {
			if !ok(v) {
				return false
			}
		}
		return true
	}
	strictBool := func(s string) bool {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "false", "yes", "no", "ano", "ne":
			return true
		}
		return false
	}
	switch {
	case all(strictBool):
		return Column{Type: TypeCheckbox}
	case all(func(s string) bool { _, ok := parseNumber(s); return ok }):
		// Count decimals in the source text: "24,90" means 2 decimals even
		// though the stored value is 24.9.
		decimals := 0
		for _, v := range values {
			v = strings.TrimSpace(v)
			if i := strings.LastIndexAny(v, ".,"); i >= 0 && !strings.ContainsAny(v[i+1:], "eE") {
				if d := len(v) - i - 1; d > decimals {
					decimals = d
				}
			}
		}
		if decimals > 10 {
			decimals = 10
		}
		return Column{Type: TypeNumber, Decimals: &decimals}
	case all(func(s string) bool { _, ok := parseDate(s); return ok }):
		return Column{Type: TypeDate}
	case all(func(s string) bool {
		_, err := NormalizeValue(Column{Type: TypeURL}, s)
		return err == nil
	}):
		return Column{Type: TypeURL}
	}
	return Column{Type: TypeText}
}

// Identifier turns a human name ("Částka (Kč)") into a SQL identifier
// ("castka_kc"); fallback is used when nothing usable remains.
func Identifier(name, fallback string) string {
	var b strings.Builder
	underscore := false
	for _, r := range norm.NFD.String(strings.ToLower(strings.TrimSpace(name))) {
		switch {
		case unicode.Is(unicode.Mn, r): // drop diacritics
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			underscore = false
		default:
			if !underscore && b.Len() > 0 {
				b.WriteByte('_')
				underscore = true
			}
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		s = fallback
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	if len(s) > MaxNameLength {
		s = strings.TrimRight(s[:MaxNameLength], "_")
	}
	if reservedWords[s] {
		s += "_"
	}
	return s
}

// FormatValue renders a stored value for display and export.
func FormatValue(c Column, v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		if c.Decimals != nil {
			// Round the exact value (half away from zero), not a float.
			if r, ok := new(big.Rat).SetString(x.String()); ok {
				return r.FloatString(*c.Decimals)
			}
		}
		return x.String()
	}
	return fmt.Sprint(v)
}

// WriteCSV writes the table (header labels, then rows) as comma-separated CSV.
func WriteCSV(w io.Writer, t *Table, rows []Row) error {
	cw := csv.NewWriter(w)
	header := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = c.Header()
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			rec[i] = FormatValue(c, r.Data[c.ID])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteMarkdown writes the table as a GitHub-flavoured Markdown table.
func WriteMarkdown(w io.Writer, t *Table, rows []Row) error {
	esc := strings.NewReplacer("|", `\|`, "\n", " ", "\r", "")
	cells := func(values []string) string {
		for i, v := range values {
			values[i] = esc.Replace(v)
		}
		return "| " + strings.Join(values, " | ") + " |\n"
	}
	header := make([]string, len(t.Columns))
	sep := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = c.Header()
		sep[i] = "---"
		if c.Type == TypeNumber {
			sep[i] = "---:"
		}
	}
	if _, err := io.WriteString(w, cells(header)+"|"+strings.Join(sep, "|")+"|\n"); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			rec[i] = FormatValue(c, r.Data[c.ID])
		}
		if _, err := io.WriteString(w, cells(rec)); err != nil {
			return err
		}
	}
	return nil
}
