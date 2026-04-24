# Architecture and Rules for Notes (Markdown)

This document outlines the core architectural decisions, parsing rules, and rendering behaviors for Markdown notes within the Atria system.

## 1. Source of Truth (Database vs. Front Matter)

* **Database Absolute Priority:** During runtime, the relational database (DB) is the primary source of truth for searching, filtering, and rendering the UI.
* **Role of Front Matter (FM):** The YAML Front Matter block in the Markdown header does not dictate application state during normal operation. It serves exclusively as a standardized metadata carrier for **portability** (exporting and importing).
* **Importing:** Only when importing an external `.md` file into the system does Atria read the FM to create or update the corresponding database record.

## 2. Synchronization and Front Matter Content

* **Update on Save:** Upon saving a note (via Web UI or API), the system takes the current metadata from the DB and automatically rewrites/generates the YAML header within the raw text stored in the `content` column. This keeps the file perpetually ready for lossless export.
* **Mandatory FM Fields:**
    * `id`: The note's UUID (critical for lossless imports and deduplication).
    * `title`: The primary title of the note.
    * `path`: The hierarchical location (e.g., `/projects/atria`).
    * `author`: The author of the note.
    * `tags`: An array of strings representing tags (e.g., `["architecture", "go"]`).
* **What DOES NOT belong in FM:** Knowledge graph links (references to other notes) and attachments. These elements semantically belong in the Markdown body (e.g., `[Link](/notes/uuid)` or `![Image](/data/...)`). The backend parses these from the body upon saving to populate the relational tables.

## 3. Display and "Heading Shift"

* **Raw Storage:** Users write natural Markdown. If they use a level 1 heading (`# Heading`), it is saved in the DB exactly as written. The stored text is never arbitrarily modified.
* **Shift on Render (Display Only):** Since the primary database title serves as the single semantic `<h1>` on the web page, the backend AST parser (Goldmark) automatically shifts all headings in the Markdown body down by one level during HTML generation (provided the document contains an `H1`). Thus, `#` becomes `<h2>`, `##` becomes `<h3>`, etc. 
* **Export Integrity:** Exported `.md` files retain their original, unshifted hierarchy.

## 4. Interactive Table of Contents (TOC)

* **No Backend Generation:** The TOC is not generated on the server side.
* **Frontend Invariants:**
    * The backend guarantees that every rendered heading in the HTML has an auto-generated, unique `id` attribute (e.g., `<h2 id="my-heading">`).
    * The Frontend uses a lightweight JavaScript function on page load to traverse the note container, collect `<h2>` and `<h3>` elements, read their `id` and `innerText`, and dynamically construct a `<details>`/`<summary>` TOC block placed directly below the main title.

## 5. Supported Markdown Extensions (Syntax & Parsers)

To ensure interoperability with modern tools (like GitHub and Obsidian), Atria supports and natively renders the following elements:

* **GFM (GitHub Flavored Markdown):** Natively enabled via Goldmark. Includes support for tables, task lists (`- [x] Task`), and strikethrough (`~~text~~`).
* **Syntax Highlighting:** Fenced code blocks starting with three backticks and a language identifier (e.g., ````go ````) are rendered with appropriate CSS classes to allow client-side highlighting (e.g., via highlight.js/Prism).
* **Mermaid Diagrams:** Fenced blocks identified with ````mermaid ```` are rendered as `<pre><code class="language-mermaid">`. A frontend script loaded from a CDN (Mermaid.js) intercepts these blocks and renders vector graphs.
* **Mathematics (MathJax / KaTeX):** Inline math wrapped in single dollars (`$...$`) and block math in double dollars (`$$...$$`) remain preserved in the HTML. A frontend script (MathJax) processes them into typographic mathematical notation (Obsidian-compatible).
