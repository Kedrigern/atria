# Manuscripts

Status: proposal. Nothing in this document is built yet.

## 1. Problem

A user converts whole books, for example old printed books, into Markdown. Each book is one large Markdown file. The user wants to keep these books in order and export them as EPUB, DOCX or a simple PDF.

Today there are two bad options:

* As notes, the books fill the notes tree, and a note has no author, language or cover.
* As e-books, the user keeps only the EPUB and loses the Markdown source, which is the format that converts to everything else.

## 2. Concept

A **manuscript** is the editable Markdown source of one book, with book metadata. It is a new entity type.

* Manuscripts have their own page and list. They do not appear in the notes tree.
* An **e-book** stays what it is today: an immutable EPUB snapshot. "Publish as e-book" creates an e-book from a manuscript. The manuscript stays the source, and a later publication creates a new snapshot.
* One manuscript is one Markdown file. The chapters come from the `#` headings.

## 3. Original page markers

The converted books keep the page numbers of the printed original, so that the user can cite them. A marker is an HTML comment on its own, with only a number:

```markdown
…end of the text of page fifteen.
<!-- 15 -->
Start of the text of page sixteen…
```

A marker `<!-- N -->` is where page N of the original ends. The text before the first marker belongs to page 1 or to the page before the first marker number.

Converted books do not all use the same form. Atria accepts these forms of a marker:

| Form | Example |
| --- | --- |
| A number | `<!-- 15 -->` |
| A number with a page word | `<!-- p. 15 -->`, `<!-- s. 15 -->`, `<!-- str. 15 -->`, `<!-- page 15 -->` |
| A Roman number, for front matter | `<!-- xiv -->`, `<!-- p. xiv -->` |

Upper and lower case are the same. A marker is always an HTML comment, so it never shows in a rendered page by accident. Forms without a comment, for example `[15]`, are not markers, because the same text is common in normal prose and in references. The final list of forms comes from the real books (§8).

Roman numbers form their own sequence before the Arabic numbers. Each sequence must grow by one.

Atria reads the markers as follows:

* **Original pages:** the number of the last marker. If text follows the last marker, the count is that number plus one.
* **Check:** the numbers must grow by one. Atria lists gaps, repeats and numbers that go back, with the line number. This finds conversion errors.
* **Display:** the web page of a manuscript shows each marker as a small page number in the margin.
* **EPUB:** each marker becomes an EPUB 3 page break (`epub:type="pagebreak"`), and the navigation document gets a `page-list`. E-book readers then show the page of the original, so citations work there too.
* **DOCX and PDF:** each marker becomes a small page number in the text, for example `[15]`. The exact form is decided in step 6.

Other HTML comments stay comments. A note can use the same markers. The parser is shared code, so notes can show original pages too.

## 3a. Markdown check

A converted book often has small errors. Atria checks a Markdown file before an upload and shows a report. The report has warnings, not errors: the user decides whether to upload the file as it is or to correct it first. The same check is on the manuscript page. The CLI has it too (`atria manuscript check book.md`), so a user can check a file before an upload.

| Area | What the check finds |
| --- | --- |
| Headings | No `#` heading. More than one `#` title where chapters use `##`. A level that skips one (`##` then `####`). An empty heading. A heading longer than 200 characters, often a paragraph with a `#` by mistake. |
| Footnotes | A reference `[^n]` without a definition. A definition that no reference uses. The same label defined twice. |
| Page markers | Gaps, repeated numbers and numbers that go back. A Roman number after the Arabic numbers. A comment that looks like a marker but does not match a form from §3, for example `<!-- 15a -->`. |
| Text | Words broken across lines by a scanner (`slo-` at the end of a line), invalid UTF-8. |

Each warning has the line number and a short quote of the line. The report also shows the counts: chapters, footnotes, standard pages and original pages.

## 4. Data

```sql
CREATE TABLE manuscripts (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    author VARCHAR(255),
    language VARCHAR(35),
    description TEXT,
    cover_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL,
    markdown_content TEXT NOT NULL
);
```

* The title, folder, tags, visibility and trash come from `entities`. The type is `manuscript`.
* The metadata fields are the same as for e-books, so publishing copies them.
* Standard pages (1 800 characters) and original pages are computed, not stored.
* Search gets a `search_vector` like notes.

## 5. Export

| Format | How |
| --- | --- |
| Markdown | The source file, unchanged. |
| EPUB | The existing EPUB generator in Atria, with the page markers from §3. |
| DOCX | `pandoc`, as a separate process. |
| PDF | `pandoc` with `typst` as the PDF engine, as a separate process. |

`pandoc` and `typst` are optional, like DuckDB for table views. If a tool is not installed, the page does not offer that export. Installing them on dakara is a separate Ansible change.

## 6. Section settings

Atria has more and more sections. Each user chooses in the profile which sections show in the sidebar and on the home page: RSS, Read-it-later, Notes, E-books, Games, Tables and later Manuscripts. A hidden section still works by its address. The setting only removes it from the menu and the home page.

## 7. Delivery plan

Small steps first. Each step is one pull request and can be released alone.

1. **Section settings** (§6): a preference with the hidden sections, the sidebar and the home page use it, the profile page sets it.
2. **Markdown check and page markers** (§3, §3a): the shared parser and check, the CLI `atria check book.md`, and the original page count and the check report on the note page.
3. **Manuscripts, basic:** migration, service, list and detail pages, Markdown upload with the check report, editing, CLI `atria manuscript import` for many files at once.
4. **Manuscript export:** Markdown and EPUB download, "Publish as e-book".
5. **EPUB page list** (§3): page breaks and `page-list` in generated EPUBs.
6. **pandoc and typst:** the server change on dakara first, then DOCX and PDF export.

## 8. Open questions

* The interface of Atria is in English only (`docs/architecture/ui.md`). The first user of manuscripts reads Czech. Is that acceptable, or is a translation needed?
* The form of page markers in DOCX and PDF (§3).
* The marker forms in §3 are a first guess. Two or three real books will show the forms that are in use. They will also show which rules in §3a give too many false warnings.
