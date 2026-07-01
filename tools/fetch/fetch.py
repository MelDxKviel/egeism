#!/usr/bin/env python3
"""Hybrid content fetcher: РЕШУ ЕГЭ (answers + номер + images) → normalized JSONL.

Design (see plan §9): the fragile, site-specific scraping lives HERE, in Python,
NOT in the Go app. This script emits one normalized RawTask per line (the shape
`cmd/ingest -source dataset` reads), and the Go ingest downloads each media URL
into MinIO. Swap or extend this script without touching the app.

Hybrid note: РЕШУ's task images are FIPI-origin, so this already gives you
FIPI-derived conditions + media + the correct answer in one place.

Inline formulas (the load-bearing detail for МАТЕМАТИКА). РЕШУ renders every
formula and special symbol as an `<img class="tex">` inside the statement (e.g.
"цена равна p = 500 руб."). The `sdamgia` library's `get_problem_by_id` throws
those away from the condition *text* (leaving holes: "цена равна  руб.") and
dumps ALL of them into a flat image list — so the app used to show each formula
as a big detached block, out of place. Instead we parse the condition HTML
ourselves and keep formulas WHERE THEY BELONG: a formula `<img>` becomes an
inline media entry (``"inline": true``) plus a ``⟦img:N⟧`` placeholder at its
exact spot in the statement; the web swaps the placeholder for the small,
baseline-aligned image. Only genuine figures (a graph, a geometry drawing —
non-``tex`` images) stay as block media rendered under the statement.

We still use the `sdamgia` library ONLY to discover problem ids (catalog +
category listings); the per-problem page is fetched and parsed here (requests +
bs4, one GET per problem, same as before) so the number/answer match the library
byte-for-byte while the statement keeps its formulas in place.

IMPORTANT: this is a runnable template. Validate against the version you install
(forks differ), and run it locally (RU sites, anti-scrape — be gentle: low rate,
cache). Usage:

    pip install -r tools/fetch/requirements.txt
    python tools/fetch/fetch.py --subject math --limit 50 > math.jsonl
    go run ./cmd/ingest -source dataset -provider sdamgia -path math.jsonl  # draft
"""
import argparse
import json
import os
import random
import re
import sys
from concurrent.futures import ThreadPoolExecutor

import requests
from bs4 import (BeautifulSoup, CData, Comment, Declaration, Doctype,
                 NavigableString, ProcessingInstruction, Tag)

# РЕШУ subject codes per our subject code. NOTE: math = ПРОФИЛЬНАЯ (math-ege);
# базовая была бы "mathb". Verify against your installed lib/site.
SDAMGIA_SUBJECT = {"rus": "rus", "math": "math", "inf": "inf", "soc": "soc"}

# sdamgia is one engine behind per-subject subdomains: https://{code}-ege.sdamgia.ru
SDAMGIA_DOMAIN = "sdamgia.ru"

UA = "Mozilla/5.0 (egeism content fetcher; personal study tool)"

# open.fipi.ru open-bank project GUIDs (for a future FIPI-first content mode).
FIPI_PROJ = {"math": "AC437B34557F88EA4115D2F374B0A07B"}  # extend as needed


def _base_url(subj: str) -> str:
    return f"https://{subj}-ege.{SDAMGIA_DOMAIN}"


