package tables

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
)

// ErrConflict means the row or schema changed since the client loaded it.
var ErrConflict = core.NewUserError("the table was changed elsewhere; reload and try again")

// positionGap is the spacing between rows appended at the end.
const positionGap = 1024

// Create makes an empty table with the given columns.
func Create(ctx context.Context, db *sql.DB, ownerID uuid.UUID, title, sqlName string, columns []Column) (*Table, error) {
	if err := ValidateIdentifier("table", sqlName); err != nil {
		return nil, err
	}
	if err := ValidateColumns(columns); err != nil {
		return nil, err
	}
	if title == "" {
		return nil, core.NewUserError("table title is required")
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
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return t, nil
}

func createTx(ctx context.Context, tx *sql.Tx, ownerID uuid.UUID, title, sqlName string, columns []Column) (*Table, error) {
	var taken bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM spreadsheets s JOIN entities e ON e.id = s.id
		               WHERE e.owner_id = $1 AND s.sql_name = $2 AND e.deleted_at IS NULL)`,
		ownerID, sqlName).Scan(&taken)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, core.NewUserError("you already have a table named %q", sqlName)
	}

	now := time.Now().UTC()
	t := &Table{
		Entity: core.Entity{
			ID: core.NewUUID(), OwnerID: ownerID, Type: core.TypeSpreadsheet,
			Visibility: core.VisibilityPrivate, Title: core.LimitRunes(title, core.MaxTitleLength),
			Path: "/", CreatedAt: now, UpdatedAt: now,
		},
		SQLName: sqlName, Columns: columns, SchemaVersion: 1,
	}
	t.Slug = core.LimitRunes(sqlName, core.MaxSlugLength)
	t.ShortID = t.ID.String()[len(t.ID.String())-8:]

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO entities (id, owner_id, type, visibility, title, slug, path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, '/', $7, $7)`,
		t.ID, ownerID, core.TypeSpreadsheet, core.VisibilityPrivate, t.Title, t.Slug, now); err != nil {
		return nil, fmt.Errorf("insert table entity: %w", err)
	}
	cols, err := json.Marshal(columns)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO spreadsheets (id, sql_name, columns) VALUES ($1, $2, $3)`, t.ID, sqlName, cols); err != nil {
		return nil, fmt.Errorf("insert table: %w", err)
	}
	return t, nil
}

const tableSelect = `
	SELECT e.id, e.short_id, e.owner_id, e.visibility, e.title, e.slug, e.created_at, e.updated_at,
	       s.sql_name, s.columns, s.schema_version,
	       (SELECT COUNT(*) FROM spreadsheet_rows r WHERE r.spreadsheet_id = s.id)
	FROM spreadsheets s JOIN entities e ON e.id = s.id`

func scanTable(scan func(...any) error) (*Table, error) {
	var t Table
	var cols []byte
	if err := scan(&t.ID, &t.ShortID, &t.OwnerID, &t.Visibility, &t.Title, &t.Slug, &t.CreatedAt, &t.UpdatedAt,
		&t.SQLName, &cols, &t.SchemaVersion, &t.RowCount); err != nil {
		return nil, err
	}
	t.Type = core.TypeSpreadsheet
	if err := json.Unmarshal(cols, &t.Columns); err != nil {
		return nil, fmt.Errorf("decode columns: %w", err)
	}
	return &t, nil
}

// Get loads a table's metadata (no rows).
func Get(ctx context.Context, db *sql.DB, ownerID, id uuid.UUID) (*Table, error) {
	t, err := scanTable(db.QueryRowContext(ctx, tableSelect+`
		WHERE e.id = $1 AND e.owner_id = $2 AND e.deleted_at IS NULL`, id, ownerID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewUserError("table not found")
	}
	return t, err
}

// List returns the owner's tables, newest first.
func List(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]Table, error) {
	rows, err := db.QueryContext(ctx, tableSelect+`
		WHERE e.owner_id = $1 AND e.deleted_at IS NULL ORDER BY e.created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Table
	for rows.Next() {
		t, err := scanTable(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, *t)
	}
	return list, rows.Err()
}

