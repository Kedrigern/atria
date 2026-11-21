# Collections and sharing

Status: idea. Nothing in this document is built yet. The decisions in §8 are open.

## 1. Problem

A user wants to give another user of the same Atria a selection of items. Examples are all e-books by one author, or the manuscripts of one book series. Today this is not possible.

Each entity has one visibility:

| Visibility | Who can read it |
| --- | --- |
| `private` | The owner. |
| `users` | Every user of this Atria. |
| `public` | Everyone with the link, also without a login. Only notes and articles have a public page. |

There is no way to share an item with one user. Only notes and articles have the visibility setting in the web interface. E-books, manuscripts and tables do not have it.

Until PR #72, the level `users` worked only in search and for attachments: another user found the item but its page answered "not found". PR #72 adds a read-only page for notes and articles of other users.

## 2. Concept

A **collection** is a named, ordered list of items of any type: e-books, manuscripts, notes, tables, articles. It is an entity of its own (type `collection`), with a title, a description and a cover.

* One item can be in many collections. A collection does not move or copy its items.
* The owner can **share** a collection with chosen users of the same Atria. They can then read every item in it.
* Folders stay as they are. A folder puts each note or table in one place. A collection is a selection for reading or sharing.

Example: the user filters the e-books by the author "Max Scheler", selects "Add all to collection", creates the collection "Scheler" and shares it with Martin. Martin sees "Scheler" under "Shared with me" and can read and download the e-books.

## 3. Other options

| Option | Why not |
| --- | --- |
| Share single items with a user | The setting is on every item. Sharing 30 e-books takes 30 steps, and nobody sees afterwards what is shared with whom. |
| Share folders | E-books and manuscripts outside the notes tree have no folders. One item cannot be in two shared folders, for example "for father" and "for my sister". |
| Share tags | Tags already work across types and many to many. But a tag name is a short slug (`a-z0-9-`, 20 characters). A tag has no description or order. If tags share, a new tag on an item silently shares the item. |
| Use visibility `users` | It shares with every user of this Atria, not with one person. |

A collection can later replace the per-item `users` level: a collection shared with every user does the same thing.

## 4. Smart collections

A smart collection holds the items that match a rule, for example "e-books by author X" or "notes with the tag `vejvanov`". New items join it automatically.

This is useful, but risky together with sharing: a new item becomes visible to other users without a step that the owner sees. The first version has only static collections. The action "Add all to collection" on a filtered list covers the common case. A smart collection can come later, maybe only for collections that are not shared.

## 5. Access rule

Today almost every read in Atria has the condition `owner_id = <logged-in user>`. If each of these queries also checks sharing, one missed query leaks data.

Instead, one central rule decides who can read an entity:

1. The owner.
2. If the visibility is `users` or `public`: every user.
3. A user to whom the owner shared a collection that contains the entity.

The rule is one SQL function, for example `can_read(entity_id, user_id)`, and one Go function that calls it. Search, the read-only pages and the attachment download use it. The owner's pages and all writes keep their owner check.

A reader gets read-only pages, like the shared page from PR #72. Edits of shared items, for example of a manuscript of another user, are a possible later step.

## 6. Data

```sql
-- entity type 'collection' in entities: title, owner, visibility, trash
CREATE TABLE collections (
    id UUID PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    description TEXT,
    cover_attachment_id UUID REFERENCES attachments(id) ON DELETE SET NULL
);

CREATE TABLE collection_items (
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    position DOUBLE PRECISION NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, entity_id)
);

CREATE TABLE collection_shares (
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(10) NOT NULL DEFAULT 'read' CHECK (role IN ('read')),
    shared_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, user_id)
);
```

* An item in the trash or deleted leaves the collections. A collection in the trash shares nothing.
* Only the owner of an item can add it to a collection. A collection cannot share items of other users.

## 7. Interface

* **Collections** section in the menu and in the profile settings (§6 of `manuscripts.md`).
* Collection page: title, description, items in their order, and for the owner "Share with" (a list of users with a remove button).
* On each item page: "Add to collection" in the sidebar, next to tags.
* On filtered lists (e-books by author, manuscripts in a folder): "Add all to collection".
* **Shared with me**: the collections that other users shared with the logged-in user, with the name of the owner.
* CLI: `atria collection create|add|rm|list|share|unshare`.

## 8. Open questions

* Read only, or also edit? Recommendation: read only first.
* Static or smart collections? Recommendation: static first, see §4.
* Which types first? Recommendation: e-books and manuscripts, then notes, tables and articles.
* Does a reader see the tags and links of a shared item? They belong to the owner and can be private.
* Does the per-item visibility `users` stay, or does a collection shared with all users replace it?

## 9. Steps

1. Central access rule (§5) for the existing levels `users` and `public`, with search, read-only pages and attachments using it. No visible change.
2. Collections without sharing: data, service, collection page, "Add to collection", CLI.
3. Sharing: `collection_shares`, "Share with", "Shared with me", the access rule reads the shares.
4. "Add all to collection" on filtered lists.
5. Later: smart collections, editing rights.
