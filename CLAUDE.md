# CLAUDE.md — ЕГЭ-платформа

Backend for an exam-prep platform: **web + Telegram bot + admin**, one Go API.
Stage 1 scope: **part 1 only** (auto-checkable answers), 4 subjects (rus/math/inf/soc),
1 student + 1 teacher but the data model is multi-user from day one.

The full spec is in `plan-claude-code.md`; the frontend brief in `plan-claude-design.md`.

## Architecture (the load-bearing rules)

```
Web (React) ─┐
Bot (Go)   ──┼─► Go API (ALL logic + checker) ─► Postgres / MinIO / Redis(asynq)
Admin UI   ─┘                                         worker ─► Telegram
```

- **All business logic lives in the API.** The checker especially is never
  duplicated in the bot. The bot is a thin HTTP client (`internal/bot`).
- Web and bot hit the **same** endpoints; only the UI and which tasks are
  `bot_solvable` differ (§8).
- Everything depends on `internal/domain` + the `internal/store` API. Those are
  the Phase-0 frozen seam that keeps workstreams independent.

## Package layout (= ownership boundaries)

```
cmd/{api,bot,worker,ingest,migrate}   process entrypoints
internal/domain      core types + enums + answer_schema (frozen)
internal/store       sqlc-generated code + domain-typed wrappers
internal/checker     answer-comparison engine (the heart, §7) + test suite
internal/api         HTTP handlers/routing (incl. admin for stage 1)
internal/bot         Telegram bot (thin API client) + minimal transport
internal/scheduler   asynq tasks/handlers (notifications, streak nudges)
internal/scoring     score forecast (placeholder tables — see §11 M5)
internal/media       MinIO storage + serving for task images/files
internal/ingest      content pipeline (isolated adapter, §9)
tools/fetch          Python hybrid fetcher (source → normalized JSONL)
migrations           goose SQL + embed for the migrate binary
api/openapi.yaml     frozen API contract (frontend binds to this)
deploy/              Dockerfile, docker-compose, seed data
```

## Conventions

- **Go 1.26**, module `egeism`. Router: `chi`. DB: `pgx` + `sqlc`. Queue: `asynq`.
- **Never hand-edit `internal/store/sqlc/`** — it's generated. Edit
  `internal/store/queries/*.sql` and run `make sqlc` (or `sqlc generate`).
- **Schema changes go through migrations only**, one migration per change,
  serialized through the orchestrator (two agents editing migrations = conflict).
  After a schema change: add/adjust `queries/*.sql`, `sqlc generate`, then map in
  `internal/store`.
- Handlers stay thin: parse → call store/checker → `writeJSON`. Domain logic
  belongs in domain packages, not handlers.
- Store methods return **domain types**, never sqlc rows or raw JSONB `[]byte`.
- Errors: store returns `store.ErrNotFound`; handlers map via `writeStoreErr`.
- The student-facing `TaskView` **must never include the correct answer**.
  Checking is server-side; the solution is revealed only after a wrong answer.

## The checker (`internal/checker`) — treat as critical

`answer_schema` (JSONB) describes **how** to compare, not just the right string.
Four types: `number` (decimal via `big.Rat`, abs tolerance, unicode-minus/comma
folding), `string` (trim/collapse, `ci`, `yo_fold`), `set` (unordered, dedup),
`sequence` (ordered). Full rules and the mandatory test matrix are in §7 of the
plan. **The table test in `checker_test.go` is the safety net — keep it green and
extend it before changing comparison logic.** Human verification on real ФИПИ
answers per subject is required before trusting it in production (§7 checkpoint).

## Auth (JWT sessions)

Real per-account auth, role tied to the account (no role toggle). Web users
`POST /api/auth/register` / `/api/auth/login` (username + password, bcrypt-hashed)
and get a signed JWT. Every protected call sends `Authorization: Bearer <jwt>`;
`withUser` verifies it (secret from `JWT_SECRET`) and loads the user.
`domain.User` never carries the password hash, so it can't leak. `GET /api/auth/me`
returns the current user; the web restores the session from a stored token.

