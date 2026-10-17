# Tables & Views — Design

> Status: **proposal** (supersedes the earlier JSONB-blob design of this file).
> UI name: *Tables*. Internal entity type: `spreadsheet` (already allowed by the
> `entities.type` check constraint).

## 1. Goals and non-goals

**Goals**

* Small, typed data tables owned by a user: expenses, reading log, inventory,
  habit tracker. Personal scale — typically tens to a few thousand rows.
* **Raw tables hold only data.** No formulas inside the data, so CSV export is
  always clean and nothing can go stale.
* **Views compute from tables**: computed columns, filters, sorting, grouping,
  totals and **joins across the user's tables**, written as SQL.
* Tables are ordinary Atria entities: tags, links, attachments, visibility,
  trash, full-text search, storage overview, CLI.
* Views can be embedded in notes and render without JavaScript (EPUB, public
  pages, e-readers).

**Non-goals (v1)**

* Spreadsheet-style cell formulas (`=A1+B1`). Calculation lives in views.
* Real-time collaborative editing. Conflicts are detected, not merged.
* Big data. Hard limits keep every table loadable in memory (see §7).

## 2. Concepts

| Concept | What it is | Stored as |
|---|---|---|
| **Table** | Typed columns + rows of values. Editable. | entity `spreadsheet` + `spreadsheets` + `spreadsheet_rows` |
| **Column** | Name, type, options. Has a stable ID; renaming never touches row data. | element of `spreadsheets.columns` |
| **Row** | One record; values keyed by column ID. | one row in `spreadsheet_rows` |
| **View** | A saved SQL `SELECT` over one or more of the owner's tables. Read-only result. | `spreadsheet_views` (see §5.4 for the open question) |

## 3. Storage options considered

| Option | Edits | Scale | Complexity | Verdict |
|---|---|---|---|---|
| **A. One JSONB array per table** (`data_rows`, the previous design) | Every cell edit rewrites the whole table value; conflicts only per table | OK to ~2–5k rows | Lowest | Rejected: whole-table rewrites and coarse conflict detection |
| **B. One Postgres row per table row**, values in a JSONB object | Cell edit rewrites one small row; conflicts per row | Comfortable to 100k+ | Low | **Chosen** |
| C. A real Postgres table per user table (dynamic `CREATE TABLE`) | Native | Excellent | High: DDL at runtime, type changes = `ALTER TABLE`, naming/permission issues, schema bloat | Rejected |
| D. Entity-attribute-value (one row per cell) | Cheap per cell | Poor reads | Medium | Rejected |
| E. Parquet files on disk | Every edit rewrites a file; separate backups and locking | Excellent for analytics | Medium | Rejected for storage; **supported as an import/export format** |

Why B: edits stay cheap and local, concurrent edits to different rows never
conflict, everything stays inside Postgres (one `pg_dump` backs up all data,
transactions, permissions and the trash work as for any entity), and JSONB
numbers are PostgreSQL `numeric` — exact, so money does not suffer from
floating-point rounding.

## 4. Schema

```sql
CREATE TABLE spreadsheets (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    -- Identifier used in view SQL: FROM expenses. Unique per owner.
    sql_name VARCHAR(63) NOT NULL CHECK (sql_name ~ '^[a-z_][a-z0-9_]*$'),
    -- Ordered column definitions, see §4.1.
    columns JSONB NOT NULL DEFAULT '[]',
    -- Bumped on every column change; clients send it back to detect stale schemas.
    schema_version INTEGER NOT NULL DEFAULT 1
);
-- sql_name uniqueness per owner is enforced in the service (owner lives in entities).

CREATE TABLE spreadsheet_rows (
    id UUID PRIMARY KEY,                       -- UUIDv7, stable row identity
    spreadsheet_id UUID NOT NULL REFERENCES spreadsheets(id) ON DELETE CASCADE,
    position DOUBLE PRECISION NOT NULL,        -- manual order, see §4.3
    data JSONB NOT NULL DEFAULT '{}',          -- {"<column id>": value, ...}
    version INTEGER NOT NULL DEFAULT 1,        -- optimistic locking per row
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector tsvector                     -- maintained by trigger, see §8
);
CREATE INDEX idx_spreadsheet_rows_order ON spreadsheet_rows (spreadsheet_id, position);
CREATE INDEX idx_spreadsheet_rows_search ON spreadsheet_rows USING GIN (search_vector);
```

### 4.1 Columns

```json
[
  {"id": "c1", "name": "item",     "type": "text"},
  {"id": "c2", "name": "price",    "type": "number", "decimals": 2},
  {"id": "c3", "name": "bought",   "type": "date"},
  {"id": "c4", "name": "category", "type": "select", "options": ["food", "home", "fun"]},
  {"id": "c5", "name": "paid",     "type": "checkbox"},
  {"id": "c6", "name": "receipt",  "type": "url"}
]
```

