#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["websockets>=13", "pillow>=10"]
# ///
"""Screenshots of the main pages of Atria, for each change of the interface.

See docs/architecture/ui.md, section 12. The script:

1. builds Atria and fills the TEST database with sample data,
2. starts a development server,
3. takes screenshots of the main pages in four variants: desktop light,
   desktop dark, phone, and e-ink (portrait, grayscale),
4. writes one PNG per page and one overview image per variant.

Warning: the script deletes all data in TEST_DATABASE_URL, as the tests do.
The name of that database must contain "test", or the script stops.

Usage (from the repository root):

    uv run scripts/ui-screenshots.py                 # writes to ui-screenshots/
    uv run scripts/ui-screenshots.py --out /tmp/shots --only desktop-light

Chromium: set CHROME to its path, or install it with Playwright
(`uvx playwright install chromium`), or have chromium on PATH.
"""

import argparse
import asyncio
import base64
import glob
import http.server
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.parse
import urllib.request
from pathlib import Path

import websockets
from PIL import Image, ImageDraw, ImageOps

ROOT = Path(__file__).resolve().parent.parent
USER = "screenshots@atria.local"

VARIANTS = {
    "desktop-light": {"width": 1400, "height": 900, "scheme": "light", "mobile": False},
    "desktop-dark": {"width": 1400, "height": 900, "scheme": "dark", "mobile": False},
    "phone": {"width": 390, "height": 844, "scheme": "light", "mobile": True},
    # Portrait Android tablet (Galaxy Tab S9 FE) in the dark theme.
    "tablet-dark": {"width": 625, "height": 1000, "scheme": "dark", "mobile": True},
    # Portrait at a typical CSS size of a 7.8" e-ink reader, then grayscale.
    "eink": {"width": 760, "height": 1012, "scheme": "light", "mobile": True, "grayscale": True},
}

ARTICLE = """<html><head><title>How to read more deeply</title></head><body><article>
<h1>How to read more deeply</h1>
<p>Deep reading needs time without interruptions. Turn off notifications and choose one text for the evening.</p>
<p>Take notes in your own words. A short summary after each chapter helps you remember the main ideas.</p>
<h2>Tools</h2><p>A read-it-later list keeps articles for the right moment. An e-reader removes the temptation of other tabs.</p>
</article></body></html>"""

FEED = """<?xml version="1.0"?><rss version="2.0"><channel><title>Science daily</title>
<item><title>New telescope sees the first galaxies</title><link>{base}/article.html</link><guid>a1</guid>
<description>Astronomers published the first images from the new space telescope.</description></item>
<item><title>Bees learn to count to four</title><link>{base}/article.html</link><guid>a2</guid>
<description>A study shows that honey bees can learn small numbers and use them to find food.</description></item>
<item><title>Ocean currents change faster than expected</title><link>{base}/article.html</link><guid>a3</guid>
<description>Measurements from 40 years show a shift in the Gulf Stream.</description></item>
</channel></rss>"""

NOTE = """# Reading plan 2026

Books for the autumn:

- **Dune** by Frank Herbert
- *Saturnin* by Zdeněk Jirotka

## Notes

Read one chapter each evening. Energy is $E = mc^2$.

```go
fmt.Println("hello")
```
"""

TABLE = "Kniha;Autor;Strany;Přečteno\nDune;Frank Herbert;612;ano\nSaturnin;Zdeněk Jirotka;240;ne\nMalý princ;Antoine de Saint-Exupéry;96;ano\n"


def fail(message):
    print(f"error: {message}", file=sys.stderr)
    sys.exit(1)


def load_dotenv(path):
    env = {}
    if path.exists():
        for line in path.read_text().splitlines():
            m = re.match(r"^\s*([A-Z_][A-Z0-9_]*)=(.*)$", line)
            if m:
                env[m.group(1)] = m.group(2).strip().strip('"').strip("'")
    return env


def test_database_url():
    dsn = os.environ.get("TEST_DATABASE_URL") or load_dotenv(ROOT / ".env").get("TEST_DATABASE_URL")
    if not dsn:
        fail("TEST_DATABASE_URL is not set (in the environment or in .env)")
    name = urllib.parse.urlparse(dsn).path.lstrip("/")
    if "test" not in name.lower():
        fail(f'refusing to wipe database "{name}": its name must contain "test"')
    return dsn


