# Atria

Atria is a personal knowledge base, read-it-later list, and RSS reader in one web application. It also has a command-line tool (CLI) for the same data.

The name comes from the *atrium*, the central hall of a Roman house. Light came in from outside there, and the family received news there. Atria has the same job for your reading. It keeps notes, saved articles, RSS feeds, and small data tables in one place. You do not need Pocket, Feedly, Notion, and loose Markdown files at the same time.

Atria is online-first: you edit on the server. You can also read saved content offline.

> [!WARNING]
> Atria is in active development. Its features and its data format can change.

## Usage

<details>
<summary>Click to show <code>atria help</code> output</summary>

```
Atria is a unified tool for managing your knowledge base, reading list, and RSS feeds.

Usage:
  atria [command]

Available Commands:
  article     Read-it-Later and article management
  attachment  File uploads and attachment management
  completion  Generate the autocompletion script for the specified shell
  db          System and database administration
  ebook       Create and manage immutable e-book snapshots
  export      Bulk export functionalities
  help        Help about any command
  link        Knowledge graph and entity relations management
  note        Knowledge base and markdown notes management
  rss         RSS feed management and triage
  search      Full-text search across notes, articles and RSS items
  server      Starts the Atria web server
  tag         Tag management for all entities
  user        User management

Flags:
  -h, --help      help for atria
  -v, --version   version for atria

Use "atria [command] --help" for more information about a command.
```

</details>

## Administration

The CLI connects to the database in `DATABASE_URL` directly. Thus these commands work on the data of all users:

```console
atria db usage                  # storage per user (largest first) + database size
atria db usage -u alice@x.cz    # one user; -f json|csv for scripts
atria user rm alice@x.cz        # shows what would be deleted; add --force to delete
atria rss prune --dry-run       # what RSS retention would free; drop --dry-run to run it now
```

## Development

You need Go 1.26 or later and PostgreSQL. On Fedora, install the `golang` package.

To run the tests, set `TEST_DATABASE_URL` in `.env`. The tests delete all data in that database, so use a separate database. Its name must contain `test`, or the tests stop.

```console
go test ./... -p 1
go test ./... -p 1 -v    # verbose
```

To start the web server for development:

```console
go run ./cmd/atria server
```

To see how Atria extracts an article from a web page, use `article parse`. It does not save anything and does not need a database.

```console
go run ./cmd/atria article parse https://example.com/post             # Markdown
go run ./cmd/atria article parse https://example.com/post -f json     # metadata + HTML + text
go run ./cmd/atria article parse https://example.com/post --save-html post.html
go run ./cmd/atria article parse --file post.html --url https://example.com/post
```

The `--save-html` flag keeps a copy of the downloaded page. If a site does not parse well, save the page once. Then parse the saved file again with `--file` while you change the extraction code. You can also use the saved page as a test fixture. The `--sanitized` flag shows the HTML as the web interface shows it.

To build the binary:

```console
go build -o atria cmd/atria/*.go
```
