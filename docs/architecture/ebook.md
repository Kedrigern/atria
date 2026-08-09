# E-books

## Motivation

- Atria’s goal is to store knowledge and make it more accessible.
- An e-book provides a genuinely offline reading experience.
- Atria already converts one or more entities to EPUB through the CLI.

## Design

An e-book is a small, user-owned Atria entity with type `ebook`.

- The generated EPUB is attached to the e-book through the existing attachment relation.
- The e-book can use existing tags and entity links.
- No new file-storage or attachment mechanism is needed.
- A small e-book subtype table stores e-book-specific metadata such as description, author, language, cover image, generation timestamp, and the generated EPUB attachment.
- Source entities may be linked to the e-book using the existing entity-link relation for traceability.

Each generated e-book is immutable: it is a snapshot of its source entities at the time of generation. Source changes do not modify an existing e-book.

## E-book generation

- Initially, generation supports only articles and notes.
- Each selected entity becomes one chapter.
- The generator ignores invalid, missing, deleted, unauthorized, and unsupported entity IDs.

- The initial web form accepts one entity ID per line.
- The CLI and web form should offer the same generation options.

Generation options:

- title;
- description or annotation;
- author or publisher;
- language;
- cover image;
- output filename.

The generated EPUB should include useful source metadata where appropriate, including the source entity title, date, and tags. The e-book entity itself can also have tags.

## User interface

- Add an **E-books** page to the main navigation.
- The page lists e-book entities owned by the current user.
- Each item links to its metadata, source entities, and EPUB download.
- Add a page for creating an e-book.

PDF support is out of scope for the initial implementation.

## Implementation roadmap

### 1. Add the e-book entity type and database schema

- Add `ebook` to the entity type constraint.
- Add an `ebooks` subtype table linked to `entities`.
- Store e-book-specific metadata such as description, author, language, cover attachment ID, generation timestamp, and the generated EPUB attachment ID.
- Reuse existing entity links to associate source articles and notes with the e-book.

- Add database migration and migration tests.

### 2. Extend the CLI

- Add an `atria ebook` command group and service functions for creating, retrieving, listing, and deleting e-books.
- Add `atria ebook create --title <title> <identifier>...` to generate an immutable EPUB snapshot.
- Add `atria ebook list` to show generated e-books, their EPUB file, and source count.
- Add `atria ebook download <identifier> --out <path>` to retrieve a generated EPUB.
- Support title, description, author, language, cover image, and output filename options.
- Ignore invalid, missing, deleted, unauthorized, and unsupported IDs.
- Create the e-book entity, attach the generated EPUB, and link the source entities.
- Add unit and integration tests for chapter order, metadata, and source filtering.

### 3. Add web pages

- Add an **E-books** item to the main navigation.
- Add an e-book library page with metadata, source links, download, and deletion actions.
- Add an e-book creation page that initially accepts one entity ID per line.
- Reuse the same generation service and validation rules as the CLI.
- Add clear success and error feedback for generation failures.
