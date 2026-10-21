# User interface — design

> Status: **accepted**. The interface uses English only (section 3).

This document defines one look and one set of building blocks for the web interface of Atria. It applies to all pages in `internal/web/templates` and to `internal/web/static/style.css`.

## 1. Goals

The interface has these goals:

1. A person who knows one page of Atria knows how all pages work.
2. Each page uses a small set of shared components (reusable pieces of markup and CSS, for example a page header or a button).
3. A new page needs no new CSS in most cases.
4. The interface works with the light theme, the dark theme, the font size setting, a keyboard, and a phone.
5. The interface works well on e-ink readers with little computing power, for example the Onyx BOOX Nova Air C. Reading on these devices is one of the main uses of Atria (section 8).

The design is simple and uses as little JavaScript as possible. These are not goals: a new visual identity, a JavaScript framework, or a CSS framework. Atria stays with server-rendered templates, htmx, `water.css` as the base, and its own `style.css`.

## 2. Current state

A review of 18 pages in October 2026 found these problems.

The interface uses two languages. The navigation and most headings are English. The note and article pages, the profile, the login page, and the storage page are Czech. The RSS feed page shows both languages on one screen. Nine templates have Czech button texts, and eight have English ones.

The layout changes from page to page. Lists use a narrow column. Detail pages use a wide column with a side panel. Tables use the full width, and the profile has its own two-column layout. Each page builds its header in a different way.

Elements with the same role look different. For example, the main button of a page has three looks: small and blue, wide and grey, and wide and blue. Icons are partly SVG and partly emoji.

The cause is in the code. The templates contain 277 `style="…"` attributes, with nine different font sizes. The CSS has three different breakpoints (the screen width at which the layout changes) for phones: 600 px, 640 px, and 1024 px.

Some content is out of date or does not work:

- The home page shows a "Proof of Concept" box that calls Atria a technology demo.
- The General tab in Settings shows three settings and a save button, but no form saves them.
- The profile shows the raw headers from the authentication proxy. This is debug information.

The review also found two bugs. The RSS feed page shows an empty error box after each successful fetch. The search page ignores `?q=` in the address, so a reload or a saved search link shows no results. These bugs do not depend on this design and get a fix of their own.

## 3. Decision: the language of the interface

The interface uses English only, on all pages. The CLI, the error messages from the server, the documentation, and the code comments are already English. The Czech texts in the templates get an English translation during the migration (section 13).

Dates and numbers keep the Czech format: `21.10.2026 13:22`, and a decimal comma where a number is shown to a person.

## 4. Design tokens

A design token is a named value, for example a color or a size, that all CSS uses instead of a literal value. All tokens are CSS variables on `:root` in `style.css`. The light theme and the dark theme set the same names with different values, as today.

### 4.1 Colors

The colors that `water.css` and `style.css` define today stay: `--background-body`, `--background`, `--background-alt`, `--border`, `--text-main`, `--text-bright`, `--text-muted`, `--links`, `--focus`. These tokens are new:

| Token | Use |
|---|---|
| `--accent` | Primary buttons, the active navigation item, the active tab |
| `--danger` | Delete buttons, error messages |
| `--success` | Success messages, the "read" state |
| `--warning` | Warnings |

Each new token has a value for the light theme and a value for the dark theme. Text in each color must have a contrast ratio of at least 4.5:1 to its background (the WCAG AA level).

### 4.2 Spacing

All margins, paddings, and gaps use a scale with a step of 4 px:

| Token | Value |
|---|---|
| `--space-1` | 4px |
| `--space-2` | 8px |
| `--space-3` | 12px |
| `--space-4` | 16px |
| `--space-5` | 24px |
| `--space-6` | 40px |

The current helper classes (`mb-1`, `mb-3`, `mt-3`) stay and use these tokens.

### 4.3 Text sizes

There are five text sizes. The font size setting of the user scales all of them.

| Token | Value | Use |
|---|---|---|
| `--text-xs` | 0.8rem | Small labels, for example the title of a side panel section |
| `--text-sm` | 0.9rem | Help text, meta lines such as dates and sources |
| `--text-base` | 1rem | Body text, buttons, form fields |
| `--text-lg` | 1.25rem | Section headings |
| `--text-xl` | 1.6rem | The page title |

### 4.4 Shape and breakpoints