def classify_answer(answer: str, subject: str = "math"):
    """Infer an AnswerSchema + a confidence in [0,1] from a raw answer string.

    Subject-aware because a bare multi-digit answer is ambiguous (§7): in math
    "245" is the number 245, but in rus/inf/soc it is usually the digit *code*
    2-4-5 (a sequence). Ambiguity yields low confidence — the task then stays
    draft for the teacher to confirm the type in the bank.
    """
    a = (answer or "").strip()
    if not a:
        return None, 0.0
    short_code = re.fullmatch(r"\d{2,4}", a) is not None
    # number (checked before set so a decimal comma like "3,5" isn't split).
    if re.fullmatch(r"[-−]?\d+(?:[.,]\d+)?", a):
        if short_code and subject not in ("math", "inf"):
            # a bare digit code, not a number → sequence (order matters); low conf.
            # (math AND информатика: a bare integer is almost always the numeric
            # answer; the digit-code case is a rus/soc "установите соответствие".)
            return {"type": "sequence", "correct": [a], "token": "char"}, 0.5
        return {"type": "number", "correct": [a], "tolerance": 0}, 0.9
    # multiple separated numbers -> set ("выберите верные", любой порядок)
    if re.fullmatch(r"\d+(?:[\s,;]+\d+)+", a):
        return {"type": "set", "correct": [a], "token": "split"}, 0.6
    # longer pure-digit string: a code — sequence by default (ambiguous)
    if re.fullmatch(r"\d{2,}", a):
        return {"type": "sequence", "correct": [a], "token": "char"}, 0.5
    # otherwise a word/phrase
    return {"type": "string", "correct": [a], "ci": True, "yo_fold": True}, 0.8


# --- condition HTML → inline-aware statement + media -----------------------

# NavigableString subclasses that are NOT real text (comments, PIs, …).
_NONTEXT = (Comment, ProcessingInstruction, Declaration, Doctype, CData)
_BLOCK = {"p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote"}


def _abs_src(src: str, base: str) -> str:
    """Resolve a (possibly relative) media src to an absolute URL.

    Formula images live on ege.sdamgia.ru (already absolute); attached figures
    are site-relative ("/get_file?id=…"), so prepend the subject base.
    """
    src = (src or "").strip()
    if not src:
        return ""
    if src.startswith(("http://", "https://", "data:")):
        return src
    return base + src if src.startswith("/") else base + "/" + src


def _is_formula(img) -> bool:
    """A rendered LaTeX chunk. РЕШУ marks these with class 'tex'; everything
    else (graphs, geometry drawings, tables-as-image) is a genuine figure."""
    return "tex" in (img.get("class") or [])


def _visible(node) -> bool:
    """False if an ancestor is display:none. РЕШУ hides its collapsible
    «Что проверяется…» theory/справка cards that way (shown in a popup on click),
    so a hidden pbody is NOT part of the condition."""
    for p in node.parents:
        if "display:none" in (p.get("style") or "").replace(" ", "").lower():
            return False
    return True


def _condition_pbodies(block):
    """The pbody blocks that make up the actual task condition.

    One РЕШУ prob_maindiv carries much more than the condition: a collapsible
    «Что проверяется и что нужно знать?» theory card (hidden via display:none),
    the condition itself, and a «Пояснение/Решение». The load-bearing tell is
    that РЕШУ wraps the CONDITION — the statement plus its «самостоятельно
    подберите…» instruction — in a `div.nobreak` keep-together block, while the
    solution is a bare pbody OUTSIDE nobreak and the theory cards are display:none
    (verified across rus/math/soc). So: visible pbodies under a nobreak wrapper.

    The old code took `pbodies[0]` blindly, which for русский (задания with big
    справки) grabbed the theory card and dumped the whole methodology — codifier
    notes, downloadable files, a worked example, the ALGORITHM — into the
    statement instead of the question."""
    cond = [pb for pb in block.find_all("div", {"class": "pbody"})
            if _visible(pb) and any("nobreak" in (p.get("class") or []) for p in pb.parents)]
    if cond:
        return cond
    # РЕШУ markup changed: degrade to the first VISIBLE pbody (still skips the
    # hidden theory card) rather than emit an empty statement.
    visible = [pb for pb in block.find_all("div", {"class": "pbody"}) if _visible(pb)]
    return visible[:1]


