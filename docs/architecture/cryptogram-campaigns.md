# Cryptogram campaigns

Status: proposed.

## 1. Goal

A campaign is one short story, told in 3 to 10 cryptograms. You solve the cryptograms in the order of the story. After each solved cryptogram, you can read all solved parts of the story as one text. This text also helps with the next cryptogram, because names and words come back.

The first campaigns come from "Fifty Famous Stories Retold" by James Baldwin (1896). The book is in the public domain and is Project Gutenberg eBook #18442. The book has 50 stories. The start has five of them:

- King Alfred and the Cakes
- Bruce and the Spider
- Androclus and the Lion
- The Sword of Damocles
- The Blind Men and the Elephant

More stories can come later with the same script and the same import.

These items are not in this design:

- Campaigns in other languages. The cryptogram table allows only `en`.
- Campaigns that users create in the web interface. The CLI imports campaigns.
- Scores, statistics, or a daily puzzle.

## 2. Terms

| Term | Meaning |
|---|---|
| Book | The source of campaigns, for example "Fifty Famous Stories Retold" |
| Campaign | One story of a book, for example "King Alfred and the Cakes" |
| Passage | One cryptogram: one to three consecutive sentences of the story |
| Story so far | All solved passages of a campaign, in order, as one text |
| Topic | A kind of content, for example quotes, fairy tales, history, or literature |

A campaign has 3 to 10 passages. The first passage starts the story and the last passage ends it. A short story needs fewer passages than a long one.

## 3. The flow

1. A script makes a draft file from the book.
2. A person reads the draft, keeps the stories that work well, and corrects the passages.
3. The CLI imports the file into the database.
4. In the web interface, you choose a campaign and solve its passages in order. Only the next unsolved passage of a campaign is open.

The steps 1 and 2 are outside Atria. Atria knows only the campaign file. This is the same approach as for the PocketBase import: Atria has no code for one specific source.

## 4. The script

The script is `scripts/cryptogram-campaign.py`, a uv script like `scripts/ui-screenshots.py`. It does these steps:

1. It reads the metadata of the book from Gutendex (a JSON API for the catalog of Project Gutenberg). If Gutendex does not mark the book as free of copyright, the script stops.
2. It downloads the plain text once and keeps it in a cache directory. It does not crawl the website, because Project Gutenberg asks people not to.
3. It removes the header and the license text of Project Gutenberg.
4. It splits the text into stories. It uses the table of contents, so a heading such as "II. The Three Answers" stays in its story.
5. It joins the syllables that the book separates for young readers: "Eng-land" becomes "England". Some hyphens belong to the word, for example in "ill-natured". If both parts are words of the book and the joined word is not, the script keeps the hyphen.
6. It changes letters to ASCII, for example "Cæsar" to "Caesar".
7. It selects 3 to 10 passages for each story (section 5).
8. It writes a draft file (section 6). The option `--story` limits the draft to the stories that you name.

## 5. Selection of passages

The script cannot know which parts of a story are important. Thus it makes a draft, and a person makes the selection. The draft follows these rules:

- A passage has 40 to 200 letters and ends at the end of a sentence.
- A passage has at most one line of dialogue, unless the dialogue carries the point (see below).
- The first passage starts the story. The last passage ends it. The other passages follow the story in order, so the story so far reads as a story.
- The script makes about one passage for each 100 words of the story, at least 3 and at most 10. The option `--passages` sets a fixed number.
- A passage with many rare names gets a lower score, because it is hard to solve.

The draft marks each story with `"reviewed": false`. The import stops while a story has this mark. Thus no draft goes into the database by mistake.

The review makes sure that the story so far keeps the point of the story. Read the passages without the book. A reader must understand the turn of the story and why it ends as it does. The point is often in a line of dialogue, for example the words of the tyrant in "The Sword of Damocles". If the dialogue carries the point, the passage with the dialogue stays. The first campaigns lost their point in four of five stories, because the selection preferred narration.

## 6. The campaign file

One file holds the stories of one book:

