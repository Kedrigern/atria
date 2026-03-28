# Atria - V0 Plan

## 1. Goal

Ship a small but complete foundation milestone that proves Atria's core technical direction works end-to-end.

V0 is not intended to deliver product features like RSS, notes, or read-it-later yet. Its purpose is to establish a stable base for future development.

## 2. What V0 Should Prove

At the end of V0, the project should demonstrate that:

- the Go project structure is workable
- the Gin server starts and serves HTML pages correctly
- PostgreSQL connectivity is reliable
- database migrations are in place
- user authentication works
- sessions protect authenticated routes
- logging is present and useful
- the app can render a simple authenticated screen backed by real data
- the main `atria` CLI can manage database setup and basic user administration
- the database and user workflow can be exercised through repeatable tests against a dedicated test database

## 3. Scope

### In Scope
- Go module and project structure
- Gin web server
- configuration loading from environment
- PostgreSQL connection
- migration setup
- `users` table
- password hashing
- login page
- logout flow
- session-based authentication
- one public page
- one authenticated page
- application and request logging
- updating `last_login_at` on successful login
- a main `atria` CLI binary
- `atria db ping`
- `atria db migrate`
- `atria db drop --force`
- `atria user add`
- `atria user list`
- `atria user show`
- `atria user role`
- a dedicated test database workflow for CLI and database integration tests

### Out of Scope
- entities
- notes
- RSS
- read-it-later
- article extraction
- tags
- backlinks
- attachments
- search
- offline reading
- EPUB export
- public sharing
- collaboration features

## 4. Deliverables

### 4.1. Project Bootstrap
Create the initial application structure, including:
- Go module
- entrypoint for the web server
- main `atria` CLI entrypoint
- internal package layout
- template directory
- static assets directory
- configuration handling

### 4.2. Database Foundation
Set up:
- PostgreSQL connection on startup
- migration workflow
- startup validation that the database is reachable
- CLI database commands for connectivity, migration, and destructive reset in development/test environments

The initial schema should include:
- `users`
- `schema_migrations`
- any required session-related storage if sessions are persisted in the database

### 4.3. Authentication
Implement a simple authentication flow:
- login form
- password verification
- secure session cookie
- logout
- route protection middleware
- CLI user bootstrap and administration commands

A successful login should update:
- `last_login_at`

### 4.4. Basic Pages
Implement at least:
- public home page
- login page
- authenticated dashboard page

The dashboard does not need product features yet. It only needs to confirm that authentication, rendering, and database-backed user context are working.

### 4.5. Logging
Add basic logging for:
- application startup
- database connection success/failure
- incoming requests
- authentication success/failure
- unexpected server errors
- CLI database and user administration operations

## 5. Suggested Initial Structure

A possible starting structure:

- `cmd/atria`
- `cmd/web`
- `internal/config`
- `internal/db`
- `internal/auth`
- `internal/http`
- `internal/logging`
- `internal/users`
- `migration`
- `web/templates`
- `web/static`

This does not need to be final, but it should be clean and consistent.

## 6. Suggested Minimal Data Model

### Users
V0 should have a minimal `users` table with fields such as:
- `id`
- `username`
- `email`
- `password_hash`
- `role`
- `last_login_at`
- `created_at`
- `updated_at`

This is enough to support authentication, basic user visibility in the app, and simple admin/user distinction in the CLI.

## 7. Acceptance Criteria

V0 is complete when all of the following are true:

- the application starts successfully in local development
- the server connects to PostgreSQL
- migrations can be applied cleanly
- a user can log in with valid credentials
- invalid login is handled correctly
- authenticated routes require a valid session
- logout clears the session
- the dashboard shows authenticated user information
- `last_login_at` is updated after successful login
- `atria db ping` works against the configured database
- `atria db migrate` applies schema changes from the `migration/` directory
- `atria db drop --force` resets application tables in the development/test database
- `atria user add`, `list`, `show`, and `role` work against the configured database
- the main CLI workflow is covered by integration tests using a dedicated test database
- logs are visible and useful during normal development

## 8. Non-Goals

V0 should avoid taking on product complexity too early.

It is intentionally not the place to:
- model the full content system
- build the first note editor
- add HTMX-heavy interactions beyond the basic authenticated shell
- design the offline system
- implement background workers
- start the article pipeline

Those come after the foundation is stable.

## 9. Next Step After V0

After V0 is complete, the next milestone should introduce the first real domain model, likely:

- `entities`
- a first content type such as `notes`
- basic CRUD flows
- protected application shell for authenticated users

## 10. Guiding Principle

V0 should be boring in the best possible way.

Its job is to reduce uncertainty, validate the stack, and create a clean base for the real product work that follows.