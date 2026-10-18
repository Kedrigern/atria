// Table editor (Tabulator) for /tables/:id. Loaded by table_detail.html;
// every cell edit is saved per row with optimistic locking (409 = conflict).
(() => {
    const root = document.getElementById("table-editor");
    if (!root || root.dataset.initialized || typeof Tabulator === "undefined") return;
    root.dataset.initialized = "true";

    const tableId = root.dataset.tableId;
    const api = `/api/tables/${tableId}`;
    const statusEl = document.querySelector("[data-table-status]");
    const rowCountEl = document.querySelector("[data-table-rowcount]");
    let meta = null; // table metadata: columns, schema_version
    let grid = null;

    const csrf = () => document.querySelector('meta[name="csrf-token"]')?.content || "";

    function status(message, isError = false) {
        if (!statusEl) return;
        statusEl.textContent = message;
        statusEl.classList.toggle("table-status-error", isError);
    }

    async function request(method, url, body) {
        const response = await fetch(url, {
            method,
            headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf() },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        let data = null;
        if (response.status !== 204) {
            try {
                data = await response.json();
            } catch {
                data = null;
            }
        }
        if (!response.ok) {
            const error = new Error(data?.error || `Request failed (${response.status})`);
            error.status = response.status;
            throw error;
        }
        return data;
    }

    // --- Rendering -------------------------------------------------------

    const escapeHTML = (s) =>
        String(s).replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[ch]);

    // Safe Markdown subset for text cells: **bold**, *italic*, [text](http(s)://…).
    // Everything is escaped first, so cell content can never inject markup.
    function renderInlineMarkdown(value) {
        if (value === null || value === undefined) return "";
        return escapeHTML(value)
            .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
            .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
            .replace(/\*([^*]+)\*/g, "<em>$1</em>");
    }

    function linkFormatter(cell) {
        const value = cell.getValue();
        if (!value) return "";
        if (!/^https?:\/\//i.test(value)) return escapeHTML(value);
        const a = document.createElement("a");
        a.href = value;
        a.textContent = value;
        a.target = "_blank";
        a.rel = "noopener noreferrer";
        return a;
    }

    function numberFormatter(column) {
        return (cell) => {
            const value = cell.getValue();
            if (value === null || value === undefined || value === "") return "";
            const n = Number(value);
            if (Number.isNaN(n)) return escapeHTML(value);
            return column.decimals === undefined || column.decimals === null ? String(value) : n.toFixed(column.decimals);
        };
    }

    function columnDefinition(column) {
        const def = {
            title: column.label || column.name,
            field: column.id,
            headerTooltip: column.name,
            minWidth: 90,
            headerSort: true,
        };
        switch (column.type) {
            case "number":
                Object.assign(def, { editor: "input", hozAlign: "right", formatter: numberFormatter(column), sorter: "number" });
                break;
            case "date":
                Object.assign(def, { editor: "date", sorter: "string" });
                break;
            case "checkbox":
                Object.assign(def, { editor: "tickCross", formatter: "tickCross", hozAlign: "center", formatterParams: { allowEmpty: true } });
                break;
            case "select":
                Object.assign(def, { editor: "list", editorParams: { values: ["", ...(column.options || [])], clearable: true } });
                break;
            case "url":
                Object.assign(def, { editor: "input", formatter: linkFormatter, maxInitialWidth: 260, variableHeight: true, cssClass: "atria-wrap" });
                break;
            default:
                Object.assign(def, {
                    editor: "input", // Enter commits, like in a spreadsheet
                    formatter: (cell) => renderInlineMarkdown(cell.getValue()),
                    // Long text wraps into several lines instead of widening the
                    // column; the user can still drag the column wider.
                    maxInitialWidth: 360,
                    variableHeight: true,
                    cssClass: "atria-wrap",
                });
        }
        return def;
    }

    const toGridRow = (row) => ({ ...row.data, _id: row.id, _version: row.version });

    function updateRowCount() {
        if (rowCountEl && grid) rowCountEl.textContent = grid.getDataCount();
    }

    // --- Loading -----------------------------------------------------------

    async function load() {
        const data = await request("GET", api);
        meta = data.table;
        const columns = meta.columns.map(columnDefinition);
        columns.push({
            title: "",
            formatter: "buttonCross",
            width: 40,
            hozAlign: "center",
            headerSort: false,
            cellClick: (e, cell) => deleteRow(cell.getRow()),
        });

        if (grid) {
            grid.setColumns(columns);
            await grid.replaceData(data.rows.map(toGridRow));
            updateRowCount();
        } else {
            grid = new Tabulator(root, {
                data: data.rows.map(toGridRow),
                columns,
                index: "_id",
                layout: "fitDataStretch",
                maxHeight: "70vh",
                placeholder: "No rows yet — add one with “+ Row”.",
            });
            grid.on("cellEdited", saveCell);
            // The row count is only known once Tabulator has built the table.
            grid.on("tableBuilt", updateRowCount);
            grid.on("dataLoaded", updateRowCount);
        }
        renderColumnsDialog();
    }

    // --- Editing -----------------------------------------------------------

    // Saves of one row run strictly one after another, each with the version
    // returned by the previous save; otherwise quick successive edits in the
    // same row would send the same version and cause a false 409 conflict.
    const rowQueues = new Map();

    function saveCell(cell) {
        const id = cell.getRow().getData()._id;
        const queued = (rowQueues.get(id) || Promise.resolve()).then(() => saveCellNow(cell));
        rowQueues.set(id, queued);
        queued.finally(() => {
            if (rowQueues.get(id) === queued) rowQueues.delete(id);
        });
        return queued;
    }

    async function saveCellNow(cell) {
        const row = cell.getRow();
        const rowData = row.getData();
        let value = cell.getValue();
        if (value === "" || value === undefined) value = null;
        if (typeof value === "number") value = String(value); // server parses numbers exactly

        status("Saving…");
        try {
            const result = await request("PATCH", `${api}/rows/${rowData._id}`, {
                version: rowData._version,
                values: { [cell.getField()]: value },
            });
            row.update({ _version: result.version });
            row.normalizeHeight(); // re-wrap after the text changed
            status("Saved");
        } catch (error) {
            if (error.status === 409) {
                status("This table was changed elsewhere — reloaded the latest version.", true);
                await load();
            } else {
                cell.restoreOldValue();
                status(error.message, true);
            }
        }
    }

    async function addRow() {
        try {
            const row = await request("POST", `${api}/rows`, { values: {} });
            await grid.addRow(toGridRow(row));
            updateRowCount();
            status("Row added");
        } catch (error) {
            status(error.message, true);
        }
    }

    async function deleteRow(row) {
        if (!window.confirm("Delete this row?")) return;
        try {
            await request("DELETE", `${api}/rows/${row.getData()._id}`);
            row.delete();
            updateRowCount();
            status("Row deleted");
        } catch (error) {
            status(error.message, true);
        }
    }

    // --- Columns dialog ------------------------------------------------------

    const dialog = document.getElementById("table-columns-dialog");
    const columnsContainer = dialog?.querySelector("[data-table-columns]");
    const columnTemplate = dialog?.querySelector("[data-table-column-template]");

    function addColumnRow(column) {
        const fragment = columnTemplate.content.cloneNode(true);
        const fieldset = fragment.querySelector("fieldset");
        const set = (name, value) => {
            fieldset.querySelector(`[name="${name}"]`).value = value ?? "";
        };
        set("id", column.id);
        set("name", column.name);
        set("label", column.label);
        set("type", column.type || "text");
        set("decimals", column.decimals);
        set("options", (column.options || []).join("\n"));
        columnsContainer.appendChild(fragment);
    }

    function renderColumnsDialog() {
        if (!columnsContainer) return;
        columnsContainer.replaceChildren();
        meta.columns.forEach(addColumnRow);
    }

    function nextColumnID() {
        const used = new Set([...columnsContainer.querySelectorAll('[name="id"]')].map((i) => i.value));
        let n = used.size + 1;
        while (used.has(`c${n}`)) n++;
        return `c${n}`;
    }

    function readColumns() {
        return [...columnsContainer.querySelectorAll("fieldset")].map((fieldset) => {
            const get = (name) => fieldset.querySelector(`[name="${name}"]`).value.trim();
            const column = { id: get("id"), name: get("name"), type: get("type") };
            if (get("label")) column.label = get("label");
            if (column.type === "number" && get("decimals") !== "") column.decimals = Number(get("decimals"));
            if (column.type === "select") {
                column.options = get("options").split("\n").map((o) => o.trim()).filter(Boolean);
            }
            return column;
        });
    }

    async function saveColumns() {
        try {
            const result = await request("PUT", `${api}/columns`, {
                schema_version: meta.schema_version,
                columns: readColumns(),
            });
            dialog.close();
            await load();
            status(result.cleared ? `Columns saved; ${result.cleared} values did not fit the new type and were cleared.` : "Columns saved");
        } catch (error) {
            if (error.status === 409) {
                dialog.close();
                await load();
            }
            status(error.message, true);
        }
    }

    dialog?.querySelector("[data-table-add-column]")?.addEventListener("click", () =>
        addColumnRow({ id: nextColumnID(), name: `column_${columnsContainer.children.length + 1}`, type: "text" }),
    );
    dialog?.querySelector("[data-table-save-columns]")?.addEventListener("click", saveColumns);
    columnsContainer?.addEventListener("click", (event) => {
        if (event.target.closest("[data-table-remove-column]")) event.target.closest("fieldset").remove();
    });
    dialog?.addEventListener("close", () => meta && renderColumnsDialog());

    document.querySelector("[data-table-add-row]")?.addEventListener("click", addRow);

    load().catch((error) => status(error.message, true));
})();
