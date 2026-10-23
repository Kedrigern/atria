#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Make a draft cryptogram campaign file from a Project Gutenberg book.

See docs/architecture/cryptogram-campaigns.md. The draft marks every story
with "reviewed": false. A person reads it, corrects the passages and sets
"reviewed": true. Then `atria games import <file>` loads it.

Usage:
    uv run scripts/cryptogram-campaign.py 18442 --join-syllables --out fifty-famous-stories.json \\
        --story "King Alfred and the Cakes" --story "Bruce and the Spider"
    uv run scripts/cryptogram-campaign.py 11 --chapters --out alice.json      # a novel
    uv run scripts/cryptogram-campaign.py 18442 --list
    uv run scripts/cryptogram-campaign.py 18442 --text-dir stories/   # cleaned text of each story

The script downloads a book once and keeps it in --cache. It does not crawl
Project Gutenberg, which asks people not to access the site automatically.
"""

import argparse
import json
import re
import sys
import unicodedata
import urllib.request
from pathlib import Path

GUTENDEX = "https://gutendex.com/books/{id}/"
USER_AGENT = "atria-cryptogram-campaign/1 (one book, downloaded once)"
MIN_LETTERS, MAX_LETTERS = 40, 200
MIN_PASSAGES, MAX_PASSAGES = 3, 40
# The draft makes about one passage per 100 words, up to this number.
DRAFT_MAX_PASSAGES = 30
SMALL_WORDS = {"a", "an", "and", "as", "at", "by", "for", "in", "of", "on", "or", "the", "to", "with"}


def fail(message):
    print(f"error: {message}", file=sys.stderr)
    sys.exit(1)


def fetch(url):
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    try:
        # Gutendex can take a minute to answer.
        with urllib.request.urlopen(request, timeout=120) as response:
            return response.read()
    except (OSError, ValueError) as error:
        fail(f"cannot download {url}: {error}")


def load_book(book_id, cache):
    """Return (metadata, text) of a book. Both are cached."""
    cache.mkdir(parents=True, exist_ok=True)
    meta_path, text_path = cache / f"{book_id}.json", cache / f"{book_id}.txt"
    if not meta_path.exists():
        meta_path.write_bytes(fetch(GUTENDEX.format(id=book_id)))
    meta = json.loads(meta_path.read_text())
    if meta.get("copyright") is not False:
        fail(f"Gutendex does not mark book {book_id} as free of copyright")
    if not text_path.exists():
        formats = meta.get("formats", {})
        url = next((u for k, u in formats.items() if k.startswith("text/plain") and "utf-8" in k), None)
        url = url or next((u for k, u in formats.items() if k.startswith("text/plain")), None)
        if not url:
            fail(f"book {book_id} has no plain text")
        text_path.write_bytes(fetch(url))
    return meta, text_path.read_text(encoding="utf-8-sig")


def strip_gutenberg(text):
    """Remove the Project Gutenberg header and license."""
    start = re.search(r"\*\*\* ?START OF (THE|THIS) PROJECT GUTENBERG EBOOK[^\n]*\n", text)
    end = re.search(r"\*\*\* ?END OF (THE|THIS) PROJECT GUTENBERG EBOOK", text)
    if not start or not end:
        fail("the text has no Project Gutenberg START and END lines")
    return text[start.end():end.start()].replace("\r\n", "\n")


def contents(body):
    """Titles from the table of contents: the lines after CONTENTS, up to the next heading in capitals."""
    m = re.search(r"\nCONTENTS\.?\n(.*?)\n\n\n\n", body, re.DOTALL)
    if not m:
        fail("the book has no table of contents")
    return [line.strip() for line in m.group(1).split("\n") if line.strip()]


def heading_key(title):
    return re.sub(r"[^a-z]", "", ascii_text(title).lower())


def split_stories(body, titles):
    """Split the book at the headings of the stories in the table of contents.

    A heading that is not in the table of contents, for example "II. THE THREE
    ANSWERS.", stays in the story before it.
    """
    wanted = {heading_key(t): t for t in titles}
    stories, current = [], None
    for paragraph in re.split(r"\n\s*\n", body):
        line = " ".join(paragraph.split())
        if not line:
            continue
        key = heading_key(line)
        if line.isupper() and key in wanted:
            if current is not None and heading_key(current["title"]) == key:
                continue  # the same heading again, for example above a poem
            current = {"title": wanted[key], "paragraphs": []}
            stories.append(current)
        elif current is not None and not line.isupper() and not line.startswith("[Illustration"):
            current["paragraphs"].append(line)
    return stories


def title_case(text):
    """Make "HOW ROBINSON FIRST WENT TO SEA" into "How Robinson First Went to Sea"."""
    if not text.isupper():
        return text
    words = text.lower().split()
    return " ".join(w if i and w in SMALL_WORDS else w[:1].upper() + w[1:] for i, w in enumerate(words))


def split_chapters(body):
    """Split a novel at the headings "CHAPTER I", "CHAPTER II", and so on.

    The title of a chapter is the text after the heading, up to the first empty
    line, so a title on two lines is one title.
    """
    heading = re.compile(r"^\s*CHAPTER [IVXLC]+\.?\s*$", re.MULTILINE)
    starts = [m for m in heading.finditer(body)]
    if not starts:
        fail("the book has no headings \"CHAPTER I\", \"CHAPTER II\", ...")
    chapters = []
    for i, m in enumerate(starts):
        end = starts[i + 1].start() if i + 1 < len(starts) else len(body)
        text = body[m.end():end].strip("\n")
        # The end matter of a book is not a part of its last chapter.
        stop = re.search(r"^\s*(THE END\b|TRANSCRIBER|Transcriber|.*\bPrinted by\b)", text, re.MULTILINE)
        if stop:
            text = text[:stop.start()]
        title_block, _, rest = text.partition("\n\n")
        title = title_case(" ".join(title_block.split()).rstrip(".;,"))
        paragraphs = []
        for paragraph in re.split(r"\n\s*\n", rest):
            line = " ".join(paragraph.split())
            if not line or line.startswith("[Illustration") or set(line) <= {"*", " "}:
                continue
            paragraphs.append(line)
        chapters.append({"title": title, "paragraphs": paragraphs})
    return chapters


def ascii_text(text):
    text = text.replace("æ", "ae").replace("Æ", "Ae").replace("œ", "oe").replace("Œ", "Oe")
    text = text.replace("’", "'").replace("‘", "'").replace("“", '"').replace("”", '"')
    # "´" marks the stressed syllable for young readers: "An´droclus".
    text = text.replace("—", " - ").replace("_", "").replace("´", "")
    return "".join(c for c in unicodedata.normalize("NFD", text) if not unicodedata.combining(c))


def join_syllables(stories):
    """Join "Eng-land" to "England", but keep "ill-natured".

    The book splits words into syllables for young readers. A hyphen stays if
    both parts are words of the book and the joined word is not.
    """
    words = set()
    for story in stories:
        for paragraph in story["paragraphs"]:
            words.update(w.lower() for w in re.findall(r"[A-Za-z]+", re.sub(r"\w+(?:-\w+)+", " ", paragraph)))

    def fix(match):
        parts = match.group(0).split("-")
        joined = "".join(parts)
        if joined.lower() in words:
            return joined
        if all(p.lower() in words and len(p) > 2 for p in parts):
            return match.group(0)
        return joined

    for story in stories:
        story["paragraphs"] = [re.sub(r"[A-Za-z]+(?:-[A-Za-z]+)+", fix, p) for p in story["paragraphs"]]


def sentences(paragraphs):
    out = []
    for paragraph in paragraphs:
        parts = re.split(r'(?<=[.!?])["\']?\s+(?=["\']?[A-Z])', paragraph)
        out.extend(p.strip() for p in parts if p.strip())
    return out


def letters(text):
    return sum(c.isalpha() for c in text)


def chunks(story_sentences):
    """Consecutive groups of one to three sentences with 40 to 200 letters."""
    result, i = [], 0
    while i < len(story_sentences):
        group = [story_sentences[i]]
        while letters(" ".join(group)) < MIN_LETTERS and i + len(group) < len(story_sentences) and len(group) < 3:
            group.append(story_sentences[i + len(group)])
        text = " ".join(group)
        if MIN_LETTERS <= letters(text) <= MAX_LETTERS:
            result.append(text)
        i += len(group)
    return result


def score(chunk):
    """Higher is better: whole sentences, little dialogue, few names."""
    names = len(re.findall(r"(?<![.!?] )(?<!^)\b[A-Z][a-z]+", chunk))
    quotes = chunk.count('"') // 2
    return -names - 2 * max(0, quotes - 1)


def draft_passages(story, count):
    candidates = chunks(sentences(story["paragraphs"]))
    if len(candidates) < MIN_PASSAGES:
        return candidates
    words = sum(len(p.split()) for p in story["paragraphs"])
    count = count or max(MIN_PASSAGES, min(DRAFT_MAX_PASSAGES, round(words / 100)))
    count = min(count, len(candidates))
    # The first passage starts the story and the last one ends it. The others
    # are the best candidates of equal parts of the story, in order.
    chosen = [0]
    middle = candidates[1:-1]
    parts = count - 2
    for k in range(parts):
        lo, hi = k * len(middle) // parts, (k + 1) * len(middle) // parts
        if hi > lo:
            best = max(range(lo, hi), key=lambda j: score(middle[j]))
            chosen.append(best + 1)
    chosen.append(len(candidates) - 1)
    return [candidates[j] for j in sorted(set(chosen))]


def slugify(text):
    return re.sub(r"[^a-z0-9]+", "-", ascii_text(text).lower()).strip("-")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("book_id", type=int, help="Project Gutenberg book number, for example 18442")
    ap.add_argument("--story", action="append", help="title of a story to include (repeat it); default: all stories")
    ap.add_argument("--join-syllables", action="store_true",
                    help='join words that a book for young readers splits into syllables ("Eng-land"); for 18442')
    ap.add_argument("--chapters", action="store_true",
                    help='a novel: split at "CHAPTER I", "CHAPTER II", ... and open the chapters in order')
    ap.add_argument("--passages", type=int, help=f"a fixed number of passages per story ({MIN_PASSAGES} to {MAX_PASSAGES})")
    ap.add_argument("--topic", action="append", default=[], help="a topic of the book, for example history (repeat it)")
    ap.add_argument("--out", help="the draft campaign file to write")
    ap.add_argument("--list", action="store_true", help="list the stories and their length, and stop")
    ap.add_argument("--text-dir", help="also write the cleaned text of each story to this directory")
    ap.add_argument("--cache", default=str(Path.home() / ".cache" / "atria-campaigns"), help="where to keep the downloaded book")
    args = ap.parse_args()
    if args.passages and not MIN_PASSAGES <= args.passages <= MAX_PASSAGES:
        fail(f"--passages must be {MIN_PASSAGES} to {MAX_PASSAGES}")

    meta, text = load_book(args.book_id, Path(args.cache))
    body = strip_gutenberg(text)
    stories = split_chapters(body) if args.chapters else split_stories(body, contents(body))
    if args.join_syllables:
        join_syllables(stories)
    for story in stories:
        story["paragraphs"] = [ascii_text(p) for p in story["paragraphs"]]

    if args.list:
        for story in stories:
            print(f"{sum(len(p.split()) for p in story['paragraphs']):5} words  {story['title']}")
        return
    if args.story:
        by_key = {heading_key(s["title"]): s for s in stories}
        missing = [t for t in args.story if heading_key(t) not in by_key]
        if missing:
            fail(f"no story with the title: {', '.join(missing)} (use --list)")
        stories = [by_key[heading_key(t)] for t in args.story]
    if args.text_dir:
        directory = Path(args.text_dir)
        directory.mkdir(parents=True, exist_ok=True)
        for story in stories:
            (directory / f"{slugify(story['title'])}.txt").write_text("\n\n".join(story["paragraphs"]) + "\n")
    if not args.out:
        return

    authors = meta.get("authors") or [{}]
    author = authors[0].get("name", "")
    if "," in author:
        last, first = (s.strip() for s in author.split(",", 1))
        author = f"{first} {last}"
    campaign = {
        "book": {
            "slug": slugify(meta["title"]),
            "title": meta["title"],
            "author": author,
            "source_url": f"https://www.gutenberg.org/ebooks/{args.book_id}",
            "license": "Public domain in the USA",
            "topics": sorted(set(args.topic)),
            "in_order": args.chapters,
        },
        "campaigns": [
            {
                "slug": slugify(story["title"]),
                "title": story["title"],
                "reviewed": False,
                "topics": [],
                "passages": draft_passages(story, args.passages),
            }
            for story in stories
        ],
    }
    Path(args.out).write_text(json.dumps(campaign, indent=2, ensure_ascii=False) + "\n")
    print(f"wrote {len(campaign['campaigns'])} stories to {args.out}. Review them and set \"reviewed\": true.")


if __name__ == "__main__":
    main()
