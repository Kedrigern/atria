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
* `atria note add "My new thought" --tags="ideas,project-x"`
  Quick-captures a markdown note.
* `atria note show <uuid>`
  Displays the raw markdown content of a specific note.

### Spreadsheets (`table` or alias `tab`)
* `atria table list`
  Lists all saved tabular data embedded in the knowledge base.
* `atria table show <uuid> [--format=csv]`
  Displays the table content. Easily exportable to CSV using the format flag.

### RSS Feeds (`rss` or alias `feed`)
* `atria rss list`
  Lists all subscribed feeds and their health/error status.
* `atria rss add <url>`
  Subscribes to a new RSS/Atom feed.

---

## 4. Bulk Exports (`export`)
* `atria export epub <uuid1> <uuid2> ...`
  Bundles the specified articles into an EPUB file for e-ink readers. 
  *(Note: The CLI will automatically validate the entity types and safely skip UUIDs that are not articles or note).*