// Rows returns a page of rows in their manual order. limit <= 0 means all.
func Rows(ctx context.Context, db *sql.DB, ownerID, tableID uuid.UUID, offset, limit int) ([]Row, error) {
	if limit <= 0 {
		limit = MaxRows
	}
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.position, r.data, r.version
		FROM spreadsheet_rows r JOIN entities e ON e.id = r.spreadsheet_id
		WHERE r.spreadsheet_id = $1 AND e.owner_id = $2 AND e.deleted_at IS NULL
		ORDER BY r.position, r.id
		OFFSET $3 LIMIT $4`, tableID, ownerID, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Row
	for rows.Next() {
		var r Row
		var data []byte
		if err := rows.Scan(&r.ID, &r.Position, &data, &r.Version); err != nil {
			return nil, err
		}
		if r.Data, err = decodeData(data); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// decodeData keeps numbers exact (json.Number) instead of float64.
func decodeData(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	m := map[string]any{}
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode row: %w", err)
	}
	return m, nil
}

// InsertRows appends rows (values keyed by column ID) at the end of the table
// and returns them with their IDs.
func InsertRows(ctx context.Context, db *sql.DB, ownerID, tableID uuid.UUID, values []map[string]any) ([]Row, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := lockTable(ctx, tx, ownerID, tableID)
	if err != nil {
		return nil, err
	}
	inserted, err := insertRowsTx(ctx, tx, t, values)
	if err != nil {
		return nil, err
	}
	if err := checkTableSize(ctx, tx, tableID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return inserted, nil
}

func insertRowsTx(ctx context.Context, tx *sql.Tx, t *Table, values []map[string]any) ([]Row, error) {
	if t.RowCount+len(values) > MaxRows {
		return nil, core.NewUserError("a table can have at most %d rows (has %d, adding %d)", MaxRows, t.RowCount, len(values))
	}
	var last float64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM spreadsheet_rows WHERE spreadsheet_id = $1`, t.ID).Scan(&last); err != nil {
		return nil, err
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO spreadsheet_rows (id, spreadsheet_id, position, data) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	out := make([]Row, 0, len(values))
	for i, v := range values {
		data, err := NormalizeRow(t.Columns, v)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		r := Row{ID: core.NewUUID(), Position: last + float64(i+1)*positionGap, Data: data, Version: 1}
		if _, err := stmt.ExecContext(ctx, r.ID, t.ID, r.Position, b); err != nil {
			return nil, fmt.Errorf("insert row: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// UpdateRow merges values (keyed by column ID; nil clears a cell) into a row.
// version must be the row version the change is based on; a mismatch returns
// ErrConflict. It returns the new version.
func UpdateRow(ctx context.Context, db *sql.DB, ownerID, tableID, rowID uuid.UUID, version int, values map[string]any) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := lockTable(ctx, tx, ownerID, tableID)
	if err != nil {
		return 0, err
	}

	var current []byte
	var currentVersion int
	err = tx.QueryRowContext(ctx, `SELECT data, version FROM spreadsheet_rows WHERE id = $1 AND spreadsheet_id = $2 FOR UPDATE`,
		rowID, tableID).Scan(&current, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, core.NewUserError("row not found")
	}
	if err != nil {
		return 0, err
	}
	if currentVersion != version {
		return 0, ErrConflict
	}

	data, err := decodeData(current)
	if err != nil {
		return 0, err
	}
	nonNil := map[string]any{}
	for id, v := range values {
		if v == nil {
			delete(data, id)
		} else {
			nonNil[id] = v
		}
	}
	normalized, err := NormalizeRow(t.Columns, nonNil)
	if err != nil {
		return 0, err
	}
	for id := range nonNil {
		if v, ok := normalized[id]; ok {
			data[id] = v
		} else {
			delete(data, id) // normalized to empty
		}
	}

	b, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE spreadsheet_rows SET data = $1, version = version + 1, updated_at = NOW() WHERE id = $2`, b, rowID); err != nil {
		return 0, err
	}
	if err := checkTableSize(ctx, tx, tableID); err != nil {
		return 0, err
	}
	if err := touchEntity(ctx, tx, tableID); err != nil {
		return 0, err
	}
	return version + 1, tx.Commit()
}

// DeleteRow removes one row.
func DeleteRow(ctx context.Context, db *sql.DB, ownerID, tableID, rowID uuid.UUID) error {
	result, err := db.ExecContext(ctx, `
		DELETE FROM spreadsheet_rows r USING entities e
		WHERE r.id = $1 AND r.spreadsheet_id = $2 AND e.id = r.spreadsheet_id
		  AND e.owner_id = $3 AND e.deleted_at IS NULL`, rowID, tableID, ownerID)
	if err != nil {
		return err
	}
	return core.RequireAffected(result, "row not found")
}

// SetColumns replaces the column definitions. schemaVersion must match the
// current one (else ErrConflict). Values of columns whose type changed are
// converted; values that do not fit are cleared. Values of removed columns are
// deleted. It returns the number of cleared cells.
func SetColumns(ctx context.Context, db *sql.DB, ownerID, tableID uuid.UUID, schemaVersion int, columns []Column) (int, error) {
	if err := ValidateColumns(columns); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := lockTable(ctx, tx, ownerID, tableID)
	if err != nil {
		return 0, err
	}
	if t.SchemaVersion != schemaVersion {
		return 0, ErrConflict
	}

	oldByID := map[string]Column{}
	for _, c := range t.Columns {
		oldByID[c.ID] = c
	}
	newByID := map[string]Column{}
	needsRewrite := false
	for _, c := range columns {
		newByID[c.ID] = c
		if old, ok := oldByID[c.ID]; ok && (old.Type != c.Type || fmt.Sprint(old.Options) != fmt.Sprint(c.Options)) {
			needsRewrite = true
		}
	}
	for id := range oldByID {
		if _, ok := newByID[id]; !ok {
			needsRewrite = true
		}
	}

	cleared := 0
	if needsRewrite {
		cleared, err = rewriteRows(ctx, tx, tableID, newByID)
		if err != nil {
			return 0, err
		}
	}

	b, err := json.Marshal(columns)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE spreadsheets SET columns = $1, schema_version = schema_version + 1 WHERE id = $2`, b, tableID); err != nil {
		return 0, err
	}
	if err := touchEntity(ctx, tx, tableID); err != nil {
		return 0, err
	}
	return cleared, tx.Commit()
}

// rewriteRows converts every row to the new columns: removed columns are
// dropped, values are re-validated for their (possibly new) type.
func rewriteRows(ctx context.Context, tx *sql.Tx, tableID uuid.UUID, columns map[string]Column) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, data FROM spreadsheet_rows WHERE spreadsheet_id = $1`, tableID)
	if err != nil {
		return 0, err
	}
	type update struct {
		id   uuid.UUID
		data []byte
	}
	var updates []update
	cleared := 0
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		data, err := decodeData(raw)
		if err != nil {
			rows.Close()
			return 0, err
		}
		out := map[string]any{}
		for colID, v := range data {
			c, ok := columns[colID]
			if !ok {
				continue
			}
			nv, err := NormalizeValue(c, convertible(v))
			if err != nil || nv == nil {
				cleared++
				continue
			}
			out[colID] = nv
		}
		b, err := json.Marshal(out)
		if err != nil {
			rows.Close()
			return 0, err
		}
		if !bytes.Equal(b, raw) {
			updates = append(updates, update{id, b})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, u := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE spreadsheet_rows SET data = $1, version = version + 1, updated_at = NOW() WHERE id = $2`, u.data, u.id); err != nil {
			return 0, err
		}
	}
	return cleared, nil
}

