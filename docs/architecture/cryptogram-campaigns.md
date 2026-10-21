# Cryptogram campaigns

Status: proposed.

## 1. Goal

A campaign is a series of cryptograms from one book. You solve the cryptograms in the order of the book. After each solved cryptogram, you can read all solved parts of the campaign as one text. This text also helps with the next cryptogram, because names and words come back.

The first campaign comes from "Fifty Famous Stories Retold" by James Baldwin (1896). The book is in the public domain and is Project Gutenberg eBook #18442.

These items are not in this design:

- Campaigns in other languages. The cryptogram table allows only `en`.
- Campaigns that users create in the web interface. The CLI imports campaigns.
- Scores, statistics, or a daily puzzle.

## 2. Terms

| Term | Meaning |
|---|---|
| Campaign | All cryptograms from one book, in a fixed order |
| Chapter | One story of the book, for example "King Alfred and the Cakes" |
| Passage | One cryptogram: one to three consecutive sentences of a chapter |
| Story so far | All solved passages of a campaign, in order, as one text |

A chapter has three to five passages. Thus the first campaign has about 200 cryptograms.

## 3. The flow

1. A script makes a draft of the campaign from the book.
2. A person reads the draft, deletes the weak passages, and corrects the text.
3. The CLI imports the campaign file into the database.
4. In the web interface, you solve the passages in order. Only the next unsolved passage is open.

The steps 1 and 2 are outside Atria. Atria knows only the campaign file. This is the same approach as for the PocketBase import: Atria has no code for one specific source.

## 4. The script

The script is `scripts/cryptogram-campaign.py`, a uv script like `scripts/ui-screenshots.py`. It does these steps:

1. It reads the metadata of the book from Gutendex (a JSON API for the catalog of Project Gutenberg). If Gutendex does not mark the book as free of copyright, the script stops.
2. It downloads the plain text once and keeps it in a cache directory. It does not crawl the website, because Project Gutenberg asks people not to.
3. It removes the header and the license text of Project Gutenberg.
4. It splits the text into chapters. It uses the table of contents, so a heading such as "II. The Three Answers" stays in its chapter.
5. It joins the syllables that the book separates for young readers: "Eng-land" becomes "England". Some hyphens belong to the word, for example in "ill-natured". If both parts are words of the book and the joined word is not, the script keeps the hyphen.
6. It changes letters to ASCII, for example "Cæsar" to "Caesar".
7. It selects passages for each chapter (section 5).
8. It writes a draft campaign file (section 6).

## 5. Selection of passages

The script cannot know which parts of a story are important. Thus it makes a draft, and a person makes the selection. The draft follows these rules:

- A passage has 40 to 200 letters and ends at the end of a sentence.
- A passage has at most one line of dialogue.
- The passages of a chapter are consecutive in the book where possible, so the story so far reads as a story.
- The first passage of a chapter starts the story. The last passage ends it.
- A passage with many rare names gets a lower score, because it is hard to solve.

The draft marks each chapter with `"reviewed": false`. The import stops while a chapter has this mark. Thus no draft goes into the database by mistake.

## 6. The campaign file

The file is JSON:

```json
{
  "slug": "fifty-famous-stories",
  "title": "Fifty Famous Stories Retold",
  "author": "James Baldwin",
  "source_url": "https://www.gutenberg.org/ebooks/18442",
  "license": "Public domain in the USA",
  "chapters": [
    {
      "title": "King Alfred and the Cakes",
      "reviewed": true,
      "passages": [
        "Many years ago there lived in England a wise and good king whose name was Alfred.",
        "The cakes were smoking on the hearth. They were burned to a crisp."
      ]
    }
  ]
}
```

The file keeps the normal case of the letters. The import makes the uppercase text for the game and keeps the normal text for the story so far.

## 7. Data model

A new table holds the campaigns:

| Column | Type | Note |
|---|---|---|
| `id` | UUID | Primary key |
| `slug` | text | Unique. The import uses it to find an existing campaign |
| `title`, `author` | text | |
| `source_url`, `license` | text | Shown on the campaign page |
| `created_at` | timestamp | |

The table `game_cryptograms` gets three columns:

| Column | Type | Note |
|---|---|---|
| `campaign_id` | UUID, null | Null for a single puzzle |
| `chapter_title` | text, null | The title of the story |
| `display_text` | text, null | The passage in normal case, for the story so far |

The existing column `order_index` is the position in the campaign. A unique index on `(campaign_id, order_index)` keeps the order clear.

The progress table does not change. The first passage is always open. Each later passage opens when the user solves the passage before it.

## 8. Import

```console
atria games import fifty-famous-stories.json
atria games import fifty-famous-stories.json --replace
```

The import runs in one transaction. It stops in these cases:

- A chapter is not reviewed.
- A passage has fewer than 40 or more than 200 letters.
- A passage has letters outside A to Z after the change to ASCII.

If a campaign with the same slug exists, the import stops. With `--replace`, it deletes the campaign, its cryptograms, and the progress of all users, and then imports the file again. The command shows how many users lose progress before it deletes.

## 9. The web interface

The games page gets two tabs (section 6 of `ui.md`):

- Campaigns: one card for each campaign, with the progress ("12 of 198 solved") and a button for the next passage.
- Single puzzles: the cryptograms without a campaign, as today.

In both tabs, the solved items go last, in a closed `details` element "Solved (N)". This works without JavaScript.

The campaign page shows:

- The title, the author, the source, and the progress.
- The button "Next passage", or "Campaign solved" at the end.
- The story so far: the solved passages in normal case, with the chapter titles as headings. If the text is long, the page shows the last chapter first and the older chapters in a closed `details` element "Earlier chapters". This keeps the page short on an e-ink reader.

The page of a passage shows the chapter title and its position, for example "King Alfred and the Cakes, passage 2 of 4". A link "Story so far" goes to the campaign page. After you solve the passage, the button "Next passage" replaces "Choose another puzzle".

You cannot open a passage before you solve the passages before it. Atria then shows the campaign page with the message "Solve the earlier passages first."

## 10. Pull requests

1. The data model, the import, and the web interface. A small test campaign shows the flow.
2. The script and the reviewed campaign file for "Fifty Famous Stories Retold".

## 11. Open questions

- Must a hint in a campaign passage cost more than in a single puzzle? This design keeps hints the same.
- Can the story so far also export to EPUB? Atria can make EPUB files, but this design does not include it.
