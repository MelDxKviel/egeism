#!/usr/bin/env python3
"""HTTP fetcher service. The Go API calls POST /fetch {subject, limit} and gets
a JSON array of normalized RawTask, which it then ingests (media → MinIO, dedup).

REAL SOURCES ONLY — there is deliberately no demo/mock generator. Fake tasks
would pollute the bank (the teacher can't tell them from real, and the variant
builder ingests as *active*). Sources per subject:
- информатика → openfipi.py  (ФИПИ open bank mirror, real conditions + answers)
- rus/math/soc → fetch.py     (РЕШУ ЕГЭ via sdamgia, FIPI-origin)

On any failure/empty the fetcher returns [] so the API says "источник не вернул
заданий" and the teacher retries — it NEVER substitutes made-up tasks. Stable
extern_ids mean repeated fetches dedup instead of duplicating.
"""
import json
import os
import socket
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import fetch as F
import openfipi as OF

# Bound network waits so an unreachable/slow source fails fast instead of hanging
# the button. socket timeout helps for connects; the hard thread deadline below
# bounds the whole fetch regardless of the lib.
socket.setdefaulttimeout(float(os.getenv("FETCH_TIMEOUT", "12")))
FETCH_DEADLINE = float(os.getenv("FETCH_DEADLINE", "40"))

SUBJECTS = ("rus", "math", "inf", "soc")


def _with_deadline(fn, seconds):
    """Run fn in a daemon thread; raise if it doesn't finish in `seconds`."""
    box = {}

    def run():
        try:
            box["ok"] = fn()
        except Exception as e:  # noqa: BLE001
            box["err"] = e

    th = threading.Thread(target=run, daemon=True)
    th.start()
    th.join(seconds)
    if th.is_alive():
        raise TimeoutError(f"источник не ответил за {int(seconds)}с")
    if "err" in box:
        raise box["err"]
    return box.get("ok", [])


def real_tasks(subject: str, limit: int, min_conf: float, number: int = 0, ids=None):
    delay = float(os.getenv("FETCH_DELAY", "0.5"))
    if ids:
        # Targeted re-fetch (upgrade path): pull exactly these ids so a task
        # ingested before a parser fix gets its statement + media re-parsed by
        # the CURRENT parser. РЕШУ ids for rus/math/soc; openfipi task ids
        # (/task/<id>) for информатика — e.g. healing the mangled
        # colspan/rowspan distance-matrix tables of задание 1.
        if subject == "inf":
            return list(OF.fetch_by_ids(ids, delay))
        return list(F.fetch_by_ids(subject, ids))
    if subject == "inf":
        # информатика: pull REAL ФИПИ open-bank tasks WITH answers from openfipi
        # (it filters by задание server-side, only lists tasks that have an
        # answer, and spreads an all-numbers pull evenly across 1..27). РЕШУ/
        # sdamgia is the source for the other subjects. Give it a wall-time
        # budget just under the hard deadline so it returns gracefully.
        gen = OF.fetch(number, limit, delay, budget=max(10.0, FETCH_DEADLINE - 5))
    else:
        # fetch a wider pool when filtering by number so we still get enough.
        gen = F.fetch(subject, limit * 4 if number else limit, delay)
    out = []
    for rt in gen:
        if rt.get("_confidence", 1.0) < min_conf:
            continue
        if number and rt.get("number") != number:
            continue
        out.append(rt)
        if len(out) >= limit:
            break
    return out


class Handler(BaseHTTPRequestHandler):
    def _send(self, code, obj, headers=None):
        b = json.dumps(obj, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(b)))
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(b)

    def do_GET(self):
        if self.path == "/health":
            self._send(200, {"status": "ok"})
        else:
            self._send(404, {"error": "not found"})

    def do_POST(self):
        if self.path.split("?")[0] != "/fetch":
            self._send(404, {"error": "not found"})
            return
        n = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(n) or b"{}")
        except json.JSONDecodeError:
            self._send(400, {"error": "bad json"})
            return
        subject = body.get("subject")
        limit = int(body.get("limit") or 30)
        number = int(body.get("number") or 0)  # drill: restrict to this task number
        min_conf = float(body.get("min_confidence") or 0.0)
        ids = body.get("ids") or None  # targeted re-fetch (upgrade) by РЕШУ id
        if subject not in SUBJECTS:
            self._send(400, {"error": "unknown subject"})
            return
        # Real sources only. On failure/empty return [] (the API then reports
        # "источник не вернул заданий") — NEVER substitute made-up tasks.
        try:
            tasks = _with_deadline(lambda: real_tasks(subject, limit, min_conf, number, ids), FETCH_DEADLINE)
        except Exception as e:  # noqa: BLE001
            print(f"real fetch FAILED for {subject}: {e}", flush=True)
            tasks = []
        self._send(200, tasks, {"X-Fetch-Mode": "real"})

    def log_message(self, *args):  # quiet
        pass


if __name__ == "__main__":
    port = int(os.getenv("PORT", "8090"))
    print(f"fetcher listening on :{port} (real sources only)", flush=True)
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
