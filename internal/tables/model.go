// Package tables implements raw data tables (entity type "spreadsheet"):
// typed columns and rows, stored one Postgres row per table row. See
// docs/architecture/spreadsheets.md.
package tables

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
)

// Limits for personal reading tables (design §7).
const (
	MaxRows          = 5000
	MaxColumns       = 80
	MaxCellChars     = 6000
	MaxTableBytes    = 10 << 20
	MaxNameLength    = 63
	maxSelectOptions = 100
)

// ColumnType is the type of every value in a column.
type ColumnType string

const (
	TypeText     ColumnType = "text"
	TypeNumber   ColumnType = "number"
	TypeDate     ColumnType = "date"
	TypeCheckbox ColumnType = "checkbox"
	TypeSelect   ColumnType = "select"
	TypeURL      ColumnType = "url"
)

// Column defines one column. ID is stable and keys the row data, so renaming
// a column never touches rows. Name is the SQL identifier used by views;
// Label is the human-readable header (defaults to Name).
type Column struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Label    string     `json:"label,omitempty"`
	Type     ColumnType `json:"type"`
	Decimals *int       `json:"decimals,omitempty"`
	Options  []string   `json:"options,omitempty"`
}

// Header returns the label shown to people.
func (c Column) Header() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Name
}

// Table is a table's metadata (entity + schema), without rows.
type Table struct {
	core.Entity
	SQLName       string   `json:"sql_name"`
	Columns       []Column `json:"columns"`
	SchemaVersion int      `json:"schema_version"`
	RowCount      int      `json:"row_count"`
}

// Row is one table row. Data maps column IDs to normalized values: string
// (text, select, url, date as YYYY-MM-DD), json.Number (number) or bool
// (checkbox). Missing key = empty cell.
type Row struct {
	ID       uuid.UUID      `json:"id"`
	Position float64        `json:"position"`
	Data     map[string]any `json:"data"`
	Version  int            `json:"version"`
}

var identifierRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// ValidateIdentifier checks a table or column name usable in view SQL.
func ValidateIdentifier(kind, name string) error {
	if len(name) > MaxNameLength || !identifierRe.MatchString(name) {
		return core.NewUserError("%s name %q must start with a letter or _, contain only a-z, 0-9 and _, and be at most %d characters", kind, name, MaxNameLength)
	}
	if reservedWords[name] {
		return core.NewUserError("%s name %q is a reserved SQL word", kind, name)
	}
	return nil
}

// A few SQL keywords that would make views awkward to write.
var reservedWords = map[string]bool{
	"select": true, "from": true, "where": true, "group": true, "order": true, "by": true,
	"join": true, "on": true, "as": true, "and": true, "or": true, "not": true, "null": true,
	"table": true, "limit": true, "union": true, "case": true, "when": true, "then": true,
	"else": true, "end": true, "true": true, "false": true, "having": true, "with": true,
}