def find_chrome():
    if os.environ.get("CHROME"):
        return os.environ["CHROME"]
    home = Path.home()
    for pattern in ("chromium-*/chrome-linux64/chrome", "chromium-*/chrome-linux/chrome", "chromium-*/chrome-mac/Chromium.app/Contents/MacOS/Chromium"):
        found = sorted(glob.glob(str(home / ".cache/ms-playwright" / pattern)))
        if found:
            return found[-1]
    for name in ("chromium", "chromium-browser", "google-chrome", "google-chrome-stable"):
        if shutil.which(name):
            return shutil.which(name)
    fail("Chromium not found: set CHROME, or run `uvx playwright install chromium`")


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def serve_fixtures(directory):
    """Serve the article and the feed, so that Atria can fetch them."""
    port = free_port()

    class Quiet(http.server.SimpleHTTPRequestHandler):
        def __init__(self, *a, **kw):
            super().__init__(*a, directory=directory, **kw)

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", port), Quiet)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, f"http://127.0.0.1:{port}"


CAMPAIGN = """{
  "book": {"slug": "sample-stories", "title": "Sample Stories", "author": "Atria", "topics": ["legends"]},
  "campaigns": [{"slug": "the-wise-king", "title": "The Wise King", "reviewed": true, "passages": [
    "Once upon a time a wise old king lived in a castle by the sea.",
    "Every morning the king walked along the shore and talked to the fishermen.",
    "One day a great storm came, and the king helped the people of the village."
  ]}]
}
"""