All rounded corners use `--radius: 6px`. The CSS uses two breakpoints only: `640px` for phones and `1024px` for small screens, where the sidebar collapses. The breakpoint of 600 px goes away.

## 5. Layout

### 5.1 The application frame

The frame stays as it is today: a narrow icon sidebar on the left and the main column on the right. On a phone, the sidebar becomes a menu behind the button in the top bar.

### 5.2 Page widths

Each page uses one of three widths. The classes exist today and stay:

| Class | Width | Pages |
|---|---|---|
| `centered-view` | at most 800 px | Lists (notes, read-it-later, RSS, e-books, tables, games), forms, settings, search, profile, login |
| `centered-view wide` | at most 1100 px | Detail pages with a side panel: note, article, e-book, RSS feed |
| `centered-view full` | the full main column | The table editor and, later, views |

The home page uses `centered-view`. The profile drops its own two-column layout.

### 5.3 The page header

Each page starts with the same header:

```html
<header class="page-header">
    <div>
        <h1 class="page-title">Knowledge Base</h1>
        <p class="page-subtitle">3 notes</p>              <!-- optional -->
    </div>
    <div class="page-actions">                            <!-- optional -->
        <a class="button-link outline btn-sm" href="/notes/import">Import</a>
        <a class="button-link btn-primary btn-sm" href="/notes/new">New note</a>
    </div>
</header>
```

These rules apply to the header:

- The page title is the only `h1` on the page.
- A page has at most one primary button. It is the last item in `page-actions`.
- On a phone, the actions go below the title.
- The settings pages put their tabs below the header.

## 6. Components

### 6.1 Buttons

There are four button types and two sizes. The classes exist today, except `btn-quiet`:

| Class | Use | Look |
|---|---|---|
| `btn-primary` | The main action of the page or the form | Filled with `--accent` |
| `outline` | All other actions | Border, no fill |
| `btn-danger` | Delete, remove, reset | Border in `--danger` |
| `btn-quiet` | Small actions in lists and toolbars, for example "Copy link" | No border, no fill |

Add `btn-sm` for buttons in headers, toolbars, and lists. A link that looks like a button gets `button-link` and one of the four types.

The text of a button is a verb, for example "Save", "Import", or "Move to trash". An icon-only button must have a `title` and an `aria-label` with that verb. A button that deletes data asks for confirmation with `data-confirm`.

### 6.2 Forms

Each field uses the same structure:

```html
<div class="form-group">
    <label for="title">Title</label>
    <input id="title" name="title" class="form-control" required />
    <p class="form-help">Used in the browser tab.</p>     <!-- optional -->
</div>
```

These rules apply to forms:

- Each field has a visible `label`. A placeholder is an example value, not the label.
- A form has one primary button, at the end of the form.
- An error for one field goes below that field. An error for the whole form goes into a flash message (6.6).

### 6.3 Lists

A list of items, for example notes, articles, RSS items, or e-books, uses `list-item`. An item has a title, which is a link, and a meta line in `--text-sm` and `--text-muted`. It can also have a short preview and quiet buttons on the right.

A list with columns, for example the tables, the storage overview, or the users, uses `data-list`. Numbers align to the right.

### 6.4 The side panel

Detail pages put the metadata into the side panel on the right. The panel contains sections of the type `meta-section` in this fixed order: contents (for notes), tags, links, attachments, visibility. On a phone, the panel goes below the content.

### 6.5 Tags and badges

A tag of the user (`#reading`) uses `tag`. A state, for example "read", "admin", "note", or "article", uses `badge` with one of the color tokens. Badges do not use emoji.

### 6.6 Messages and empty states

A flash message (a message that shows once after an action) uses `flash-message` with one of the types `success`, `error`, or `info`. A page without items shows one sentence in `page-empty-hint` and, where useful, the button that creates the first item.

### 6.7 Dialogs

Dialogs use the HTML `dialog` element with the class `dialog`. The title is an `h2`. The buttons are at the end, with the primary button last.

## 7. Icons

The interface uses one icon set: the SVG icons in `style.css` (`icon`, `icon-sm`, and the names such as `icon-trash`). Emoji are not used as icons. Each emoji in the templates today (for example 🎲, 💡, 📦, 🔐) gets an SVG icon or no icon. A new icon is added to the same set.

## 8. E-ink readers