**Telegram linking (bot auth).** The bot no longer auto-provisions an anonymous
student. A user (student *or* teacher) links their real web account to Telegram
via a one-time code: the web (sidebar "Привязать Telegram" button →
`POST /api/auth/telegram/link-code`) issues a code + `t.me/<TELEGRAM_BOT_USERNAME>?start=<code>`
deep link (short-lived, `telegram_link_codes` table, migration 00003). The bot
redeems it — `/start <code>` or `/link <code>` → `POST /api/auth/telegram/link`
(`{code, telegram_id}`) — which binds `telegram_id` to that account
(`RedeemTelegramLinkCode`, one tx; `users.telegram_id` UNIQUE → one Telegram per
account). Thereafter `POST /api/auth/telegram` (`{telegram_id}`) is **resolve-only**
(404 if unlinked → the bot prompts to link). The account's role decides the bot's
command set: students solve, teachers get read-only stats/что назначено/как решено.

## Web frontend (`/web`)

React + Vite + TypeScript, TanStack Query, Recharts. Design tokens (light/dark)
from the handoff live in `web/src/theme.css`; API client + hooks in
`web/src/api.ts`. All 11 screens (student + teacher) call the real API. Auth: a
login/register screen (`web/src/Login.tsx`) stores the JWT and sends it as
`Authorization: Bearer`; the role comes from the account, no toggle. `vite dev`
proxies `/api` and `/health` to `:8080`. The random-variant generator
(`POST /api/admin/tests/generate`) is the teacher's one-click test builder.

## Content ingest (§9) — hybrid FIPI + РЕШУ via a Python fetcher

Decision history: datasets were rejected (no images/files); FIPI open bank has
media but **no answers**; РЕШУ (sdamgia) has answers + номер + FIPI-origin
images. Chosen: a **hybrid**, and — per §9's "isolated adapter" rule — the
fragile, site-specific scraping lives in a small **Python fetcher**
(`tools/fetch/`), NOT in Go. It emits our **normalized JSONL** (`RawTask` per
line, with media URLs + answers) which `cmd/ingest -source dataset` consumes; the
Go side stays source-agnostic and tested. `classify_answer` infers the answer
type with a confidence (subject-aware: "245" is a number in math/информатика but
a digit code elsewhere, §7); low confidence → stays `draft` for curation. Nothing
goes live without a human approving it in the bank.

**Sources are per-subject** (`tools/fetch/server.py` dispatches on subject):
- **информатика → `openfipi.py`** — scrapes **openfipi.devinf.ru**, a community
  mirror of the ФИПИ **open bank** for информатика grouped by задание, that
  carries the real FIPI condition + images (some inlined as base64) + attached
  `.zip` + a **curated answer** per task. This is the reliable one (real FIPI +
  answer in one place); use it, not РЕШУ, for информатика. `requests` + `bs4`
  only (server-rendered; no Selenium). Filters `has_answer=y` and задание via the
  site's POST form (`type = number-1`), random order so repeat pulls grow the
  bank. Statements keep tables **legible** — a leaf table becomes `a | b | c`
  rows (the web renders `statement` with `white-space: pre-wrap`), so truth
  tables/DB headers don't collapse; icon-sized base64 junk (the download arrow,
  the "Forbidden" stub) is dropped, real FIPI images/inline-base64 are kept.
  Answers are crowdsourced → curate before going live (all `draft`).
- **rus / math / soc → `fetch.py`** — the `sdamgia` (РЕШУ) path (FIPI-origin
  tasks + answers). РЕШУ is ~seconds per request, so `fetch.py` fetches
  **concurrently** (a `ThreadPoolExecutor`, `FETCH_WORKERS`=6) and **round-robins
  one category per задание** for coverage — a full variant that used to time out
  now completes (≈20 real math tasks in ~20s). Part-2 problems (topic like
  "Д14 C4") are skipped. Still less battle-tested than openfipi; if it returns
  empty, check `docker compose logs fetcher`.

