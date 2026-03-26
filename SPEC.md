# Atria - Product & Technical Specification

## 1. Overview
**Atria** is a personal mind palace and collaborative knowledge base. It unifies a Read-it-Later service, an RSS reader, Markdown-based notes, and basic spreadsheets into a single, blazing-fast, distraction-free environment. 

While designed for absolute personal focus, Atria supports multi-user instances, allowing small teams or families to share an instance, share specific items, and mention each other in notes. The core philosophy of Atria is **focus and permanence**: removing cognitive overload, avoiding data lock-in, and preventing storage bloat while maintaining offline availability.

## 2. Core Modules

### 2.1. RSS Reader
* **Purpose:** Fast triage of external information.
* **Features:**
  * Automated background fetching of RSS/Atom feeds.
  * Skimmable list view (titles and excerpts only) for quick "keep or discard" decisions.
  * **Action:** One-click transfer of an interesting feed item to the "Read-it-Later" inbox, which triggers the backend to download and parse the full article.

### 2.2. Read-it-Later (Bookmarks & Articles)
* **Purpose:** Distraction-free reading and long-term archiving.
* **Workflow:** `Inbox` (to be read) ➔ `Archive` (read and saved).
* **Features:**
  * **Readability Extraction:** Automatically strips ads, menus, and popups, saving only the clean text, author, date, and main images.
  * **Full Web Archiving (WARC):** Generates and stores a complete, offline WARC (Web ARChive) file of the original page to ensure absolute permanence, even if the source website goes permanently offline.
  * **E-ink Export:** Select multiple articles and generate a `.epub` file for reading on Kindle/Kobo devices.
  * **Public Sharing:** Generate public, read-only links for specific articles.

### 2.3. Knowledge Base (Markdown Notes)
* **Purpose:** Personal and public note-taking and knowledge linking.
* **Features:**
  * Full Markdown support (including YAML frontmatter for metadata).
  * **Advanced Plugins:** Support for extended Markdown features, notably **Mermaid.js** for rendering complex diagrams, charts, and mind maps directly from text.
  * **Organization:** Standard folder hierarchy combined with flexible `#tags`.
  * **Inline Attachments:** Support for uploading and embedding images and PDF documents.
  * **User Mentions:** Type `@username` to tag other registered users on the instance. This creates a semantic link to their profile and can trigger in-app notifications.
  * **Zensical Sync (Hybrid Storage):** A dedicated folder in the database that bi-directionally syncs with a physical directory of `.md` files on the server, allowing the user to maintain a public GitHub-backed static site while editing it from the Atria mobile app.

### 2.4. Spreadsheets
* **Purpose:** Lightweight tabular data management.
* **Features:**
  * Basic grid interface with rows, columns, and simple formulas (SUM, AVERAGE).
  * Can be embedded or linked within Markdown notes.

## 3. Cross-Cutting Features

### 3.1. The Knowledge Graph (Backlinks)
Everything in Atria is connected. A dedicated linking engine parses internal links (e.g., `[[Note Name]]`) and maintains a graph. Every article, note, or spreadsheet displays a "Mentioned in" section at the bottom, creating an organic web of knowledge.

### 3.2. Users & Collaboration
* **Multi-User Instance:** A single Atria server can host multiple isolated accounts.
* **Mentions & Notifications:** Tagging a user (`@user`) in a Markdown document creates a backlink to that user and sends them an internal notification.
* **Access Control:** Users can keep items strictly private, share them with specific `@users` on the instance, or generate a public URL for the outside world.

### 3.3. Data Ingestion
* **Web Extension:** Captures the *rendered* HTML of the current page. This allows Atria to save articles behind paywalls, login screens, or corporate SSO (like Jira/Confluence) because the browser has already done the authentication work.
* **Mobile Share Target:** Fully integrated into the native Android/iOS share sheet via PWA Web Share Target API.

### 3.4. Offline-First PWA (Progressive Web App)
* Installable on mobile and desktop devices.
* Uses **IndexedDB** to store the database locally and **Service Workers** to cache UI assets and images.
* Allows reading saved articles and editing notes in subway tunnels or remote areas without internet access.

