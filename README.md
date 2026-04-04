# Atria

**Your personal mind palace.** *A knowledge base, read-it-later, and RSS reader that respects your time and focus.*

In ancient Roman architecture, the *Atrium* was the central, light-filled hall of a house. It was the place where light entered from the outside, where news was received, and where calm prevailed. 

That is exactly what **Atria** aims to be for your digital life. 

Instead of scattering your attention across Pocket, Feedly, Notion, and local Markdown files, Atria provides a single, blazing-fast, and online-first space with offline reading designed for **distraction-free reading and deep thinking**.

> [!WARNING]
> 🚧 **Work in Progress** > Atria is currently under active development as an online-first app with offline reading support.


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
  help        Help about any command
  link        Knowledge graph and entity relations management
  note        Knowledge base and markdown notes management
  rss         RSS feed management and triage
  tag         Tag management for all entities
  user        User management

Flags:
  -h, --help      help for atria
  -v, --version   version for atria

Use "atria [command] --help" for more information about a command.
```

</details>

## Dev and compilation

Go package: `golang` (Fedora 43)

Run tests:

```console
go test ./... -p 1
go test ./... -p 1 -v    # verbose
```

Build app:

```console
go build -o atria cmd/atria/*.go
```
