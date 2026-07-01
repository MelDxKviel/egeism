#!/usr/bin/env python3
"""openfipi.devinf.ru scraper: REAL ФИПИ информатика tasks *with answers*.

Why this exists (see plan §9). РЕШУ/sdamgia (`fetch.py`) is our source for
rus/math/soc, but for информатика it is unreliable. openfipi.devinf.ru is a
community mirror of the ФИПИ **open bank** for информатика, grouped by задание
number, that ALSO carries, per task: the FIPI-origin condition + images, the
attached-files `.zip`, and a (crowdsourced, curated) **correct answer**. That is
real ФИПИ conditions + media + answer in one place — exactly what the bank needs.

информатика ONLY (the site is inf-only). Output is the SAME normalized RawTask
dict `fetch.py` emits, so `cmd/ingest` is unchanged. Deps: requests + bs4 (no
Selenium — pages are server-rendered). Be gentle (low rate, --delay). Everything
ingests as `draft` and is curated in the bank; answers are crowdsourced, so a
human confirming them is doubly warranted.
"""
import base64
import re
import sys
import time
from urllib.parse import urljoin

import requests
from bs4 import (BeautifulSoup, CData, Comment, Declaration, Doctype,
                 NavigableString, ProcessingInstruction, Tag)

from fetch import classify_answer

# NavigableString subclasses that are NOT real text — HTML comments (e.g. the
# `<!--?import namespace=... MathPlayer ... declareNamespace-->` MathML directive
# Word injects), processing instructions, doctypes — must never leak into text.
_NONTEXT = (Comment, ProcessingInstruction, Declaration, Doctype, CData)

BASE = "https://openfipi.devinf.ru"
UA = "Mozilla/5.0 (egeism content fetcher; personal study tool)"

# On the listing form the задание number is a 0-indexed <select>: value "0" =
# задание 1 … value "26" = задание 27. So type = number - 1 (or -1 for "Все").
EGE_MAX_NUMBER = 27


def _session():
    s = requests.Session()
    s.headers["User-Agent"] = UA
    return s


def _utf8(r):
    """openfipi serves UTF-8 but omits the charset header, so requests would
    guess (and mangle Cyrillic). Force UTF-8 and return the response."""
    r.encoding = "utf-8"
    return r


def _csrf(session):
    """GET the listing once to obtain a CSRF token (Flask-WTF) + session cookie,
    both required to POST the filter form. The token is reused across searches."""
    r = _utf8(session.get(f"{BASE}/tasks_ege/", timeout=25))
    r.raise_for_status()
    m = re.search(r'name="csrf_token"[^>]*value="([^"]+)"', r.text)
    if not m:
        raise RuntimeError("openfipi: no csrf_token on /tasks_ege/ (layout changed?)")
    return m.group(1)


def _search(session, token, number, want, per_page):
    """POST the filter form and return up to `want` (task_id, number) candidates.

    Filters: has_answer=y (only tasks that HAVE an answer), new=y (current),
    type=number-1 (a задание, or "Все"), random order (repeat pulls stay fresh).
    """
    data = {
        "csrf_token": token,
        "new": "y",
        "has_answer": "y",
        "type": str(number - 1) if number else "-1",
        "theme": "0",
        "order": "Случайный порядок",
        "per_page": str(per_page),
    }
    r = _utf8(session.post(f"{BASE}/tasks_ege/", data=data, timeout=25))
    r.raise_for_status()
    soup = BeautifulSoup(r.text, "html.parser")
    table = soup.find("table", class_="table-striped")
    out = []
    if not table:
        return out
    for tr in table.select("tr"):
        a = tr.select_one('a[href^="/task/"]')
        if not a:
            continue  # header row / embedded-statement row
        tid = a["href"].rstrip("/").rsplit("/", 1)[-1]
        tds = tr.find_all("td", recursive=False)
        num = number
        if len(tds) > 1 and tds[1].get_text(strip=True).isdigit():
            num = int(tds[1].get_text(strip=True))
        out.append((tid, num))
        if len(out) >= want:
            break
    return out