def _serialize(node, base, media, out):
    """Walk the condition DOM in reading order, appending text to `out`.

    A formula <img> is replaced INLINE by a ``⟦img:N⟧`` placeholder (and recorded
    as inline media, N = its index in `media`); a figure <img> is recorded as
    block media with NO placeholder (the web renders it under the statement, as
    before). Block-level tags emit a trailing newline so paragraphs/list items
    stay on their own line for the web's pre-wrap rendering.
    """
    if isinstance(node, NavigableString):
        if not isinstance(node, _NONTEXT):
            out.append(str(node))
        return
    if not isinstance(node, Tag):
        return
    name = node.name
    if name in ("script", "style"):
        return
    if name == "br":
        out.append("\n")
        return
    if name == "img":
        src = _abs_src(node.get("src"), base)
        if not src:
            return
        alt = (node.get("alt") or "").strip()
        inline = _is_formula(node)
        media.append({"url": src, "kind": "image", "alt": alt, "inline": inline})
        if inline:
            out.append(f"⟦img:{len(media) - 1}⟧")
        return
    for c in node.children:
        _serialize(c, base, media, out)
    if name in _BLOCK:
        out.append("\n")


def _clean(s: str) -> str:
    """Trim lines and drop blanks. Strips the soft hyphens (U+00AD) and NBSPs
    РЕШУ sprinkles through justified text, which otherwise litter the statement."""
    s = (s or "").replace("­", "").replace("\xa0", " ").replace("​", "")
    lines = [re.sub(r"[ \t]+", " ", ln).strip() for ln in s.splitlines()]
    return "\n".join(ln for ln in lines if ln)


def _parse_problem(content: bytes, base: str):
    """Parse a РЕШУ /problem page → (topic, statement, media, answer) or None.

    Mirrors the sdamgia library's field extraction (so number+answer are
    identical) but builds the statement with inline formula placeholders.
    """
    soup = BeautifulSoup(content, "html.parser")
    block = soup.find("div", {"class": "prob_maindiv"})
    if block is None:
        return None
    # "Тип 12 № 26718" → topic "12" (drop the leading "Тип" word and "№ id"),
    # exactly as the library derives it, so номер задания is unchanged.
    nums = block.find("span", {"class": "prob_nums"})
    topic = " ".join(nums.get_text().split()[1:][:-2]) if nums else ""
    cond = _condition_pbodies(block)
    if not cond:
        return None
    media, out = [], []
    for pb in cond:  # statement + instruction, minus theory cards / solution
        _serialize(pb, base, media, out)
    statement = _clean("".join(out))
    ans = block.find("div", {"class": "answer"})
    answer = ans.get_text().replace("Ответ: ", "").strip() if ans else ""
    return topic, statement, media, answer


def to_raw_task(subject_code: str, pid: str, url: str,
                topic: str, statement: str, media: list, answer: str,
                require_answer: bool = True):
    """Assemble a normalized RawTask (or None if unusable: no/undecidable answer
    or a non-numeric «тип», i.e. a part-2 problem — skip it like the old int()).

    With require_answer=False (the re-fetch/upgrade path) the task is returned
    even when the answer can't be classified: the caller only wants the refreshed
    statement + inline media and keeps the task's existing (curated) answer.
    """
    schema, conf = classify_answer(answer, subject_code)
    if schema is None and require_answer:
        return None
    try:
        number = int(topic or 0)
    except ValueError:
        return None  # part-2 «тип» like "C4"/"Д14" isn't a plain задание number
    rt = {
        "subject": subject_code,
        "number": number,
        "statement": statement,
        "media": media,
        "answer_schema": schema,
        "source": {
            "provider": "sdamgia",
            "extern_id": str(pid),
            "url": url,
        },
        # Not persisted by the app; a hint for you when reviewing the JSONL.
        "_confidence": round(conf, 2),
    }
    if schema is None:
        del rt["answer_schema"]  # upgrade path: leave the existing answer untouched
    return rt


def _fetch_one(subject_code: str, base: str, session, pid, require_answer: bool = True):
    """Fetch and parse one РЕШУ problem page → RawTask (or None). Shared by the
    bulk `fetch` (discovery) and `fetch_by_ids` (targeted re-fetch/upgrade)."""
    url = f"{base}/problem?id={pid}"
    try:
        r = session.get(url, timeout=25)
        r.raise_for_status()
        parsed = _parse_problem(r.content, base)
        if not parsed:
            return None
        topic, statement, media, answer = parsed
        return to_raw_task(subject_code, pid, url, topic, statement, media, answer,
                           require_answer=require_answer)
    except Exception as e:  # noqa: BLE001 — keep going on a bad problem
        print(f"skip {pid}: {e}", file=sys.stderr)
        return None


