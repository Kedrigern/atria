package tables_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/tables"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expenseColumns() []tables.Column {
	two := 2
	return []tables.Column{
		{ID: "c1", Name: "item", Type: tables.TypeText},
		{ID: "c2", Name: "price", Type: tables.TypeNumber, Decimals: &two},
		{ID: "c3", Name: "paid", Type: tables.TypeCheckbox},
	}
}

func TestTableLifecycle(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tbl, err := tables.Create(ctx, db, owner.ID, "Expenses", "expenses", expenseColumns())
	require.NoError(t, err)

	_, err = tables.Create(ctx, db, owner.ID, "Other title", "expenses", expenseColumns())
	assert.ErrorContains(t, err, "already have a table named")

	inserted, err := tables.InsertRows(ctx, db, owner.ID, tbl.ID, []map[string]any{
		{"c1": "milk", "c2": "24,90", "c3": "ano"},
		{"c1": "bread", "c2": 35},
	})
	require.NoError(t, err)
	require.Len(t, inserted, 2)
	assert.Less(t, inserted[0].Position, inserted[1].Position)

	rows, err := tables.Rows(ctx, db, owner.ID, tbl.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "milk", rows[0].Data["c1"])
	assert.Equal(t, json.Number("24.9"), rows[0].Data["c2"], "numbers come back exact")
	assert.Equal(t, true, rows[0].Data["c3"])

	t.Run("update with version and conflict", func(t *testing.T) {
		v, err := tables.UpdateRow(ctx, db, owner.ID, tbl.ID, rows[1].ID, rows[1].Version, map[string]any{"c2": "36.5", "c3": true})
		require.NoError(t, err)
		assert.Equal(t, rows[1].Version+1, v)

		_, err = tables.UpdateRow(ctx, db, owner.ID, tbl.ID, rows[1].ID, rows[1].Version, map[string]any{"c2": "1"})
		assert.ErrorIs(t, err, tables.ErrConflict, "stale version must be rejected")

		_, err = tables.UpdateRow(ctx, db, owner.ID, tbl.ID, rows[1].ID, v, map[string]any{"c3": nil})
		require.NoError(t, err)
		got, _ := tables.Rows(ctx, db, owner.ID, tbl.ID, 0, 0)
		_, hasPaid := got[1].Data["c3"]
		assert.False(t, hasPaid, "nil clears a cell")
		assert.Equal(t, json.Number("36.5"), got[1].Data["c2"])

		_, err = tables.UpdateRow(ctx, db, owner.ID, tbl.ID, rows[1].ID, v+1, map[string]any{"c2": "not a number"})
		assert.ErrorContains(t, err, "is not a number")
	})

	t.Run("change column type converts or clears", func(t *testing.T) {
		current, err := tables.Get(ctx, db, owner.ID, tbl.ID)
		require.NoError(t, err)
		cols := current.Columns
		cols[0].Type = tables.TypeNumber // "milk", "bread" cannot become numbers
		cols[1].Type = tables.TypeText   // numbers become text
		cols = cols[:2]                  // drop "paid"
		cleared, err := tables.SetColumns(ctx, db, owner.ID, tbl.ID, current.SchemaVersion, cols)
		require.NoError(t, err)
		assert.Equal(t, 2, cleared)

		_, err = tables.SetColumns(ctx, db, owner.ID, tbl.ID, current.SchemaVersion, cols)
		assert.ErrorIs(t, err, tables.ErrConflict, "stale schema version")

		got, _ := tables.Rows(ctx, db, owner.ID, tbl.ID, 0, 0)
		assert.Equal(t, "24.9", got[0].Data["c2"])
		assert.NotContains(t, got[0].Data, "c1")
		assert.NotContains(t, got[0].Data, "c3")
	})

	t.Run("search vector is maintained", func(t *testing.T) {
		_, err := tables.InsertRows(ctx, db, owner.ID, tbl.ID, []map[string]any{{"c2": "quasar observation"}})
		require.NoError(t, err)
		var hits int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM spreadsheet_rows WHERE search_vector @@ to_tsquery('english', 'quasar')`).Scan(&hits))
		assert.Equal(t, 1, hits)
	})

	t.Run("delete row and trash", func(t *testing.T) {
		require.NoError(t, tables.DeleteRow(ctx, db, owner.ID, tbl.ID, rows[0].ID))
		assert.Error(t, tables.DeleteRow(ctx, db, owner.ID, tbl.ID, rows[0].ID))

		require.NoError(t, core.SoftDeleteEntity(ctx, db, owner.ID, tbl.ID))
		_, err := tables.Get(ctx, db, owner.ID, tbl.ID)
		assert.Error(t, err)
		_, err = tables.Create(ctx, db, owner.ID, "Expenses again", "expenses", expenseColumns())
		assert.NoError(t, err, "a trashed table frees its SQL name")
	})
}

func TestTablesAreOwnerScoped(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	other, err := users.CreateUser(ctx, db, "other@atria.local", "Other", "pass", core.RoleUser)
	require.NoError(t, err)
	tbl, err := tables.Create(ctx, db, owner.ID, "Private", "private_data", expenseColumns())
	require.NoError(t, err)
	inserted, err := tables.InsertRows(ctx, db, owner.ID, tbl.ID, []map[string]any{{"c1": "secret"}})
	require.NoError(t, err)

	_, err = tables.Get(ctx, db, other.ID, tbl.ID)
	assert.Error(t, err)
	rows, err := tables.Rows(ctx, db, other.ID, tbl.ID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows)
	_, err = tables.InsertRows(ctx, db, other.ID, tbl.ID, []map[string]any{{"c1": "x"}})
	assert.Error(t, err)
	_, err = tables.UpdateRow(ctx, db, other.ID, tbl.ID, inserted[0].ID, 1, map[string]any{"c1": "hacked"})
	assert.Error(t, err)
	assert.Error(t, tables.DeleteRow(ctx, db, other.ID, tbl.ID, inserted[0].ID))
	_, err = tables.FindBySQLName(ctx, db, other.ID, "private_data")
	assert.Error(t, err)

	// The same SQL name is fine for another user.
	_, err = tables.Create(ctx, db, other.ID, "Mine", "private_data", expenseColumns())
	assert.NoError(t, err)
}

func TestLimitsAndImport(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	var csv strings.Builder
	csv.WriteString("n\n")
	for i := 0; i < tables.MaxRows+1; i++ {
		csv.WriteString("1\n")
	}
	_, err := tables.ImportCSV(ctx, db, owner.ID, "Too big", "", strings.NewReader(csv.String()))
	assert.ErrorContains(t, err, "at most 5000")

	res, err := tables.ImportCSV(ctx, db, owner.ID, "Výdaje", "", strings.NewReader("Položka;Cena\nMléko;24,90\n"))
	require.NoError(t, err)
	assert.Equal(t, "vydaje", res.Table.SQLName)
	assert.Equal(t, 1, res.Rows)

	tbl, err := tables.Get(ctx, db, owner.ID, res.Table.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, tbl.RowCount)

	values := make([]map[string]any, tables.MaxRows)
	for i := range values {
		values[i] = map[string]any{}
	}
	_, err = tables.InsertRows(ctx, db, owner.ID, tbl.ID, values)
	assert.ErrorContains(t, err, "at most 5000 rows")
}