```json
{
  "book": {
    "slug": "fifty-famous-stories",
    "title": "Fifty Famous Stories Retold",
    "author": "James Baldwin",
    "source_url": "https://www.gutenberg.org/ebooks/18442",
    "license": "Public domain in the USA",
    "topics": ["history", "legends"]
  },
  "campaigns": [
    {
      "slug": "king-alfred-and-the-cakes",
      "title": "King Alfred and the Cakes",
      "reviewed": true,
      "topics": ["history"],
      "passages": [
        "Many years ago there lived in England a wise and good king whose name was Alfred.",
        "The woman was baking some cakes upon the hearth.",
        "The cakes were smoking on the hearth. They were burned to a crisp."
      ]
    }
  ]
}
```

A campaign gets the topics of the book and its own topics. The file keeps the normal case of the letters. The import makes the uppercase text for the game and keeps the normal text for the story so far.

## 7. Data model

A new table holds the campaigns:

| Column | Type | Note |
|---|---|---|
| `id` | UUID | Primary key |
| `slug` | text | Unique: the book slug and the story slug, for example `fifty-famous-stories/king-alfred-and-the-cakes` |
| `title` | text | The title of the story |
| `book_title`, `book_author` | text | The games page groups the campaigns by book |
| `source_url`, `license` | text | Shown on the campaign page |
| `position` | integer | The position of the story in the book |
| `topics` | text array | For the topic filter (section 9) |
| `created_at` | timestamp | |

The table `game_cryptograms` gets three columns:

| Column | Type | Note |
|---|---|---|
| `campaign_id` | UUID, null | Null for a single puzzle |
| `display_text` | text, null | The passage in normal case, for the story so far |
| `topics` | text array | Topics of a single puzzle. A passage uses the topics of its campaign |

The migration gives the existing single puzzles the topic `quotes`, because all of them are quotes. The existing column `order_index` is the position of the passage in the campaign. A unique index on `(campaign_id, order_index)` keeps the order clear.

The progress table does not change. The first passage of a campaign is always open. A solved passage opens the next passage of the same campaign.

## 8. Import

```console
atria games import fifty-famous-stories.json
atria games import fifty-famous-stories.json --replace
```

The import runs in one transaction. It stops in these cases:

- A story is not reviewed.
- A campaign has fewer than 3 or more than 10 passages.
- A passage has fewer than 40 or more than 200 letters.
- A passage has letters outside A to Z after the change to ASCII.

The import adds the campaigns that are not in the database yet. Thus you can import five stories now and more stories later from the same file. A campaign that exists with the same passages stays as it is.

If a campaign exists with different passages, the import stops. With `--replace`, it deletes that campaign, its cryptograms, and the progress of all users on it. Then it imports the campaign again. Before the delete, the command shows how many users lose progress.

## 9. The web interface

The games page gets two tabs (section 6 of `ui.md`):

- Campaigns: the campaigns grouped by book. Each campaign shows its progress ("2 of 6 solved") and a button for the next passage.
- Single puzzles: the cryptograms without a campaign, as today.

Above the list, filter links show the topics, for example "All", "Quotes", "Fairy tales", "History". The filter is in the address (`/games?topic=history`), so it works without JavaScript. The active topic is bold and underlined, as on the RSS page.

In both tabs, the solved items go last, in a closed `details` element "Solved (N)". This works without JavaScript.

The campaign page shows:

- The title of the story, the book, the author, the source, and the progress.
- The button "Next passage", or the badge "Solved" at the end.
- The story so far: the solved passages in normal case, as one text.

The page of a passage shows its position, for example "King Alfred and the Cakes, passage 2 of 6". A link "Story so far" goes to the campaign page. After you solve the passage, the button "Next passage" replaces "Choose another puzzle". After the last passage, the button "Read the story" goes to the campaign page.

You cannot open a passage before you solve the passages before it. Atria then shows the campaign page with the message "Solve the earlier passages first."

## 10. Pull requests