def run(binary, env, *args, stdin=None):
    result = subprocess.run([binary, *args], env=env, input=stdin, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        fail(f"atria {' '.join(args)} failed: {result.stderr.strip()[-500:]}")
    return result.stdout


def seed(binary, env, work):
    fixtures = work / "fixtures"
    fixtures.mkdir()
    server, base = serve_fixtures(str(fixtures))
    (fixtures / "article.html").write_text(ARTICLE)
    (fixtures / "feed.xml").write_text(FEED.format(base=base))
    (work / "books.csv").write_text(TABLE)
    (work / "campaign.json").write_text(CAMPAIGN)
    try:
        run(binary, env, "db", "drop", "--force")
        run(binary, env, "db", "migrate")
        run(binary, env, "user", "add", "--email", USER, "--name", "Screenshots", "--password", "x", "--role", "admin")
        run(binary, env, "note", "add", "Reading plan 2026", "--path", "/personal", stdin=NOTE)
        run(binary, env, "note", "add", "Garden ideas", "--path", "/personal/home", stdin="Tomatoes, basil and a small pond.\n")
        run(binary, env, "note", "add", "Work log", "--path", "/work", stdin="Meeting notes from Monday.\n")
        run(binary, env, "article", "add", f"{base}/article.html")
        run(binary, env, "rss", "add", "Science daily", f"{base}/feed.xml")
        run(binary, env, "rss", "fetch", "Science daily")
        run(binary, env, "tag", "add", "reading")
        run(binary, env, "tag", "attach", "Reading plan 2026", "reading")
        run(binary, env, "table", "import", str(work / "books.csv"), "--title", "Knihovna")
        run(binary, env, "ebook", "create", "Reading plan 2026", "How to read more deeply", "--title", "Weekend reading")
        run(binary, env, "games", "import", str(work / "campaign.json"))
    finally:
        server.shutdown()


def first_link(base, page, prefix):
    """The first /prefix/<uuid> link on a list page."""
    html = urllib.request.urlopen(f"{base}{page}", timeout=10).read().decode()
    m = re.search(rf'href="({re.escape(prefix)}/[0-9a-f-]{{36}})"', html)
    return m.group(1) if m else None


def pages(base):
    found = {
        "note": first_link(base, "/notes", "/notes"),
        "article": first_link(base, "/read", "/read"),
        "feed": first_link(base, "/rss/feeds", "/rss"),
        "ebook": first_link(base, "/ebooks", "/ebooks"),
        "table": first_link(base, "/tables", "/tables"),
        "campaign": first_link(base, "/games", "/games/campaigns"),
    }
    result = [
        ("01-home", "/"), ("02-notes", "/notes"), ("03-note", found["note"]),
        ("04-read", "/read"), ("05-article", found["article"]), ("06-rss", "/rss"),
        ("07-rss-feed", found["feed"]), ("08-ebooks", "/ebooks"), ("09-ebook", found["ebook"]),
        ("10-tables", "/tables"), ("11-table", found["table"]), ("12-games", "/games"),
        ("13-cryptogram", "/games/cryptograms/01900000-0000-7000-8000-000000000001"),
        ("14-search", "/search?q=reading"), ("15-settings", "/settings/rss"),
        ("16-storage", "/settings/storage"), ("17-profile", "/profile"), ("18-login", "/login"),
        ("19-campaign", found["campaign"]), ("20-games-single", "/games?tab=single"),
    ]
    return [(name, path) for name, path in result if path]


def start_chrome(chrome):
    """Start headless Chromium; return the process and the WebSocket URL of its page."""
    port = free_port()
    proc = subprocess.Popen([chrome, "--headless=new", f"--remote-debugging-port={port}", f"--user-data-dir={tempfile.mkdtemp()}",
                             "--no-first-run", "about:blank"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        try:
            targets = json.load(urllib.request.urlopen(f"http://127.0.0.1:{port}/json", timeout=1))
            return proc, next(t for t in targets if t["type"] == "page")["webSocketDebuggerUrl"]
        except (OSError, ValueError, StopIteration):
            time.sleep(0.1)
    proc.terminate()
    fail("Chromium did not start")


async def screenshot_all(ws_url, page_list, out, variants):
    async with websockets.connect(ws_url, max_size=None) as ws:
        counter = 0

        async def call(method, **params):
            nonlocal counter
            counter += 1
            await ws.send(json.dumps({"id": counter, "method": method, "params": params}))
            while True:
                msg = json.loads(await ws.recv())
                if msg.get("id") == counter:
                    return msg

        await call("Page.enable")
        for variant in variants:
            v = VARIANTS[variant]
            target = out / variant
            target.mkdir(parents=True, exist_ok=True)
            await call("Emulation.setDeviceMetricsOverride", width=v["width"], height=v["height"], deviceScaleFactor=1, mobile=v["mobile"])
            await call("Emulation.setEmulatedMedia", features=[{"name": "prefers-color-scheme", "value": v["scheme"]}])
            for name, path in page_list:
                await call("Page.navigate", url=path)
                await asyncio.sleep(2)
                shot = await call("Page.captureScreenshot", format="png")
                image = target / f"{name}.png"
                image.write_bytes(base64.b64decode(shot["result"]["data"]))
                if v.get("grayscale"):
                    ImageOps.grayscale(Image.open(image)).save(image)
            overview(target, out / f"{variant}.png")
            print(f"{variant}: {len(list(target.glob('*.png')))} pages -> {out / (variant + '.png')}")


def overview(directory, path, columns=3, width=460):
    """One image with all pages of a variant, with the page name above each."""
    images = sorted(directory.glob("*.png"))
    thumbs = []
    for image in images:
        im = Image.open(image).convert("RGB")
        im.thumbnail((width, 10_000))
        thumbs.append((image.stem, im))
    cell_h = max(t.height for _, t in thumbs) + 24
    rows = (len(thumbs) + columns - 1) // columns
    sheet = Image.new("RGB", (columns * (width + 12) + 12, rows * (cell_h + 12) + 12), "#888888")
    draw = ImageDraw.Draw(sheet)
    for i, (name, im) in enumerate(thumbs):
        x = 12 + (i % columns) * (width + 12)
        y = 12 + (i // columns) * (cell_h + 12)
        draw.text((x, y), name, fill="#000000")
        sheet.paste(im, (x, y + 18))
    sheet.save(path)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", default=str(ROOT / "ui-screenshots"), help="output directory (default: ui-screenshots/)")
    ap.add_argument("--only", choices=sorted(VARIANTS), action="append", help="only these variants (repeatable)")
    args = ap.parse_args()

    dsn = test_database_url()
    chrome = find_chrome()
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)

    with tempfile.TemporaryDirectory() as tmp:
        work = Path(tmp)
        binary = str(work / "atria")
        print("building atria…")
        subprocess.run(["go", "build", "-o", binary, "./cmd/atria"], cwd=ROOT, check=True)

        port = free_port()
        env = {**os.environ, "DATABASE_URL": dsn, "ATRIA_USER": USER, "ATRIA_ENV": "development",
               "STORAGE_PATH": str(work / "storage"), "PORT": str(port), "ATRIA_HOST": "127.0.0.1"}
        env.pop("DEBUG", None)
        print("seeding the test database…")
        seed(binary, env, work)

        server = subprocess.Popen([binary, "server"], env=env, cwd=work, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        base = f"http://127.0.0.1:{port}"
        try:
            for _ in range(100):
                try:
                    urllib.request.urlopen(f"{base}/healthz", timeout=1)
                    break
                except OSError:
                    time.sleep(0.1)
            else:
                fail("the Atria server did not start")
            page_list = [(name, base + path) for name, path in pages(base)]
            browser, ws_url = start_chrome(chrome)
            try:
                asyncio.run(screenshot_all(ws_url, page_list, out, args.only or list(VARIANTS)))
            finally:
                browser.terminate()
        finally:
            server.terminate()


if __name__ == "__main__":
    main()