// convertible turns a stored value into input NormalizeValue understands for
// any target type (e.g. a number 5 becomes "5" for a text column).
func convertible(v any) any {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return v
}

// lockTable loads the owner's table and locks its spreadsheets row, which
// serialises writers of one table (row limits, positions, schema).
func lockTable(ctx context.Context, tx *sql.Tx, ownerID, tableID uuid.UUID) (*Table, error) {
	t, err := scanTable(tx.QueryRowContext(ctx, tableSelect+`
		WHERE e.id = $1 AND e.owner_id = $2 AND e.deleted_at IS NULL
		FOR UPDATE OF s`, tableID, ownerID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewUserError("table not found")
	}
	return t, err
}

func checkTableSize(ctx context.Context, tx *sql.Tx, tableID uuid.UUID) error {
	var size int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(pg_column_size(data)), 0) FROM spreadsheet_rows WHERE spreadsheet_id = $1`, tableID).Scan(&size); err != nil {
		return err
	}
	if size > MaxTableBytes {
		return core.NewUserError("the table would exceed %s", core.FormatBytes(MaxTableBytes))
	}
	return nil
}

func touchEntity(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `UPDATE entities SET updated_at = NOW() WHERE id = $1`, id)
	return err
}

// FindBySQLName returns the owner's active table with this SQL name.
func FindBySQLName(ctx context.Context, db *sql.DB, ownerID uuid.UUID, sqlName string) (*Table, error) {
	t, err := scanTable(db.QueryRowContext(ctx, tableSelect+`
		WHERE e.owner_id = $1 AND s.sql_name = $2 AND e.deleted_at IS NULL`, ownerID, sqlName).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewUserError("table not found")
	}
	return t, err
}
