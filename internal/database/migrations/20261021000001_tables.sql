-- +goose Up

-- Raw data tables (entity type 'spreadsheet'). Design:
-- docs/architecture/spreadsheets.md. Column definitions live in
-- spreadsheets.columns; each table row is one spreadsheet_rows row whose
-- values are keyed by stable column IDs.
CREATE TABLE spreadsheets (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    sql_name VARCHAR(63) NOT NULL CHECK (sql_name ~ '^[a-z_][a-z0-9_]*$'),
    columns JSONB NOT NULL DEFAULT '[]',
    schema_version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE spreadsheet_rows (
    id UUID PRIMARY KEY,
    spreadsheet_id UUID NOT NULL REFERENCES spreadsheets(id) ON DELETE CASCADE,
    position DOUBLE PRECISION NOT NULL,
    data JSONB NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector tsvector
);
CREATE INDEX idx_spreadsheet_rows_order ON spreadsheet_rows (spreadsheet_id, position);
CREATE INDEX idx_spreadsheet_rows_search ON spreadsheet_rows USING GIN (search_vector);

-- +goose StatementBegin
CREATE FUNCTION spreadsheet_rows_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', COALESCE(
        (SELECT string_agg(value, ' ') FROM jsonb_each_text(NEW.data)), ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER spreadsheet_rows_search_vector_trig
BEFORE INSERT OR UPDATE OF data ON spreadsheet_rows
FOR EACH ROW EXECUTE FUNCTION spreadsheet_rows_search_vector_update();

-- +goose Down

DROP TABLE IF EXISTS spreadsheet_rows;
DROP FUNCTION IF EXISTS spreadsheet_rows_search_vector_update();
DROP TABLE IF EXISTS spreadsheets;
