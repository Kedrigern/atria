package tables

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeValue(t *testing.T) {
	two := 2
	cases := []struct {
		col  Column
		in   any
		want any
		ok   bool
	}{
		{Column{Type: TypeText}, "  hello ", "hello", true},
		{Column{Type: TypeText}, "", nil, true},
		{Column{Type: TypeText}, strings.Repeat("ž", MaxCellChars+1), nil, false},
		{Column{Type: TypeNumber, Decimals: &two}, "1 234,50", json.Number("1234.5"), true},
		{Column{Type: TypeNumber}, 0.1, json.Number("0.1"), true},
		{Column{Type: TypeNumber}, "1e3", json.Number("1000"), true},
		{Column{Type: TypeNumber}, "1/3", nil, false},
		{Column{Type: TypeNumber}, "12abc", nil, false},
		{Column{Type: TypeNumber}, "-0", json.Number("0"), true},
		{Column{Type: TypeDate}, "31.12.2026", "2026-12-31", true},
		{Column{Type: TypeDate}, "2026-02-30", nil, false},
		{Column{Type: TypeCheckbox}, "ano", true, true},
		{Column{Type: TypeCheckbox}, false, false, true},
		{Column{Type: TypeCheckbox}, "maybe", nil, false},
		{Column{Type: TypeSelect, Options: []string{"a", "b"}}, "b", "b", true},
		{Column{Type: TypeSelect, Options: []string{"a", "b"}}, "c", nil, false},
		{Column{Type: TypeURL}, "https://example.com/x", "https://example.com/x", true},
		{Column{Type: TypeURL}, "javascript:alert(1)", nil, false},
		{Column{Type: TypeURL}, "ftp://example.com", nil, false},
	}
	for _, c := range cases {
		got, err := NormalizeValue(c.col, c.in)
		if !c.ok {
			assert.Error(t, err, "%s %v", c.col.Type, c.in)
			continue
		}
		require.NoError(t, err, "%s %v", c.col.Type, c.in)
		assert.Equal(t, c.want, got, "%s %v", c.col.Type, c.in)
	}
}

func TestIdentifier(t *testing.T) {
	cases := map[string]string{
		"Částka (Kč)":   "castka_kc",
		"Výdaje říjen":  "vydaje_rijen",
		"  2026 plan ":  "_2026_plan",
		"select":        "select_",
		"%%%":           "fallback",
		"Hello, World!": "hello_world",
	}
	for in, want := range cases {
		assert.Equal(t, want, Identifier(in, "fallback"), in)
	}
	assert.LessOrEqual(t, len(Identifier(strings.Repeat("abc ", 40), "x")), MaxNameLength)
}

func TestValidateColumns(t *testing.T) {
	ok := []Column{{ID: "c1", Name: "a", Type: TypeText}}
	assert.NoError(t, ValidateColumns(ok))

	bad := map[string][]Column{
		"empty":          nil,
		"dup name":       {{ID: "c1", Name: "a", Type: TypeText}, {ID: "c2", Name: "a", Type: TypeText}},
		"dup id":         {{ID: "c1", Name: "a", Type: TypeText}, {ID: "c1", Name: "b", Type: TypeText}},
		"bad name":       {{ID: "c1", Name: "Bad Name", Type: TypeText}},
		"reserved":       {{ID: "c1", Name: "from", Type: TypeText}},
		"unknown type":   {{ID: "c1", Name: "a", Type: "money"}},
		"select no opts": {{ID: "c1", Name: "a", Type: TypeSelect}},
	}
	for name, cols := range bad {
		assert.Error(t, ValidateColumns(cols), name)
	}
	many := make([]Column, MaxColumns+1)
	for i := range many {
		many[i] = Column{ID: Identifier("c", "c") + string(rune('a'+i%26)) + strings.Repeat("x", i/26), Name: "n" + strings.Repeat("x", i), Type: TypeText}
	}
	assert.Error(t, ValidateColumns(many))
}

func TestParseCSVInference(t *testing.T) {
	data := "\ufeffPoložka;Částka (Kč);Datum;Zaplaceno;Odkaz;Pozn\n" +
		"Mléko;24,90;3.10.2026;ano;https://shop.cz/m;\n" +
		"Chleba;1 249,5;2026-10-04;ne;;x\n"
	cols, recs, err := parseCSV(strings.NewReader(data))
	require.NoError(t, err)
	require.Len(t, recs, 2)

	types := []ColumnType{TypeText, TypeNumber, TypeDate, TypeCheckbox, TypeURL, TypeText}
	names := []string{"polozka", "castka_kc", "datum", "zaplaceno", "odkaz", "pozn"}
	for i, c := range cols {
		assert.Equal(t, types[i], c.Type, c.Name)
		assert.Equal(t, names[i], c.Name)
	}
	require.NotNil(t, cols[1].Decimals)
	assert.Equal(t, 2, *cols[1].Decimals, `"24,90" means 2 decimals`)
	assert.Equal(t, "Částka (Kč)", cols[1].Header())
	assert.Equal(t, "Položka", cols[0].Header(), "BOM stripped from the first header")
}

func TestFormatValueRoundsExactly(t *testing.T) {
	one, two := 1, 2
	assert.Equal(t, "5.3", FormatValue(Column{Type: TypeNumber, Decimals: &one}, json.Number("5.25")))
	assert.Equal(t, "189.00", FormatValue(Column{Type: TypeNumber, Decimals: &two}, json.Number("189")))
	assert.Equal(t, "0.1", FormatValue(Column{Type: TypeNumber}, json.Number("0.1")))
	assert.Equal(t, "true", FormatValue(Column{Type: TypeCheckbox}, true))
	assert.Equal(t, "", FormatValue(Column{Type: TypeText}, nil))
}