def _candidates(session, token, number, limit, deadline):
    """Return a candidate (id, number) list.

    For a specific задание: a random pool. For "all numbers" (number=0): pull a
    few from EACH задание and round-robin-interleave them, so a pull gives EVEN
    coverage across 1..27 instead of clustering on whatever's abundant (which is
    what a single random query does). Stops early if `deadline` is reached.
    """
    if number:
        return _search(session, token, number, max(limit * 2, 20), per_page=50)

    per_num = max(2, (limit + EGE_MAX_NUMBER - 1) // EGE_MAX_NUMBER + 1)
    buckets = []
    for n in range(1, EGE_MAX_NUMBER + 1):
        if time.monotonic() > deadline:
            break
        try:
            buckets.append(_search(session, token, n, per_num, per_page=5))
        except Exception as e:  # noqa: BLE001 — skip a bad задание, keep going
            print(f"openfipi search {n}: {e}", file=sys.stderr)
    out = []
    depth = max((len(b) for b in buckets), default=0)
    for i in range(depth):  # round-robin: one from each number, then seconds, …
        for b in buckets:
            if i < len(b):
                out.append(b[i])
    return out


# --- media -----------------------------------------------------------------

# Real image magic bytes → the correct mime. openfipi inlines some FIPI images
# as base64 and MISLABELS the mime (e.g. a JPEG tagged image/gif), so we detect
# the true format from the bytes rather than trusting the data: URI header.
_MAGIC = ((b"\x89PNG\r\n", "image/png"), (b"\xff\xd8\xff", "image/jpeg"),
          (b"GIF87a", "image/gif"), (b"GIF89a", "image/gif"))

# Below this decoded size a base64 image is an ICON, not content: it filters
# openfipi's inline download-arrow (a ~800-byte 49×43 PNG) and the tiny
# "<h1>Forbidden</h1>" stub. Real FIPI diagrams/schemas are several KB+.
_MIN_IMG_BYTES = 1500


def _data_uri_image(src):
    """Return a normalized `data:` URI if src is a real inline image, else None.
    Rewrites the mime from the true magic bytes and drops icon-sized junk."""
    m = re.match(r"data:[^,]*;base64,(.*)$", src, re.S)
    if not m:
        return None
    try:
        raw = base64.b64decode(m.group(1).strip() + "===")
    except Exception:  # noqa: BLE001
        return None
    mime = next((mt for magic, mt in _MAGIC if raw.startswith(magic)), None)
    if mime is None or len(raw) < _MIN_IMG_BYTES:
        return None
    return f"data:{mime};base64,{base64.b64encode(raw).decode()}"


# --- text ------------------------------------------------------------------

_BLOCK = {"p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote"}


def _node_to_text(node):
    """Serialize HTML to readable text. Unlike get_text(), this renders a leaf
    data table as a **Markdown table** (`| a | b |` with a `---` header rule), so
    truth tables (задание 2) / DB headers (задание 3) stay a real grid that the
    web draws as a styled <table> instead of an unreadable stream of digits."""
    if isinstance(node, NavigableString):
        return "" if isinstance(node, _NONTEXT) else str(node)
    if not isinstance(node, Tag):
        return ""
    name = node.name
    if name in ("script", "style", "img"):
        return ""
    if name == "br":
        return "\n"
    if name == "table" and node.find("table") is None:  # leaf table
        md = _table_to_markdown(node)
        if md is not None:
            return md
        # Not a real data grid — a layout/wrapper table the site uses for the
        # statement body / download row. Fall through and serialize it as text so
        # it wraps, instead of a one-cell table that scrolls sideways and clips.
    text = "".join(_node_to_text(c) for c in node.children)
    if name in _BLOCK:
        text += "\n"
    return text


def _intattr(tag, name):
    """A colspan/rowspan value as a positive int (missing/garbage → 1)."""
    try:
        return max(1, int(tag.get(name) or 1))
    except (TypeError, ValueError):
        return 1


def _table_to_markdown(table):
    """Render a leaf <table> as a Markdown grid, or return None if it isn't one.

    A naive `td` list per row gets two real ФИПИ tables wrong:

    * **Merged cells.** задание-1 distance matrices merge the "Номер пункта"
      banner (colspan) and diagonal corners (rowspan). Reading the raw cells of
      each row shifts every following cell left, so nothing lines up — the
      mangled grid. We expand spans into a rectangle (the merged value in its
      first cell, the cells it covers left blank) so the columns align.
    * **Layout tables.** Attachment tasks wrap the whole statement (and the
      download row) in a <table>; that is not data. We return None unless it
      looks like a real grid — at least two columns and only short cells — so the
      caller renders it as ordinary text that wraps, not a one-cell table.
    """
    grid = []
    carry = {}  # col -> [rows_left, fill] for cells spanning down from above
    for tr in table.find_all("tr"):
        cells = tr.find_all(["td", "th"], recursive=False) or tr.find_all(["td", "th"])
        row, col, ci = [], 0, 0
        while ci < len(cells) or any(c >= col for c in carry):
            if col in carry:                      # a rowspan reaching down to here
                rows_left, fill = carry[col]
                if rows_left <= 1:
                    del carry[col]
                else:
                    carry[col] = [rows_left - 1, fill]
                row.append(fill)
                col += 1
                continue
            if ci >= len(cells):                  # gap before a rightward rowspan
                row.append("")
                col += 1
                continue
            cell = cells[ci]
            ci += 1
            txt = " ".join(_node_to_text(cell).split())
            cspan, rspan = _intattr(cell, "colspan"), _intattr(cell, "rowspan")
            for k in range(cspan):                # value in the first cell, then blanks
                row.append(txt if k == 0 else "")
                if rspan > 1:
                    carry[col] = [rspan - 1, ""]  # rows this cell covers stay blank
                col += 1
        if any(c.strip() for c in row):           # skip fully-empty rows
            grid.append(row)

    if not grid:
        return None
    width = max(len(r) for r in grid)
    longest = max((len(c) for r in grid for c in r), default=0)
    # A data table has several columns of short values; a wrapper table is one
    # column, or a single cell holding a whole paragraph.
    if width < 2 or longest > 80:
        return None
    md = []
    for i, r in enumerate(grid):
        r = r + [""] * (width - len(r))           # pad ragged rows to a rectangle
        md.append("| " + " | ".join(r) + " |")
        if i == 0:  # header rule after the first row (leading/trailing pipes
            md.append("| " + " | ".join(["---"] * width) + " |")  # survive _clean)
    return "\n" + "\n".join(md) + "\n"


def _clean(s):
    """Trim each line and drop blank lines; preserves table rows / newlines."""
    s = (s or "").replace("\xa0", " ").replace("​", "")
    lines = [re.sub(r"[ \t]+", " ", ln).strip() for ln in s.splitlines()]
    return "\n".join(ln for ln in lines if ln)


def _parse_task(html, task_id):
    """Extract (number, statement, media, answer) from a /task/<id> page."""
    soup = BeautifulSoup(html, "html.parser")
    page_url = f"{BASE}/task/{task_id}"  # base for resolving relative media URLs

    h4 = soup.find("h4")  # first <h4> is the задание number
    number = int(h4.get_text(strip=True)) if h4 and h4.get_text(strip=True).isdigit() else 0

    h3 = soup.find("h3", string=lambda t: t and t.strip() == task_id)
    if h3 is None:
        return None

    # The statement is everything between the <h3> id heading and the answer
    # accordion. Tasks with attached files wrap it in a <table> (for the download
    # row); simple tasks use bare <p>s — so collect siblings, not one container.
    media = []
    text_parts = []
    for sib in h3.next_siblings:
        name = getattr(sib, "name", None)
        if name is None:
            continue  # whitespace / HTML comment between tags
        cls = " ".join(sib.get("class") or [])
        if name in ("form", "hr") or "accordion" in cls or "row" in cls:
            break  # reached the answer accordion / submit form
        for img in sib.find_all("img"):
            src = (img.get("src") or "").strip()
            alt = (img.get("alt") or "").strip()
            alt = "" if alt == "undefined" else alt
            if "ege.fipi.ru" in src:  # FIPI-hosted image → download by URL
                media.append({"url": urljoin(page_url, src), "kind": "image", "alt": alt})
            elif src.startswith("data:"):  # some FIPI images are inlined as base64
                du = _data_uri_image(src)
                if du:
                    media.append({"url": du, "kind": "image", "alt": alt})
            # else: openfipi's own download.gif / Forbidden stub → skip
        text_parts.append(_node_to_text(sib))

    # Attached-files .zip (tasks "выполняется с использованием прилагаемых файлов").
    # The href is sometimes relative (../../files/...), so resolve it to absolute.
    zip_a = soup.find("a", href=re.compile(r"/files/ege/"))
    if zip_a:
        media.append({"url": urljoin(page_url, zip_a["href"]), "kind": "file",
                      "alt": "Прилагаемые файлы"})

    statement = _clean("".join(text_parts))

    body = soup.select_one(".accordion-body")
    answer = re.sub(r"\s+", " ", body.get_text(" ")).strip() if body else ""

    return number, statement, media, answer


def _to_raw(task_id, number, statement, media, answer):
    schema, conf = classify_answer(answer, "inf")
    if schema is None or not statement:
        return None
    return {
        "subject": "inf",
        "number": number,
        "statement": statement,
        "media": media,
        "answer_schema": schema,
        "source": {
            "provider": "openfipi",
            "extern_id": task_id,  # stable → repeated pulls dedup instead of duping
            "url": f"{BASE}/task/{task_id}",
        },
        "_confidence": round(conf, 2),
    }


def fetch(number, limit, delay, budget=60.0):
    """Yield up to `limit` normalized RawTask dicts for информатика.

    `number` (1..27) restricts to one задание (drills); 0 pulls evenly across all
    numbers. `budget` bounds total wall-time so we return gracefully before the
    caller's hard deadline. Signature is a superset of fetch.fetch (extra kwarg).
    """
    if number and not (1 <= number <= EGE_MAX_NUMBER):
        number = 0  # out of range → pull across all numbers instead of erroring
    deadline = time.monotonic() + budget
    session = _session()
    token = _csrf(session)
    candidates = _candidates(session, token, number, limit, deadline)
    yielded = 0
    for tid, num in candidates:
        if yielded >= limit or time.monotonic() > deadline:
            break
        try:
            r = _utf8(session.get(f"{BASE}/task/{tid}", timeout=25))
            r.raise_for_status()
            parsed = _parse_task(r.text, tid)
        except Exception as e:  # noqa: BLE001 — keep going on a bad task
            print(f"openfipi skip {tid}: {e}", file=sys.stderr)
            continue
        if not parsed:
            continue
        n, statement, media, answer = parsed
        rt = _to_raw(tid, num or n, statement, media, answer)
        if rt:
            yield rt
            yielded += 1
        time.sleep(delay)  # be gentle with the mirror


if __name__ == "__main__":  # quick manual probe: python openfipi.py [number] [limit]
    import json

    num = int(sys.argv[1]) if len(sys.argv) > 1 else 0
    lim = int(sys.argv[2]) if len(sys.argv) > 2 else 5
    for rt in fetch(num, lim, 0.3):
        print(json.dumps(rt, ensure_ascii=False))
