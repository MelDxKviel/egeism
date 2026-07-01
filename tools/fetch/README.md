# Content fetcher (source → normalized JSONL)

The app is source-agnostic: `internal/ingest` reads a **normalized JSONL** (one
`RawTask` per line) and downloads media into MinIO. All the fragile,
site-specific scraping lives here in Python, so swapping/extending the source
never touches the Go app (plan §9).

## Sources (per subject)

- **информатика → `openfipi.py`** (openfipi.devinf.ru). This is the reliable one:
  a community mirror of the ФИПИ **open bank** for информатика, grouped by
  задание, that carries the FIPI condition + images (some inlined as base64) +
  the attached-files `.zip` + a **curated answer** per task. Real ФИПИ + answer
  in one place — no РЕШУ needed for информатика. `server.py` routes `subject=inf`
  here. Deps: `requests` + `beautifulsoup4` only (server-rendered HTML, no
  Selenium). Answers are crowdsourced → curate before going live (they ingest as
  `draft` like everything else).
- **rus / math / soc → `fetch.py`** (РЕШУ ЕГЭ via `sdamgia`, below).

Probe openfipi directly: `python openfipi.py <number|0> <limit>`
(e.g. `python openfipi.py 3 5` = five задание-3 tasks).

## Why here and not in Go

РЕШУ ЕГЭ (`sdamgia`) has a maintained Python parser that already returns, per
problem: `topic` (= номер задания), `condition` (text + **image URLs**),
`solution`, and the **`answer`**. Reusing it beats reimplementing brittle HTML
parsing in Go. РЕШУ's task images are FIPI-origin, so you get FIPI-derived
conditions + media + the correct answer in one place.

## Legality / etiquette

Scraping РЕШУ is against their ToS (grey zone; the plan records this without
moralizing). This is a low-volume, personal study tool — keep it gentle:
`--delay`, low `--limit`, cache, don't hammer. A pure-FIPI content mode
(open.fipi.ru `questions.php` per project GUID) is sketched in `fetch.py`
(`FIPI_PROJ`); matching FIPI↔РЕШУ for answers needs OCR and is a future step.

## Usage

```bash
pip install -r tools/fetch/requirements.txt

# fetch → JSONL (draft by default when ingested)
python tools/fetch/fetch.py --subject inf --limit 50 --min-confidence 0.7 > inf.jsonl

# ingest as draft; media downloads into MinIO; curate answers in the Bank
go run ./cmd/ingest -source dataset -provider sdamgia -path inf.jsonl
```

## Confidence & curation

`classify_answer` infers the `answer_schema` type from the raw answer and returns
a confidence. Ambiguous cases (e.g. a multi-digit code that could be a `set` or a
`sequence`) get **low confidence** — filter them with `--min-confidence`, or
ingest anyway and confirm the answer/type in the teacher's **Bank** before
approving to `active`. Nothing goes live without a human ok.

## Caveats

- `fetch.py` is a runnable **template**: validate the exact `sdamgia` API calls
  against the fork/version you install (the wrappers differ).
- Run locally (RU sites, TLS, anti-scraping) — not from CI/sandboxes.
- File-attachment tasks (некоторые инф-задания 9/17/18/24–27) reference data
  files; the wrapper may only give the condition text/images. Those are
  "web-only" (§8) and may need the file URL pulled separately.