* `id` is generated once and never changes; `name` is what users and SQL see
  (`^[a-z_][a-z0-9_]*$`, unique within the table; a display label may differ).
* Row data is keyed by `id`, so **renaming a column does not touch any row**.
* Deleting a column removes it from `columns`; leftover keys in rows are
  ignored and cleaned up lazily.

### 4.2 Types and stored values

| Type | JSON value in `data` | DuckDB type in views | Notes |
|---|---|---|---|
| `text` | string | `VARCHAR` | Inline Markdown subset rendered in the UI: `**bold**`, `*italic*`, `[link](https://…)`; everything else escaped |
| `number` | number (Postgres `numeric`, exact) | `DECIMAL(18, decimals)`, or `DOUBLE` when `decimals` is unset | |
| `date` | `"YYYY-MM-DD"` | `DATE` | |
| `checkbox` | `true` / `false` | `BOOLEAN` | |
| `select` | one of `options` | `VARCHAR` | |
| `url` | string, http(s) only | `VARCHAR` | |

Missing key or `null` = empty cell. Values are validated in Go on every write;
the database never sees an invalid value. Changing a column's type converts
values that fit and blanks the rest (the UI shows how many before confirming).

### 4.3 Row order

`position` is a float: inserting between rows *a* and *b* uses `(a + b) / 2`.
When two neighbours get too close (< 1e-9) the table is renumbered `1, 2, 3, …`
in one statement. Sorting in the UI is a view concern and never rewrites
`position`.

### 4.4 Concurrency

* Each row carries `version`. An update sends the version it was based on;
  `UPDATE … WHERE id = $1 AND version = $2` → 0 rows = conflict → HTTP 409, and
  the editor reloads that row and shows a notice.
* Column changes check `schema_version` the same way.
* Different rows never conflict, so two devices editing different rows just work.

## 5. Views

### 5.1 Definition

A view is a single SQL `SELECT` over the owner's tables, referenced by
`sql_name`:

```sql
SELECT e.category,
       SUM(e.price)                 AS spent,
       c.budget,
       c.budget - SUM(e.price)      AS remaining
FROM expenses e
JOIN categories c USING (category)
WHERE e.bought >= DATE '2026-10-01'
GROUP BY e.category, c.budget
ORDER BY spent DESC
```

Computed columns are just expressions in the `SELECT` (`price * qty AS total`),
so one language covers formulas, filters, grouping and joins. Optional
per-view display settings (column formats, footer totals) live next to the SQL.

### 5.2 Query engine: embedded DuckDB

