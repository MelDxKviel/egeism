#!/usr/bin/env python3
"""Hybrid content fetcher: РЕШУ ЕГЭ (answers + номер + images) → normalized JSONL.

Design (see plan §9): the fragile, site-specific scraping lives HERE, in Python,
reusing the maintained `sdamgia` library — NOT in the Go app. This script emits
one normalized RawTask per line (the shape `cmd/ingest -source dataset` reads),
and the Go ingest downloads each media URL into MinIO. Swap or extend this script
without touching the app.

Hybrid note: РЕШУ's task images are FIPI-origin, so this already gives you
FIPI-derived conditions + media + the correct answer in one place. A pure
FIPI-first content mode (open.fipi.ru questions.php per proj GUID) is sketched in
FIPI_PROJ below; matching FIPI↔РЕШУ for answers needs OCR and is left as an
extension — low-confidence classifications stay `draft` for curation regardless.

IMPORTANT: this is a runnable template. Validate the sdamgia API calls against
the version you install (forks differ), and run it locally (RU sites, anti-scrape
— be gentle: low rate, cache). Usage:

    pip install -r tools/fetch/requirements.txt
    python tools/fetch/fetch.py --subject inf --limit 50 > inf.jsonl
    go run ./cmd/ingest -source dataset -provider sdamgia -path inf.jsonl   # draft
"""
import argparse
import json
import os
import random
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor

# РЕШУ subject codes per our subject code. NOTE: math = ПРОФИЛЬНАЯ (math-ege);
# базовая была бы "mathb". Verify against your installed lib/site.
SDAMGIA_SUBJECT = {"rus": "rus", "math": "math", "inf": "inf", "soc": "soc"}

# open.fipi.ru open-bank project GUIDs (for a future FIPI-first content mode).
FIPI_PROJ = {"math": "AC437B34557F88EA4115D2F374B0A07B"}  # extend as needed


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


def to_raw_task(subject_code: str, problem: dict):
    """Map a sdamgia problem dict to our RawTask (or None if unusable)."""
    answer = problem.get("answer")
    schema, conf = classify_answer(answer, subject_code)
    if schema is None:
        return None
    cond = problem.get("condition") or {}
    media = [{"url": u, "kind": "image"} for u in (cond.get("images") or [])]
    return {
        "subject": subject_code,
        "number": int(problem.get("topic") or 0),
        "statement": (cond.get("text") or "").strip(),
        "media": media,
        "answer_schema": schema,
        "source": {
            "provider": "sdamgia",
            "extern_id": str(problem.get("id")),
            "url": problem.get("url", ""),
        },
        # Not persisted by the app; a hint for you when reviewing the JSONL.
        "_confidence": round(conf, 2),
    }


def fetch(subject_code: str, limit: int, delay: float):
    """Yield RawTask dicts for a subject using the sdamgia library."""
    try:
        from sdamgia import SdamGIA  # validate against your installed version
    except ImportError as e:
        # Raise (not sys.exit) so the server can catch it and fall back to demo.
        raise RuntimeError("sdamgia lib not installed: pip install -r tools/fetch/requirements.txt") from e

    subj = SDAMGIA_SUBJECT[subject_code]
    api = SdamGIA()
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

    def _one(pid):
        try:
            return to_raw_task(subject_code, api.get_problem_by_id(subj, pid))
        except Exception as e:  # noqa: BLE001 — keep going on a bad problem
            print(f"skip {pid}: {e}", file=sys.stderr)
            return None

    with ThreadPoolExecutor(max_workers=workers) as ex:
        for rt in ex.map(_one, ids):
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
