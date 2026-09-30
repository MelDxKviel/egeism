# Content fetcher (source → normalized JSONL)

The app is source-agnostic: `internal/ingest` reads a **normalized JSONL** (one
`RawTask` per line) and downloads media into MinIO. All the fragile,
site-specific scraping lives here in Python, so swapping/extending the source
never touches the Go app (plan §9).

## Current bank policy (September 2026)

HTTP `POST /fetch` uses `recent.py` for all four subjects. It discovers links to
dated actual **ФИПИ exam variants** on each РЕШУ `/methodist` page, verifies the
task's membership, retrieves its condition and answer, and preserves images.
The provider stays `sdamgia`: this is a third-party publication of a real exam,
not a claimed official ФИПИ API. Task numbers come from the actual variant's
positions, which can differ from РЕШУ's topic taxonomy.

`source.published_at` records the original source/exam date, with
`date_evidence_url` and `date_evidence`. `verified_at` is the separate fetch/check
time. The earliest explicit citation wins: a task reused in a 2026 variant but
also cited in 2021 stays dated 2021. A citation with only a year or no verifiable
date remains unverified. Import time, copyright year and HTTP Last-Modified are
never evidence of task age. Go enforces a rolling one-year window for active
ingest, manual activation, random variants and practice queries; old rows remain
available for review and existing assignments/history. Missing proof is reported
as `held` and keeps a task in draft. Old imports receive no invented backfill.

The live adapter was checked on 2026-09-30: math 19 candidates (12 current;
older/undated repeats held), rus 26 current, inf 27 current, soc 16 current.
These are observations, not hardcoded IDs or guaranteed future counts. If the
source changes or has no verified recent tasks, the fetch returns no candidates;
the bank never creates substitutes. Low-water background jobs deduplicate draws.

Run offline regressions: `python -m pytest tools/fetch -q`.

## Legacy adapters (manual CLI and repairs)

- **информатика → `openfipi.py`** (openfipi.devinf.ru). This is the reliable one:
  a community mirror of the ФИПИ **open bank** for информатика, grouped by
  задание, that carries the FIPI condition + images (some inlined as base64) +
  the attached-files `.zip` + a **curated answer** per task. Real ФИПИ + answer
  in one place. Its original publication dates are not exposed, so these tasks
  stay unverified. By-id repair for `subject=inf` routes here unless
  `provider=sdamgia` is passed. Deps: `requests` + `beautifulsoup4` only (server-rendered HTML, no
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