### 3.5. Command Line Interface (CLI)
* **Purpose:** Frictionless quick-capture and management for power users directly from the terminal.
* **Architecture:** A standalone Python CLI client (built with `Typer`) that communicates with the Atria FastAPI backend via REST API.
* **Core Commands:**
  * `atria add <url>`: Instantly saves and parses an article for Read-it-Later.
  * `atria note "content" --tags`: Quick-captures a Markdown note without opening the GUI.
  * `atria search <query>`: Performs a full-text search across all items and returns formatted terminal output.
  * `atria sync`: Triggers the Zensical directory synchronization manually.
  * `atria prune`: Forces the garbage collection process to free up disk space.

## 4. Technical Architecture

### 4.1. Technology Stack
* **Backend API:** Python + FastAPI (Asynchronous, high-performance, automatic Swagger UI).
* **CLI Client:** Python + Typer.
* **Database:** PostgreSQL (Ideal for structural data, relationships, and full-text search).
* **Frontend:** Vue.js / Svelte (SPA compiled to a PWA).

### 4.2. Storage & Anti-Bloat Strategy
To prevent the application from bloating to gigabytes of data due to automated web scraping and WARC files, Atria employs strict storage management:
* **Separation of Concerns:** PostgreSQL stores only text, metadata, and references. Binary files (images, PDFs, WARC archives) are saved directly to the server's local file system.
* **Deduplication:** Every incoming image is hashed (SHA-256). Duplicate images (e.g., repeating site logos) are only saved once.
* **Aggressive Compression:** All downloaded images are automatically converted to `WebP` or `AVIF` and resized (e.g., max-width 800px).
* **Workflow Pruning (Garbage Collection):** When an article is moved from the `Inbox` to the `Archive`, the system deletes its associated images from the disk to save space, keeping only the highly compressible text and a single cover image. (WARC files can be configured to auto-delete after X days or be kept permanently).
* **X-Accel-Redirect / X-Sendfile:** The backend handles authentication for privat# Atria - Product & Technical Specification

## 1. Overview
**Atria** is a personal mind palace and collaborative knowledge base. It unifies a Read-it-Later service, an RSS reader, Markdown-based notes, and basic spreadsheets into a single, blazing-fast, distraction-free environment. 

While designed for absolute personal focus, Atria supports multi-user instances, allowing small teams or families to share an instance, share specific items, and mention each other in notes. The core philosophy of Atria is **focus and permanence**: removing cognitive overload, avoiding data lock-in, and preventing storage bloat while maintaining offline availability.

## 2. Core Modules

### 2.1. RSS Reader
* **Purpose:** Fast triage of external information.
* **Features:**
  * Automated background fetching of RSS/Atom feeds.
  * Skimmable list view (titles and excerpts only) for quick "keep or discard" decisions.
  * **Action:** One-click transfer of an interesting feed item to the "Read-it-Later" inbox, which triggers the backend to download and parse the full article.

### 2.2. Read-it-Later (Bookmarks & Articles)
* **Purpose:** Distraction-free reading and long-term archiving.
* **Workflow:** `Inbox` (to be read) ➔ `Archive` (read and saved).
* **Features:**
  * **Readability Extraction:** Automatically strips ads, menus, and popups, saving only the clean text, author, date, and main images.
  * **Full Web Archiving (WARC):** Generates and stores a complete, offline WARC (Web ARChive) file of the original page to ensure absolute permanence, even if the source website goes permanently offline.
  * **E-ink Export:** Select multiple articles and generate a `.epub` file for reading on Kindle/Kobo devices.
  * **Public Sharing:** Generate public, read-only links for specific articles.

### 2.3. Knowledge Base (Markdown Notes)
* **Purpose:** Personal and public note-taking and knowledge linking.
* **Features:**
  * Full Markdown support (including YAML frontmatter for metadata).
  * **Advanced Plugins:** Support for extended Markdown features, notably **Mermaid.js** for rendering complex diagrams, charts, and mind maps directly from text.
  * **Organization:** Standard folder hierarchy combined with flexible `#tags`.
  * **Inline Attachments:** Support for uploading and embedding images and PDF documents.
  * **User Mentions:** Type `@username` to tag other registered users on the instance. This creates a semantic link to their profile and can trigger in-app notifications.
  * **Zensical Sync (Hybrid Storage):** A dedicated folder in the database that bi-directionally syncs with a physical directory of `.md` files on the server, allowing the user to maintain a public GitHub-backed static site while editing it from the Atria mobile app.

