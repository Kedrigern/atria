# Spreadsheet: Architecture and Rules for Spreadsheets

This document outlines the architectural decisions, database schema, and frontend constraints for the lightweight spreadsheet module within the Atria system.

## 1. Philosophical Design & UX Constraints
* **Column-Oriented Model:** Unlike traditional cell-driven spreadsheets (e.g., Excel), Atria utilizes a column-driven data layout inspired by Notion and Airtable. The column is the core structural element defining data types and validation.
* **Restricted Formulas:** Arbitrary cell formulas (e.g., `=A1+B1` anywhere in the grid) are prohibited to prevent cyclic dependencies. Calculations are restricted to:
    * **Footer Aggregations:** Column-level summaries displayed at the bottom (e.g., SUM, AVERAGE).
    * **Computed Columns:** Row-level operations where an entire column is read-only and automatically calculated based on a per-row rule (e.g., `[Price] * [Quantity]`).
* **Rich Text via Markdown:** Formatting (bold, italics) and hyperlinks are handled directly via native Markdown within the cell text (e.g., `**Important**`, `[Link](url)`), parsed on the frontend.

## 2. Database Schema & Storage (PostgreSQL)
Spreadsheets are first-class entities inheriting from the core `entities` table via Class Table Inheritance. To optimize performance and I/O operations, the spreadsheet data is split into two distinct `JSONB` columns.

```sql
CREATE TABLE spreadsheets (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    columns_config JSONB NOT NULL DEFAULT '[]', -- Schema definition: field names, types, aggregations, formatting
    data_rows JSONB NOT NULL DEFAULT '[]'       -- User data: flat array of row objects
);
```

### Benefits of the Split JSONB Model:

* **I/O Efficiency:** Editing cell values only updates `data_rows`, preventing PostgreSQL from re-writing the structural configuration.
* **Targeted Indexing:** GIN indexes are applied exclusively to `data_rows` to enable fast full-text search without cluttering indexes with configuration metadata.
* **Payload Optimization:** Allows fetching structural metadata (`columns_config`) independently for dashboard widgets or lists without transferring the entire dataset.

## 3. Frontend Implementation & API Workflow

* **Library Selection:** **Tabulator.js** is selected as the frontend engine due to its out-of-the-box support for inline editing, footer calculations (`bottomCalc`), local sorting, filtering, and native CSV exporting.
* **API Interactions:** The backend serves as a clean, stateless API.
* `GET /api/sheet/{uuid}` returns both `columns_config` and `data_rows`.
* `PATCH /api/sheet/{uuid}` receives only the updated `data_rows` payload upon cell edits, ensuring minimal data transfer and preventing schema corruption.


* **Embedding in Notes:** Spreadsheets can be seamlessly embedded into Markdown notes using a custom shortcode (`{{table:uuid}}`). The backend replaces this code with a wrapper `<div>` containing the JSON payload, which is initialized into a Tabulator instance on page load.
