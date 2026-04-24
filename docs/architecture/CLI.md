# Atria - Command Line Interface (CLI)

The Atria CLI is a unified tool for managing the database, administering users, and interacting with the personal knowledge base directly from the terminal.

## Design Pattern
The CLI uses a standard `<resource> <action>` pattern:
`atria <resource> <action> [arguments] [flags]`

## Global Flags
For commands that manipulate personal data (articles, notes, tags), a user context is required. This is provided via a global flag:
* `-u, --user <email|uuid>` : Specifies the owner of the content.
*(Note: Can be omitted if the `ATRIA_USER` environment variable is set).*

---

## 1. System & Database Administration (`db`)
Used for infrastructure operations. No user context required.

* `atria db ping`
  Verifies the connection to the PostgreSQL database.
* `atria db migrate`
  Applies all pending database migrations (Goose).
* `atria db dump`
  Exports the current database state (useful for backups).
* `atria db drop --force`
  Drops all tables and resets the database (used heavily in dev/test environments).

## 2. User Administration (`user`)
Used to manage instance accounts and access. No user context required.

* `atria user add <email> --name="John Doe" --role=admin`
  Creates a new user account and prompts for a password.
* `atria user list`
  Outputs a tabular list of all users on the instance.
* `atria user show <email|uuid>`
  Displays detailed information about a specific user.
* `atria user role <email|uuid> <user|admin>`
  Changes the permission role of an existing user.

---

## 3. Content Management (Domain Specific)
*These commands require the `-u` flag or `ATRIA_USER` env variable.*

### Articles & Read-it-Later (`article`)
* `atria article list [--format=table|csv|json]`
  Lists saved articles (Inbox). Supports different output formats for easy scripting.
* `atria article add <url>`
  Fetches the URL, runs Readability extraction, and saves it to the Inbox.
* `atria article show <uuid>`
  Displays the extracted text and metadata of the article.

### Knowledge Base (`note`)
* `atria note list [--format=table|csv|json]`
  Lists existing notes in the knowledge base.
* `atria note add <"Title"> [--path="/virtual/path"] [--tags="..."] [--file=<local_path>]`
  Creates a new note with the specified title. Missing parent folders in `--path` are created automatically.
  Examples:
  - Inline content: `echo "My quick thought" | atria note add "Quick thought" --path="/inbox"`
  - From file: `atria note add "Solar setup" --path="/home/solar" --file=./draft.md`
* `atria note show <uuid|short-uuid|"Title"> [--path="/virtual/path"]`
  Displays the raw markdown content of a specific note. 
  Resolution order:
  1. Exact UUID match.
  2. Prefix UUID match (Git-style short UUID, e.g., `550e840`).
  3. Title match across the ENTIRE knowledge base (Option II). If `--path` is provided, it narrows down the search.
  4. If multiple notes share the same title (or short UUID), the command fails safely and outputs a disambiguation list (UUID + Path) so the user can re-run the command with a specific identifier.
* `atria note export <uuid|short-uuid|"Title"> [--path="/virtual/path"] --out=<local_directory>`
  Exports a note or an entire folder hierarchy to the local filesystem.
  - Generates `.md` files containing the raw markdown content.
  - Recreates the virtual folder structure as physical directories inside the specified `--out` directory.
* `atria note rm <uuid|short-uuid|"Title"> [--path="/virtual/path"] [--recursive]`
  Deletes a note or folder. 
  - If the target is a single note, it deletes it.
  - If the target is a folder, the command will fail safely UNLESS the `--recursive` (or `-r`) flag is provided. This prevents accidental mass deletion.

### Spreadsheets (`table` or alias `tab`)
* `atria table list`
  Lists all saved tabular data embedded in the knowledge base.
* `atria table show <uuid> [--format=csv]`
  Displays the table content. Easily exportable to CSV using the format flag.

### RSS Feeds (`rss`)
* `atria rss list`: Lists all subscribed feeds, including their health status (`last_fetch_status`) and last sync time.
* `atria rss fetch`: Manually triggers the background worker to pull updates for all pending feeds.
* `atria rss show [--format=table|json|csv] [-l, --long]`: Displays the **Triage (unread items)**. 
    * Default view shows the 8-char **Item ID suffix**.
    * `--long` flag displays full URLs and Feed IDs.
* `atria rss save <item-id-suffix>`: The core bridge. Converts an RSS item into a permanent `article`, runs readability extraction, and marks the item as read.
* `atria rss rm <feed-id-suffix>`: Removes the feed subscription (and its metadata).
---

## 4. Bulk Exports (`export`)
* `atria export epub <uuid1> <uuid2> ...`
  Bundles the specified articles into an EPUB file for e-ink readers. 
  *(Note: The CLI will automatically validate the entity types and safely skip UUIDs that are not articles or note).*