1. The data model, the import, and the web interface. A small test campaign shows the flow.
2. The script and the reviewed file with the five stories.

## 11. Novels: books with chapters

Status: proposed, as an addition to sections 1 to 10.

A collection of short stories, such as "Fifty Famous Stories Retold", has campaigns that you can play in any order. A novel has chapters, and the chapters must come in order. Two novels come next:

| Book | Source | Chapters | Words |
|---|---|---|---|
| "Alice's Adventures in Wonderland", Lewis Carroll (1865) | Project Gutenberg #11, the original text | 12 | about 26,500 |
| "Robinson Crusoe, Told to the Children", John Lang (about 1905) | Project Gutenberg #67907, a retelling for children | 10 | about 21,000 |

The original "Robinson Crusoe" by Daniel Defoe is not a good source. Many of its sentences have more than 100 words, and a passage has at most 200 letters.

### 11.1 Three levels

A novel has three levels: the book, the chapter, and the passage. One chapter is one campaign. Thus the existing campaign pages and the story so far work for a chapter as they are.

A chapter of these books has about 2,000 words. Ten passages keep only the frame of a chapter. Thus the limits of the number of passages change:

- The script makes about one passage for each 100 words, at least 3 and at most 30. The option `--passages` sets another number.
- The import accepts 3 to 40 passages in a campaign. These limits are a safety net, not a target. The rule for a good campaign is that the story so far keeps the point (section 5).
- A passage still has 40 to 200 letters, because a longer passage does not fit on the screen of an e-ink reader.

### 11.2 Order of the chapters

The book in the campaign file gets the field `"in_order"`. If it is `true`, only the first campaign of the book is open at the start. A campaign opens after you solve all passages of the campaign before it. Inside a campaign, the passages open one by one, as before. A collection of stories has `"in_order": false`, which is the default.

A locked campaign shows on the games page with the text "Opens after chapter N" and without a button.

### 11.3 The book page

A novel gets a book page at `/games/books/<slug>`. It shows the title, the author, the source, the progress for each chapter, and the button "Continue" for the next passage. Below, it shows the solved passages of all chapters in order, with the chapter titles as headings. The page shows the current chapter first. The earlier chapters are in a closed `details` element "Earlier chapters", so the page stays short on an e-ink reader.

The games page links the title of a book to its book page.

### 11.4 Data model

A new table `game_books` holds the books: `id`, `slug` (unique), `title`, `author`, `source_url`, `license`, `in_order`, and `topics`. The table `game_campaigns` gets the column `book_id`, and the book columns move from `game_campaigns` to `game_books`. The migration makes one book for each book slug of the existing campaigns (the part of the campaign slug before the `/`). The existing campaigns keep their progress.

### 11.5 The script

The script gets the option `--chapters`. With it, the script splits the book at the headings "CHAPTER I", "CHAPTER II", and so on. The title of a chapter is the line after its heading. Without it, the script uses the table of contents, as for "Fifty Famous Stories Retold".

### 11.6 Selection

The selection of the passages is the same as in section 5. For each chapter, the review makes sure that the turn of the chapter and its end are in the passages. Many names in "Alice's Adventures in Wonderland" are rare words, for example "Caterpillar", "Gryphon", and "Mock Turtle". They are a part of the book and they stay. They make a passage harder, and a name that comes back helps in a later passage.

### 11.7 Pull requests

1. This design.
2. The data model with books, the order of the chapters, the book page, and the new limits.
3. The script with `--chapters`, and the reviewed file for "Alice's Adventures in Wonderland".
4. The reviewed file for "Robinson Crusoe, Told to the Children".

## 12. Decisions and open questions

- Must a hint in a campaign passage cost more than in a single puzzle? This design keeps hints the same.
- Topics are not the tags of Atria. Tags belong to one user and attach to that user's notes and articles. Cryptograms are a catalog that all users share. Thus the catalog has its own topics, and the tags of a user stay free of game content.
- Can the story so far also export to EPUB? Atria can make EPUB files, but this design does not include it.