**Button-driven (primary UX):** the fetcher runs as an HTTP service
(`tools/fetch/server.py`, the `fetcher` compose service) exposing `POST /fetch`.
The bank's **"Подтянуть задания"** button hits `POST /api/admin/tasks/fetch`
(teacher), which calls the fetcher and runs the result through the same ingest
(media → MinIO, dedup, draft). **REAL sources only — there is no mock/demo
generator** (it was removed: fake tasks polluted the bank, the teacher couldn't
tell them apart, and the variant builder ingests as *active*). openfipi serves
информатика (requests+bs4, always installed); РЕШУ/sdamgia serves the rest (the
image installs the fork from git, best-effort). On failure or empty the fetcher
returns `[]` (always `X-Fetch-Mode: real`) and the API answers "источник не
вернул заданий" so the teacher retries — it NEVER substitutes made-up tasks. A
wall-time budget under `FETCH_DEADLINE` (compose sets 80s; API call timeout 90s)
keeps a long pull from being hard-killed. Both sources **round-robin across
задания** for even coverage (not a clustered random sample) and pull a random
sample so repeat pulls grow the bank instead of duping. If sdamgia returns empty
for rus/math/soc, check `docker compose logs fetcher`. A file upload
(`POST /api/admin/tasks/import`) is the manual fallback. The demo seed is behind
the `demo` compose profile, so a plain `up` leaves the bank empty (pull real via
the button).

## Media (MinIO) — `internal/media`

Task images and attached files live in MinIO. Ingest downloads each source media
URL into MinIO with a **content-addressed key** (sha1 → auto-dedup); tasks store
`domain.Media{Key,Kind,Alt}`. `http(s)` URLs are downloaded, `data:` URIs (inline
base64, e.g. openfipi's inlined FIPI images) are decoded and stored, local paths
are read and uploaded. The API serves them at public `GET /api/media/<key>`
(keys are unguessable hashes). If a download fails, the source URL is kept as the
key and `mediaUrl()` uses it directly (graceful fallback; a bad `data:` URI is
dropped rather than kept as a giant key). The web renders images
inline and files as download links (`MediaBlock`). The **bot now renders media
tasks too, as Rich Messages** (it no longer filters on `bot_solvable`): the
statement goes out as Telegram HTML (pipe tables → aligned monospace `<pre>`,
`⟦img:N⟧` inline formulas → their `alt` text — mirrors `web/src/ui.tsx`), then
block figures are sent as photos and attached files as documents. Presentation
lives in the bot (`internal/bot/format.go`, `richmedia.go`), not the API — it is
UI, not the frozen business logic. Transparent PNG figures are **flattened onto
white** (`flattenToWhite`) before sending so they stay legible on Telegram's dark
theme, exactly like the web's always-white figure container.

## Run it

```
make dev            # infra only (postgres/redis/minio) for local `go run`
make migrate        # apply migrations
make ingest         # load the seed dataset (as draft)
go run ./cmd/api    # start API on :8080
cd web && npm install && npm run dev   # web on :5173 (proxies to :8080)
make up             # or: build+run the whole stack in docker (web on :3000)
make test           # all Go tests (checker safety net)
```

## Status / what's stubbed

Done: Phase 0, checker + full test suite, student solve flow (M1 backbone),
stats + feeds endpoints, random-variant generator, bot conversation, scheduler
(asynq), dataset ingest (file/URL, JSON/JSONL), score-forecast placeholder,
docker stack incl. web, the full React frontend (all 11 screens wired), and
real JWT auth (register/login per role, bot on tokens), and the media pipeline
(MinIO upload/serve, images+files rendered in the web).
TODO: run/validate the Python fetcher against live РЕШУ/FIPI (it's a template),
Telegram deep-link handoff, PDF export, LLM-assisted answers + progressive hints
(part-2), real ФИПИ primary→test score tables.
