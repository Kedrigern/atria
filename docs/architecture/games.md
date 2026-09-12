# Cryptograms

## Goal

Add a lightweight, authenticated cryptogram game to Atria. A player solves a monoalphabetic substitution cipher: each distinct letter in a puzzle is displayed as a number, while spaces and punctuation remain visible.

The first version is deliberately small:

- Zen mode only: no timer, score, rewards, or arcade challenges;
- free keyboard input only;
- English, ASCII A–Z puzzles only;
- a small curated catalog of ten public-domain or attribution-noted quotes supplied with the application;
- persisted progress for each user and puzzle.

Text ingestion, Project Gutenberg import, LLM adaptation, difficulty telemetry, adaptive hints, card-style input, and speed runs are out of scope. They can be added without changing the core puzzle or progress model.

## Fit with Atria

Atria uses Go, Gin, PostgreSQL, server-rendered Go templates, HTMX, and small page-specific JavaScript. Game routes and form endpoints must use the existing authenticated route group and CSRF middleware.

A cryptogram is not an `entities` subtype in the initial version:

- it is application catalog data rather than user-authored knowledge content;
- it does not need Atria's title/slug/path, tag, attachment, link, visibility, or soft-delete machinery;
- game definitions can be shared by all authenticated users, while progress is private to each user.

All game tables use the `game_` prefix. IDs follow Atria's UUIDv7 convention, including catalog and progress records. The database continues to enforce foreign keys and cascade rules; no ORM is introduced.

## Data model

### `game_cryptograms`

The shared catalog stores plaintext and display metadata. It must never contain a precomputed number mapping: mapping codes are different per user session.

```sql
CREATE TABLE game_cryptograms (
    id UUID PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    category VARCHAR(150),
    order_index INTEGER,
    language VARCHAR(10) NOT NULL DEFAULT 'en'
        CHECK (language = 'en'),

    -- A manually curated, normalized source sentence or short quote.
    -- `plain_text` preserves punctuation and spaces for rendering.
    plain_text TEXT NOT NULL,
    intro_text TEXT,       -- Brief source context or a player-facing hint before solving.
    completion_text TEXT,  -- Attribution or background revealed after completion.

    -- Optional initial solution letters, such as ARRAY['E']. Empty by default.
    initial_revealed_letters VARCHAR(1)[] NOT NULL DEFAULT '{}',
    difficulty_score SMALLINT NOT NULL DEFAULT 3
        CHECK (difficulty_score BETWEEN 1 AND 5),

    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CHECK (char_length(btrim(plain_text)) > 0),
    CHECK (order_index IS NULL OR order_index >= 0)
);

CREATE INDEX idx_game_cryptograms_category_order
    ON game_cryptograms(category, order_index);
CREATE INDEX idx_game_cryptograms_difficulty
    ON game_cryptograms(difficulty_score);
```

The application validates `plain_text` before insertion: it accepts only printable ASCII characters and requires at least one `A`–`Z` letter after uppercasing. Persisting the readable sentence makes punctuation, contractions, and word boundaries available to the UI. It also avoids the redundant `original_text`/`clean_text` fields proposed in the original design.

### `game_cryptogram_progress`

One record represents one user's persistent attempt at one puzzle. `cipher_mapping` is an object whose keys are numeric codes and whose values are solution letters, for example `{"1":"E","2":"T"}`. It is generated once when the user first opens the puzzle and is retained through refreshes and later visits.

```sql
CREATE TABLE game_cryptogram_progress (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cryptogram_id UUID NOT NULL REFERENCES game_cryptograms(id) ON DELETE CASCADE,

    -- Per-user shuffled code -> solution-letter mapping. Never expose this field to the browser.
    cipher_mapping JSONB NOT NULL,

    -- Player's current code -> guessed-letter assignments.
    guesses JSONB NOT NULL DEFAULT '{}'::jsonb,
    hints_used SMALLINT NOT NULL DEFAULT 0 CHECK (hints_used >= 0),
    moves_count INTEGER NOT NULL DEFAULT 0 CHECK (moves_count >= 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, cryptogram_id),
    CHECK (jsonb_typeof(cipher_mapping) = 'object'),
    CHECK (jsonb_typeof(guesses) = 'object'),
    CHECK (completed_at IS NULL OR completed_at >= started_at)
);

CREATE INDEX idx_game_cryptogram_progress_user_updated
    ON game_cryptogram_progress(user_id, updated_at DESC);
CREATE INDEX idx_game_cryptogram_progress_user_completed
    ON game_cryptogram_progress(user_id, completed_at)
    WHERE completed_at IS NULL;
```

`cipher_mapping` belongs in progress, rather than being recomputed for every request. A randomly regenerated mapping would invalidate saved guesses and let a refresh change a puzzle already in progress. The catalog stays mapping-free, while every user receives a stable, independent mapping.