E-ink readers are a main target of Atria. An e-ink screen (a screen that shows text like ink on paper) refreshes slowly. Each change on the screen can leave a faint ghost image. The processor of these devices is also slow, and the color e-ink screen of the BOOX Nova Air C shows only weak colors. These rules follow from that:

1. The interface has no animations, no transitions, and no smooth scrolling. Today the CSS fades and slides some elements for 0.2 to 0.3 seconds. The RSS list animates the removal of an item, and the "Top" button scrolls smoothly. All of these go away.
2. A page loads only the JavaScript that it uses. Today each page loads highlight.js, MathJax, and Mermaid. Most pages have no code, no formulas, and no diagrams. These libraries must load only on pages with that content. The table editor (Tabulator) stays on the table page only.
3. Color is never the only signal. A state, an error, or a selection also has a text, an icon, or a border. On a color e-ink screen, two colors can look the same.
4. Each element that you tap has a size of at least 40 × 40 px. E-ink readers use touch or a stylus, and you cannot hover. Thus no action is available only on hover.
5. Long content uses pages, not endless scrolling. Scrolling on e-ink is slow and refreshes the screen many times.
6. Reading pages (a note, an article, an RSS item) show the toolbar above and below the content. After you read to the end, you can act at once, for example archive the article, without a scroll back to the top.

## 9. Accessibility

These rules apply to all pages:

- Each element that you can click must also work with the keyboard, and it must show a visible focus outline in `--focus`.
- Text and backgrounds meet the contrast ratio from 4.1. The light theme is the main theme for e-ink readers, so its contrast matters most.
- The language in `<html lang="…">` matches the language of the interface.
- Images have an `alt` text. A decorative icon has `aria-hidden="true"`.

## 10. Texts in the interface

The texts in the interface follow the same rules as the documentation: short, plain sentences, the same word for the same thing on all pages, and no exclamation marks. These terms are fixed:

| Use | Do not use |
|---|---|
| Move to trash | Delete, Smazat (for soft delete) |
| Delete permanently | Delete (for removal that you cannot undo) |
| Save | Uložit, Save All Changes |
| Read-it-later | Inbox, Read-it-Later Inbox |

The texts stay in the templates. A translation system is not needed for one language.

## 11. Rules for the code

These rules keep the design consistent after the migration:

1. Templates do not use `style="…"`. The only exception is a value that the server calculates, for example a width in percent. It goes into a CSS variable: `style="--progress: 40%"`.
2. CSS uses the tokens from section 4 and no literal colors, sizes, or spacings.
3. A component gets its CSS in one place in `style.css`, under a comment with its name.
4. A Go test reads all templates. If a template has a `style` attribute that is not a CSS variable, the test fails. This is the same approach as the test that forbids inline JavaScript.

## 12. Screenshots for each change

The repository gets the screenshot script from the review, as `scripts/ui-screenshots`. It starts a server with sample data and takes screenshots of all main pages. It does this in the light and the dark theme, at desktop and phone width. Then it puts the screenshots into one overview image.

Each pull request that changes the interface adds the overview images before and after the change. This makes layout regressions visible before a release. The script also takes portrait screenshots in grayscale. They show approximately what an e-ink reader shows.

## 13. Migration plan

The work is done in small pull requests, in this order:

1. Fix the two bugs from section 2. This does not depend on this design.
2. Apply the e-ink rules 1 and 2 from section 8: remove the animations, and load highlight.js, MathJax, and Mermaid only where they are used. This makes all pages faster on e-ink readers at once.
3. Add the tokens, the components, and the screenshot script. No page changes yet.
4. Remove the outdated content: the "Proof of Concept" box, the General settings that do not work, and the proxy debug table. Settings then opens on the RSS tab.
5. Convert the pages area by area: notes, read-it-later and RSS, e-books, tables, games, search, settings and profile, login. Each pull request also translates the texts of its area into English (section 3).
6. Add the Go test from section 11. It fails until the last area is converted, so it comes last.

All six steps are done (pull requests #30 to #42). The tests `TestTemplatesHaveNoInlineStyles` and `TestTemplatesAreInEnglish` keep the rules from sections 3 and 11.

## 14. Decisions and open questions

1. Decided: the interface uses English only (section 3).
2. Decided: reading pages keep the toolbar above and below the content (section 8, rule 6).
3. Deferred: a note that starts with `# Title` shows its title twice. This design does not change it.