// ValidateColumns checks a complete column list.
func ValidateColumns(columns []Column) error {
	if len(columns) == 0 {
		return core.NewUserError("a table needs at least one column")
	}
	if len(columns) > MaxColumns {
		return core.NewUserError("a table can have at most %d columns", MaxColumns)
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, c := range columns {
		if c.ID == "" || ids[c.ID] {
			return core.NewUserError("column IDs must be unique and non-empty (%q)", c.ID)
		}
		ids[c.ID] = true
		if err := ValidateIdentifier("column", c.Name); err != nil {
			return err
		}
		if names[c.Name] {
			return core.NewUserError("duplicate column name %q", c.Name)
		}
		names[c.Name] = true
		if utf8.RuneCountInString(c.Label) > 200 {
			return core.NewUserError("column label for %q is too long", c.Name)
		}
		switch c.Type {
		case TypeText, TypeDate, TypeCheckbox, TypeURL:
		case TypeNumber:
			if c.Decimals != nil && (*c.Decimals < 0 || *c.Decimals > 10) {
				return core.NewUserError("column %q: decimals must be between 0 and 10", c.Name)
			}
		case TypeSelect:
			if len(c.Options) == 0 || len(c.Options) > maxSelectOptions {
				return core.NewUserError("select column %q needs 1 to %d options", c.Name, maxSelectOptions)
			}
		default:
			return core.NewUserError("column %q has unknown type %q (use text, number, date, checkbox, select or url)", c.Name, c.Type)
		}
	}
	return nil
}

// NormalizeValue validates raw input for column c and returns the value to
// store, or nil for an empty cell. raw may come from JSON (string, float64,
// bool, json.Number) or from text input such as CSV.
func NormalizeValue(c Column, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		raw = s
	}

	switch c.Type {
	case TypeText:
		s := fmt.Sprint(raw)
		if utf8.RuneCountInString(s) > MaxCellChars {
			return nil, core.NewUserError("column %q: text is longer than %d characters", c.Header(), MaxCellChars)
		}
		return s, nil

	case TypeNumber:
		n, ok := parseNumber(raw)
		if !ok {
			return nil, core.NewUserError("column %q: %q is not a number", c.Header(), fmt.Sprint(raw))
		}
		return n, nil

	case TypeDate:
		d, ok := parseDate(fmt.Sprint(raw))
		if !ok {
			return nil, core.NewUserError("column %q: %q is not a date (use YYYY-MM-DD or DD.MM.YYYY)", c.Header(), fmt.Sprint(raw))
		}
		return d, nil

	case TypeCheckbox:
		b, ok := parseBool(raw)
		if !ok {
			return nil, core.NewUserError("column %q: %q is not true/false", c.Header(), fmt.Sprint(raw))
		}
		return b, nil

	case TypeSelect:
		s := fmt.Sprint(raw)
		for _, o := range c.Options {
			if o == s {
				return s, nil
			}
		}
		return nil, core.NewUserError("column %q: %q is not one of the options", c.Header(), s)

	case TypeURL:
		s := fmt.Sprint(raw)
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(s) > 2000 {
			return nil, core.NewUserError("column %q: %q is not an http(s) URL", c.Header(), s)
		}
		return s, nil
	}
	return nil, core.NewUserError("column %q has unknown type %q", c.Header(), c.Type)
}

var numberRe = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?$`)

// parseNumber accepts numbers from JSON or text ("1 234,50" and "1234.5" both
// work) and returns them as an exact decimal json.Number, stored as a JSON
// number (PostgreSQL numeric) so values never pick up float rounding.
func parseNumber(raw any) (json.Number, bool) {
	var s string
	switch v := raw.(type) {
	case json.Number:
		s = v.String()
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		s = strconv.Itoa(v)
	case int64:
		s = strconv.FormatInt(v, 10)
	case string:
		s = v
	default:
		return "", false
	}
	s = strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(strings.TrimSpace(s))
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1) // decimal comma
	}
	if !numberRe.MatchString(s) {
		return "", false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", false
	}
	return json.Number(trimDecimal(r.FloatString(10))), true
}

func trimDecimal(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}

var dateLayouts = []string{"2006-01-02", "2.1.2006", "02.01.2006", "2. 1. 2006", "2006/01/02"}

func parseDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

func parseBool(raw any) (bool, bool) {
	if b, ok := raw.(bool); ok {
		return b, true
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(raw))) {
	case "true", "1", "yes", "y", "ano", "x", "✓", "✔":
		return true, true
	case "false", "0", "no", "n", "ne", "":
		return false, true
	}
	return false, false
}

// NormalizeRow validates values keyed by column ID. Unknown column IDs are
// rejected; nil values remove the key.
func NormalizeRow(columns []Column, values map[string]any) (map[string]any, error) {
	byID := make(map[string]Column, len(columns))
	for _, c := range columns {
		byID[c.ID] = c
	}
	out := make(map[string]any, len(values))
	for id, raw := range values {
		c, ok := byID[id]
		if !ok {
			return nil, core.NewUserError("unknown column %q", id)
		}
		v, err := NormalizeValue(c, raw)
		if err != nil {
			return nil, err
		}
		if v != nil {
			out[id] = v
		}
	}
	return out, nil
}