The service must validate JSON in Go before storing it:

- codes are unique positive integers;
- mapping values are uppercase `A`–`Z` and each plaintext letter maps to exactly one code;
- guesses use existing mapping codes and uppercase `A`–`Z` values;
- a user may not submit a guess for an initial reveal;
- a completed puzzle cannot accept further guesses.

## Puzzle generation and lifecycle

1. The player opens a catalog puzzle.
2. In one transaction, the service reads existing progress or creates it with a randomly shuffled code assignment for every distinct letter in `plain_text`.
3. The service applies `initial_revealed_letters` as correct immutable guesses.
4. The server returns display tokens only. A letter token contains its number code and the player-visible guessed value, if any. It never returns the plaintext letter or `cipher_mapping`.
5. The browser selects a code and submits one letter assignment.
6. The server validates ownership and the assignment against the secret mapping. On success it stores the guess and increments `moves_count`; on failure it returns a validation error without changing stored guesses.
7. If all codes are correctly assigned, the server sets `completed_at` and returns the completed puzzle state.

A code is a label for a plaintext letter, not a separate character instance. Assigning a letter to one code updates all matching positions. A submitted letter must also be rejected if it is already assigned to another code; this preserves the one-to-one substitution rule.

## Web interface and routes

The game uses a standalone `games` feature package and a page-specific `cryptogram.js` file. HTMX remains appropriate for navigation and server responses; plain JavaScript handles responsive keyboard interaction, code selection, and rendering updates.

| Route | Purpose |
| --- | --- |
| `GET /games` | List available cryptograms and the current user's completion state. |
| `GET /games/cryptograms/:id` | Render the playable puzzle page; create or load its progress. |
| `POST /api/games/cryptograms/:id/guesses` | Validate and persist a guess. Return the updated puzzle state or an error. |
| `POST /api/games/cryptograms/:id/reset` | Delete only the current user's progress, then redirect to a newly initialized attempt. |

All routes belong to the existing authenticated group, so they inherit `AuthMiddleware` and `CSRFMiddleware`. Queries must include `user_id` for every progress read or write; a puzzle ID by itself must never authorize access to another player's state.

The game page should provide:

- a readable title, category, difficulty, and a reset control;
- a word-wrapped cipher grid that preserves spaces and punctuation;
- a visibly selected number code and all of its occurrences highlighted;
- an on-screen A–Z keyboard, with physical keyboard support;
- immediate, accessible feedback for accepted and rejected guesses;
- a completion state and a link to the next unfinished puzzle.

The navigation receives a **Games** entry and the home dashboard receives a compact game card. Styling belongs in the existing `style.css`; only game-specific behavior belongs in `cryptogram.js`. Both static asset cache version and asset list in `sw.js` must be updated when adding the new JavaScript file.

## Package and test layout

```text
internal/games/
  service.go                # catalog, progress, secure puzzle projection, guessing, reset
  service_test.go           # mapping, validation, completion, isolation tests
internal/web/
  handlers_games.go         # HTTP request/response and template rendering
  handlers_games_test.go    # authentication, ownership, routes, handler outcomes
  static/cryptogram.js
  templates/pages/games_list.html
  templates/pages/cryptogram_play.html
internal/database/migrations/
  <timestamp>_add_games.sql
```

The migration creates the two `game_` tables and indexes. Initial catalog content should be introduced separately from the schema migration, either through an idempotent seed command or a deliberately reviewed data migration.

Minimum automated coverage:

- a new attempt receives a stable mapping and a different user's attempt cannot be read or changed;
- punctuation is preserved and every occurrence of a code is rendered consistently;
- correct, incorrect, duplicate, revealed, invalid, and post-completion guesses behave correctly;
- completion is saved once all codes are resolved;
- reset affects only the requesting user's progress;
- all game endpoints require authentication and reject requests without a valid CSRF token.

## Deferred design

When ETL is introduced, it can populate `game_cryptograms` with normalized chunks plus source metadata (`source_type`, `source_ref`, `is_llm_adapted`, and `llm_model`). Difficulty metrics may be added after real catalog data exists. Do not add speculative telemetry columns now.

Speed Run / Arcade mode should be modeled as an explicit `mode` and a separate attempt/challenge table rather than overloading the Zen progress record. Its timers, challenge rewards, error metrics, and scoring rules have different lifecycle and integrity requirements.

Card-style input is a frontend alternative over the same secure guess endpoint. It does not require a schema change.

## Decision needed before implementation

The initial catalog is seeded by the games migration with ten manually curated English quotes. Each entry has `intro_text` for short context or a hint during play and `completion_text` for attribution or background after the player completes it.

When the catalog grows, move seed content to an idempotent seed command so migrations remain schema-only. If the catalog must instead be user-authored, `game_cryptograms` needs an `owner_id` and a creation/management interface; that is a different product scope.