Views run in [DuckDB](https://duckdb.org) embedded in the Atria process — not
in the main Postgres:

1. Parse the view SQL to find referenced table names (DuckDB's
   `json_serialize_sql` / prepared-statement metadata); reject unknown names.
2. Open a **fresh in-memory DuckDB** for this one query.
3. Load **only the referenced tables of the view's owner** from Postgres
   (typed per §4.2; a 5k-row table loads in milliseconds).
4. Run the query with limits (§7); return columns + rows; discard the database.

Running user SQL in the main Postgres was rejected: it would share the
database with every other user's data and with the application itself
(permissions, CPU, locks).

### 5.3 Sandbox (mandatory)

User SQL is untrusted. Every query connection is configured before the user's
statement runs:

* `SET enable_external_access = false` — no file, URL or database access
  (`read_csv('/etc/passwd')`, `httpfs`, `ATTACH`, `COPY … TO` all fail).
* `SET autoinstall_known_extensions = false`, `SET autoload_known_extensions = false`.
* `SET lock_configuration = true` — the user statement cannot undo the above.
* `SET memory_limit = '256MB'`, `SET threads = 2`.
* Only one statement, and it must be a `SELECT` (checked on the parsed
  statement type, not by string matching).
* Context timeout (5 s) interrupts the query; results are capped (§7).
* Isolation between users is structural: another user's tables are never
  loaded into the in-memory database, so they cannot be referenced.

A dedicated test suite tries each escape (file read, URL, `ATTACH`, `COPY`,
`INSTALL`, `SET` after lock, multiple statements, huge cross joins) and must
see every one fail.

### 5.4 Open question: views inside tables, or standalone entities?

* **Inside tables** (`spreadsheet_views.spreadsheet_id`): simplest; a view
  inherits its table's visibility and is deleted with it. Awkward for joins —
  which table "owns" a view over two tables?
* **Standalone entities** (type `view`): own title, tags, visibility (e.g. a
  public summary over private data), natural home for multi-table views,
  embeddable by ID like notes.

Because views can join several tables, **standalone entities are the better
fit**; a view simply lists the tables it reads. Decision pending.

### 5.5 Dependency cost

The official Go driver (`github.com/duckdb/duckdb-go/v2`, successor of
`marcboeker/go-duckdb`) uses cgo: the binary grows considerably (estimate:
tens of MB — to be measured by the prototype) and builds need a C toolchain (present on Fedora and GitHub
runners); cross-compiling gets harder. Alternative without cgo:
`dolthub/go-mysql-server` (pure Go, MySQL dialect) — slower and less complete
for analytics. To be validated by the prototype (§11).

## 6. Embedding in notes

`{{view:<uuid or short id>}}` (and `{{table:…}}` for a raw table) in Markdown is
replaced **on the server** by a plain HTML `<table>`:

* Works without JavaScript: EPUB export, public pages, offline, e-readers;
  fits the strict CSP.
* Only the note owner's tables/views can be embedded; on a **public** note only
  views/tables that are themselves public are rendered, otherwise a
  placeholder.
* Results are capped (e.g. 200 rows) with a "showing 200 of N" footer and a
  link to the full view.
* Editing happens on the table's own page, not inside the note.

## 7. Limits

| Limit | Value | Why |
|---|---|---|
| Rows per table | 20,000 | Keeps view loading in memory cheap |
| Columns per table | 100 | |
| Stored data per table | 10 MB | |
| Cell text | 10,000 characters | |
| Tables joined in one view | 8 | |
| View runtime | 5 s | DuckDB interrupt via context |
| View result | 10,000 rows (UI pages it), 200 in embeds | |
| DuckDB memory | 256 MB per query | |

Limits are constants in one place and reported as user errors when hit.

## 8. Integration with the rest of Atria

* **Entities:** tables and views are entities → title, tags, links,
  attachments, visibility, trash, `short_id` and title lookup work unchanged.
* **Search:** a trigger builds `spreadsheet_rows.search_vector` from the text,
  select and url values; search returns the *table* (with the matching row as
  the snippet), filtered like notes (owner, visibility, not deleted).
* **Storage overview:** a "Tables" line (`pg_column_size(data)` summed per owner).
* **Retention/trash:** deleting a table soft-deletes the entity; purging
  cascades to rows. Views referencing a deleted table show an error, not data.
* **Import/export:** CSV (types inferred on import, overridable), Parquet via
  DuckDB, Markdown table for export.

## 9. API (web)

| Method | Path | Purpose |
|---|---|---|
| GET | `/tables` | list of tables and views |
| GET | `/tables/:id` | editor page (Tabulator) |
| GET | `/api/tables/:id/rows?offset=&limit=` | rows page (JSON) |
| POST | `/api/tables/:id/rows` | insert row(s) after a given position |
| PATCH | `/api/tables/:id/rows/:row` | update cells `{version, values: {col: v}}` → 409 on conflict |
| DELETE | `/api/tables/:id/rows/:row` | delete row |
| PUT | `/api/tables/:id/columns` | replace column definitions `{schema_version, columns}` |
| POST | `/api/tables/import` | CSV/Parquet upload → new table |
| GET | `/tables/:id/export.(csv\|parquet)` | download |
| GET/POST | `/views/:id`, `/api/views/:id/run` | view page / run (JSON) |

All JSON endpoints use the existing session, CSRF and owner checks; errors use
`core.UserError` for validation, conflict and limit messages.

**Frontend:** Tabulator 6.4 (already vendored) from `/static`, initialised by a
static script with `data-*` configuration — no inline JS, consistent with the
CSP. Edits are debounced (500 ms) and sent per row.

## 10. CLI

```
atria table list
atria table show <table> [-f table|csv|md|json]
atria table import data.csv [--name expenses] [--title "Expenses 2026"]
atria table export <table> [-o file.csv|file.parquet]
atria table rename|rm <table>
atria view create <title> --sql 'SELECT …' | --file view.sql
atria view show <view> [-f table|csv|md|json]
atria view edit|rm <view>
```

## 11. Delivery plan

0. **Prototype (throwaway)** — DuckDB in Go: sandbox escape tests (§5.3),
   binary size and CI build with cgo, load + join timing on 10k rows.
   Go/no-go for §5.2.
1. **Raw tables: backend + CLI** — migration, service (types, validation,
   ordering, versions), CSV import/export, tests.
2. **Web editor** — list page, Tabulator editor, per-row autosave with 409
   handling, column management, CSV import/export.
3. **Integration** — search, storage overview, tags/links/visibility panels.
4. **Views** — entity type, DuckDB engine with sandbox, web page + CLI.
5. **Embedding** in notes (server-rendered HTML).

Steps 1–3 do not depend on the outcome of the prototype.

## 12. Open questions

1. Views as standalone entities (recommended, §5.4) or inside tables?
2. Is the DuckDB cgo dependency acceptable (§5.5), pending the prototype?
3. Are the limits in §7 right for your use?
4. Should number columns default to exact decimals (`DECIMAL`) or floats
   (`DOUBLE`) when the user does not set `decimals`?