### 2.4. Spreadsheets
* **Purpose:** Lightweight tabular data management.
* **Features:**
  * Basic grid interface with rows, columns, and simple formulas (SUM, AVERAGE).
  * Can be embedded or linked within Markdown notes.

## 3. Cross-Cutting Features

### 3.1. The Knowledge Graph (Backlinks)
Everything in Atria is connected. A dedicated linking engine parses internal links (e.g., `[[Note Name]]`) and maintains a graph. Every article, note, or spreadsheet displays a "Mentioned in" section at the bottom, creating an organic web of knowledge.

### 3.2. Users & Collaboration
* **Multi-User Instance:** A single Atria server can host multiple isolated accounts.
* **Mentions & Notifications:** Tagging a user (`@user`) in a Markdown document creates a backlink to that user and sends them an internal notification.
* **Access Control:** Users can keep items strictly private, share them with specific `@users` on the instance, or generate a public URL for the outside world.

### 3.3. Data Ingestion
* **Web Extension:** Captures the *rendered* HTML of the current page. This allows Atria to save articles behind paywalls, login screens, or corporate SSO (like Jira/Confluence) because the browser has already done the authentication work.
* **Mobile Share Target:** Fully integrated into the native Android/iOS share sheet via PWA Web Share Target API.

### 3.4. Offline-First PWA (Progressive Web App)
* Installable on mobile and desktop devices.
* Uses **IndexedDB** to store the database locally and **Service Workers** to cache UI assets and images.
* Allows reading saved articles and editing notes in subway tunnels or remote areas without internet access.

## 4. Technical Architecture

### 4.1. Technology Stack
* **Backend:** Python + FastAPI (Asynchronous, high-performance, automatic Swagger UI).
* **Database:** PostgreSQL (Ideal for structural data, relationships, and full-text search).
* **Frontend:** Vue.js / Svelte (SPA compiled to a PWA).

### 4.2. Storage & Anti-Bloat Strategy
To prevent the application from bloating to gigabytes of data due to automated web scraping and WARC files, Atria employs strict storage management:
* **Separation of Concerns:** PostgreSQL stores only text, metadata, and references. Binary files (images, PDFs, WARC archives) are saved directly to the server's local file system.
* **Deduplication:** Every incoming image is hashed (SHA-256). Duplicate images (e.g., repeating site logos) are only saved once.
* **Aggressive Compression:** All downloaded images are automatically converted to `WebP` or `AVIF` and resized (e.g., max-width 800px).
* **Workflow Pruning (Garbage Collection):** When an article is moved from the `Inbox` to the `Archive`, the system deletes its associated images from the disk to save space, keeping only the highly compressible text and a single cover image. (WARC files can be configured to auto-delete after X days or be kept permanently).
* **X-Accel-Redirect / X-Sendfile:** The backend handles authentication for private files and WARC downloads, but delegates the actual file serving to a reverse proxy (Nginx/Caddy) for blazing-fast performance.

## 5. High-Level Data Model

* **Users:** Stores authentication, profiles, and preferences.
* **Items (Polymorphic):** The core table storing metadata, tags, content, and `owner_id` for Articles, Notes, and Spreadsheets.
* **Attachments:** Stores `id`, `filename`, `file_hash`, `size`, `type` (image, pdf, warc), and `disk_path`.
* **Item_Attachments (Relations):** Tracks which item uses which attachment (Reference counting for Garbage Collection).
* **Item_Links (Backlinks):** Stores `source_id` and `target_id` to build the knowledge graph and enable the "Mentioned in" feature. Includes mentions of users (`target_id` = User).
* **Tags:** Global taxonomy system unifying all content types.
e files and WARC downloads, but delegates the actual file serving to a reverse proxy (Nginx/Caddy) for blazing-fast performance.

## 5. High-Level Data Model

* **Users:** Stores authentication, profiles, and preferences.
* **Items (Polymorphic):** The core table storing metadata, tags, content, and `owner_id` for Articles, Notes, and Spreadsheets.
* **Attachments:** Stores `id`, `filename`, `file_hash`, `size`, `type` (image, pdf, warc), and `disk_path`.
* **Item_Attachments (Relations):** Tracks which item uses which attachment (Reference counting for Garbage Collection).
* **Item_Links (Backlinks):** Stores `source_id` and `target_id` to build the knowledge graph and enable the "Mentioned in" feature. Includes mentions of users (`target_id` = User).
* **Tags:** Global taxonomy system unifying all content types.