def fetch(subject_code: str, limit: int, delay: float):
    """Yield RawTask dicts for a subject.

    Ids come from the sdamgia library (catalog + category listings); each
    problem PAGE is fetched and parsed here so formulas stay inline.
    """
    try:
        from sdamgia import SdamGIA  # validate against your installed version
    except ImportError as e:
        # Raise (not sys.exit) so the server can catch it and return [].
        raise RuntimeError("sdamgia lib not installed: pip install -r tools/fetch/requirements.txt") from e

    subj = SDAMGIA_SUBJECT[subject_code]
    base = _base_url(subj)
    api = SdamGIA()
    session = requests.Session()
    session.headers["User-Agent"] = UA
    # РЕШУ is ~seconds per request, so EVERYTHING here is fetched concurrently — a
    # small pool keeps it gentle while cutting wall-time ~Nx (sequential blows
    # past the fetch deadline on anything but a tiny pull).
    workers = max(1, int(os.getenv("FETCH_WORKERS", "6")))

    # One representative category per topic (= задание), fetched in parallel, so a
    # pull COVERS the whole exam structure (round-robin) instead of clustering,
    # and the id-gathering itself doesn't eat the deadline.
    cat_ids = [t["categories"][0]["category_id"]
               for t in api.get_catalog(subj) if t.get("categories")]

    def _cat(cid):
        try:
            return api.get_category_by_id(subj, cid)
        except Exception as e:  # noqa: BLE001
            print(f"skip cat {cid}: {e}", file=sys.stderr)
            return []

    with ThreadPoolExecutor(max_workers=workers) as ex:
        buckets = list(ex.map(_cat, cat_ids))
    for b in buckets:
        random.shuffle(b)  # repeat pulls stay fresh
    ids, depth = [], max((len(b) for b in buckets), default=0)
    for i in range(depth):  # round-robin: one per задание, then seconds, …
        for b in buckets:
            if i < len(b):
                ids.append(b[i])
    ids = list(dict.fromkeys(ids))[: max(limit * 2, limit + 12)]  # over-fetch for failures

    with ThreadPoolExecutor(max_workers=workers) as ex:
        for rt in ex.map(lambda pid: _fetch_one(subject_code, base, session, pid), ids):
            if rt and rt["statement"]:
                yield rt


def fetch_by_ids(subject_code: str, ids):
    """Re-fetch specific РЕШУ problems by id → RawTask dicts (statement + inline
    media refreshed), used to upgrade tasks ingested before inline-formula
    support. The answer is left to the caller's existing curated value."""
    subj = SDAMGIA_SUBJECT[subject_code]
    base = _base_url(subj)
    session = requests.Session()
    session.headers["User-Agent"] = UA
    workers = max(1, int(os.getenv("FETCH_WORKERS", "6")))
    ids = [str(i) for i in ids if str(i).strip()]
    with ThreadPoolExecutor(max_workers=workers) as ex:
        for rt in ex.map(
            lambda pid: _fetch_one(subject_code, base, session, pid, require_answer=False), ids):
            if rt and rt["statement"]:
                yield rt


def main():
    ap = argparse.ArgumentParser(description="Fetch ЕГЭ tasks → normalized JSONL")
    ap.add_argument("--subject", required=True, choices=list(SDAMGIA_SUBJECT))
    ap.add_argument("--limit", type=int, default=50)
    ap.add_argument("--delay", type=float, default=0.5, help="seconds between requests")
    ap.add_argument("--min-confidence", type=float, default=0.0,
                    help="drop tasks below this classification confidence")
    args = ap.parse_args()

    for rt in fetch(args.subject, args.limit, args.delay):
        if rt["_confidence"] < args.min_confidence:
            continue
        print(json.dumps(rt, ensure_ascii=False))


if __name__ == "__main__":
    main()
