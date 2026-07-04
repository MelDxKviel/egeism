# CLAUDE.md — ЕГЭ-платформа

Backend for an exam-prep platform: **web + Telegram bot + admin**, one Go API.
Stage 1 scope: **part 1 only** (auto-checkable answers), 4 subjects (rus/math/inf/soc).
Multi-user: **admin + teachers (per-subject or сверхучитель) + many students in
teacher-owned classes** (a student may also be classless — the репетитор case).

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
internal/pdf         printable variant export (embedded DejaVu for Cyrillic)
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

## Auth, roles & classes (JWT sessions)

Real per-account auth, role tied to the account: **student / teacher / admin**.
**Self-registration is GONE** (no `/api/auth/register`; `GET /api/config` returns
`allow_registration:false` for old clients): accounts are created only by an
**admin** (`POST /api/admin/users`, any role) or by a **teacher**
(`POST /api/students` — the student is enrolled to them, optionally straight
into a class). The **bootstrap admin** is created on API startup when no active
admin exists (`ADMIN_USERNAME`/`ADMIN_PASSWORD`; empty password → generated and
printed to the log once — dev compose pins admin/admin). Login via
`POST /api/auth/login`; every protected call sends `Authorization: Bearer <jwt>`;
`withUser` verifies it (secret from `JWT_SECRET`), loads the user and **cuts off
deactivated accounts** (`users.is_active`, toggled in the admin panel — takes
effect on the next request, web and bot alike). `domain.User` never carries the
password hash. `GET /api/auth/me` returns the current user; `GET /api/profile`
the identity payload (student: classes + teacher names; teacher: subject scope,
classes, roster size).

**Roles.** *Admin* manages accounts (create/edit/activate/delete — delete is
refused with 409 while history exists; self-guards stop an admin from demoting/
deactivating themselves) and watches `GET /api/admin/stats` (platform counters +
per-subject activity). *Teachers* carry an optional `users.subject`: set = they
work ONLY that subject (bank, tests, generator, assignments are all checked via
`subjectInScope`/`testInScope`/`taskInScope` in `internal/api/admin.go`; the web
locks the subject tabs); NULL = **сверхучитель**, any subject (file import is
super-teacher-only — a file can mix subjects). *Students* solve; they see only
their own stats.

**Classes & the roster.** `classes` (teacher-owned) + `class_members` (m2m,
migration 00005). Adding a member also creates the `enrollments` row (one tx,
`store.AddClassMember`) — enrollment is THE teacher↔student link every
per-student authorization runs on (`resolveStudent`, `attemptReadable`,
assignment targeting); removing from a class keeps it, so the student stays "мой
ученик без класса". `GET /api/students` returns the teacher's enrolled students
tagged with class names (`?scope=all` = the platform-wide picker; admins always
see all). `POST /api/admin/assignments` takes exactly one target — `student_id`
(enrolled check) or `class_id` (**fan-out**: one assignment + bell notification
+ Telegram schedule per member). `GET /api/classes/{id}/overview?subject=` is
the teacher's **color grid** (per-member per-number accuracy, empty members
included) the web renders red→green so lagging students/numbers pop.

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
command set: students solve, teachers get read-only stats/что назначено/как
решено (multi-student: /students list + /student N picker), admins get a
pointer to the web panel (the bot won't let an admin pollute solve stats).

**Bot UX (inline keyboards).** Messages are styled Telegram-HTML with inline
keyboards; `Reply.Buttons` rides through the transport as `reply_markup`, and
`callback_query` updates are dispatched to `Bot.HandleCallback` (data grammar:
`solve:<code>`, `next`, `tests`, `assign:<id>`, `finish`, teacher `t:<cmd>`).
Students solve **assigned tests in the chat**: the worker's assignment
notification carries «▶️ Решать тут» (callback `assign:<id>`; the worker and bot
share one bot identity, so worker-sent buttons arrive at the bot's long-poll)
and «🌐 Решать на сайте» (URL from `WEB_URL`, omitted when unset). The test flow
serves exactly the variant's tasks (`GET /api/tests/{id}/tasks` +
`POST /api/attempts {assignment_id}`), shows «3/15» progress, and `/finish` (or
the last «Итоги» button) closes the attempt — the assignment flips to done
server-side. Notifier degrades keyboard → callback-only → plain so a bad button
URL never loses the notification.

## Web frontend (`/web`)

React + Vite + TypeScript, TanStack Query, Recharts. Design tokens (light/dark)
from the handoff live in `web/src/theme.css`; API client + hooks in
`web/src/api.ts`. All screens (student + teacher + admin: users CRUD in `web/src/admin.tsx`,
classes/roster/color grid + per-student stats in `web/src/teacher.tsx`, profile
in `web/src/profile.tsx`) call the real API. Auth: a login-only screen
(`web/src/Login.tsx` — no signup) stores the JWT and sends it as
`Authorization: Bearer`; the role comes from the account, no toggle. `vite dev`
proxies `/api` and `/health` to `:8080`. The random-variant generator
(`POST /api/admin/tests/generate`) is the teacher's one-click test builder.

**Dialogs:** the ONE portaled `Modal` lives in `web/src/ui.tsx` — it re-wraps its
overlay in `.app` + `data-theme` because the design tokens are scoped there and a
bare `createPortal` to `<body>` loses them (transparent panel, system font — the
Telegram-link-modal bug). Every dialog must go through it.

**Assignments actually get solved:** the dashboard card «Начать» starts the
assigned variant itself — `GET /api/tests/{id}/tasks` (student-safe views, no
answers) + `POST /api/attempts` with `assignment_id` (validated: the assignment
must be the student's and match the test). Finishing the attempt flips the
assignment `scheduled → done` (shown as «решён ✓»). The assign form's «Уведомить
в Telegram» toggle is real: `notify=false` pre-stamps `notified_at` so neither
the queue nor the worker sweep ever messages the student.

**In-app notifications (the header bell):** a `notifications` table (migration
00004) records assignment events — `assignment_created` for the student when
the teacher assigns (always, independent of the Telegram toggle), and
`assignment_done` for the assigning teacher on the **first** scheduled→done
finish only (re-finishing another attempt stays silent — `completeAssignment`
in `internal/api/solve.go`). Rows reference only the assignment; titles/names
join at read time, and `ON DELETE CASCADE` follows test deletion.
`GET /api/notifications` returns `{unread, items}` (exact badge count + the
enriched feed); `POST …/{id}/read` / `…/read-all` clear it (user-scoped:
someone else's id → 404). The bell (`NotificationsBell`, `web/src/shell.tsx`)
polls every 30s, badges the unread count, lists the feed in the shared Modal
and toasts genuinely-new arrivals; a click marks read and jumps to the test —
the student straight into solving the assigned variant (a done one shows «уже
решён ✓»), the teacher into the test view. The query is keyed by user id so a
logout→login on one browser never flashes another user's feed.

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
  tables/DB headers don't collapse; colspan/rowspan expand to a rectangle; the
  decorative «Номер пункта» banner row + label column of distance matrices are
  **collapsed to a compact corner grid** (they cost half a phone screen for zero
  information — the renderers mark both axes as headers instead); icon-sized
  base64 junk (the download arrow, the "Forbidden" stub) is dropped, real FIPI
  images/inline-base64 are kept. Answers are crowdsourced → curate before going
  live (all `draft`).
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

**Statement repair (`POST /api/admin/tasks/refetch-formulas`, the bank's
«Обновить условия у старых заданий» button)** heals tasks ingested before a
parser fix, in place — answers/status/test placements survive. РЕШУ tasks are
re-fetched when they look stale (theory-card bloat, formulas as detached
blocks); **openfipi tasks are ALL re-parsed by the current parser** (by-id
`/task/<id>` re-fetch) and rewritten only when the statement actually changed —
statement diff, no per-bug heuristic, so e.g. the mangled colspan/rowspan
distance matrices (задание 1) healed automatically. Repeated clicks converge to
«updated: 0». Both sources' by-id paths live in `server.py` (`ids` mode).

## Media (MinIO) — `internal/media`

Task images and attached files live in MinIO. Ingest downloads each source media
URL into MinIO with a **content-addressed key** (sha1 → auto-dedup); tasks store
`domain.Media{Key,Kind,Alt}`. `http(s)` URLs are downloaded, `data:` URIs (inline
base64, e.g. openfipi's inlined FIPI images) are decoded and stored, local paths
are read and uploaded. The API serves them at public `GET /api/media/<key>`
(keys are unguessable hashes). If a download fails, the source URL is kept as the
key and `mediaUrl()` uses it directly (graceful fallback; a bad `data:` URI is
dropped rather than kept as a giant key). The web renders images
inline and files as download links (`MediaBlock`).

**Bot task rendering — REAL Rich Messages (Bot API 10.1, June 2026).** Tasks go
out rich-first via `sendRichMessage` (`internal/bot/richhtml.go` builds the
extended HTML): pipe tables become **real `<table>` grids** (header rows before
the `---` separator as `<th>`), the header is an `<h3>`, paragraphs are `<p>`,
`⟦img:N⟧` inline formulas substitute their `alt` text. **Inline `<img>` accepts
PUBLIC http(s) URLs only** (verified live: a localhost URL fails the whole call
with `RICH_MESSAGE_PHOTO_URL_INVALID`; file_id/attach:///data:/Telegram's own
file URLs are all rejected, and no document rich block exists — attached .zip
files can never inline). Figures inline when a public media base is configured:
`MEDIA_PUBLIC_URL` (e.g. the anonymous-download MinIO bucket exposed directly —
`http://<host>:9000/egeism-media` — a smaller surface than the whole web app),
falling back to `WEB_URL/api/media`, or when the media key is an absolute public
URL; otherwise figures follow as captioned photos/albums. Delivery degrades:
`sendRichMessage` → caption bubble (statement as an HTML caption above the
photo/album, ≤1024 chars) → classic text + album (`format.go`'s `<pre>` tables).
The bot no longer filters on `bot_solvable`. Presentation lives in the bot
(`richhtml.go`, `format.go`, `richmedia.go`), not the API — it is UI, not the
frozen business logic. Transparent PNG figures are **flattened onto white**
(`flattenToWhite`) before sending so they stay legible on Telegram's dark theme,
exactly like the web's always-white figure container.

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

**Production** (on the server) is a separate, hardened stack — see
[deploy/DEPLOY.md](deploy/DEPLOY.md). `deploy/docker-compose.prod.yml` is
standalone (NOT layered on the dev compose): only **Caddy** is exposed (80/443,
automatic Let's Encrypt TLS via `deploy/Caddyfile`), everything else is
internal-network-only — the dev compose publishes DB/Redis/MinIO/API host ports,
which is unsafe on a public box (Docker bypasses host firewalls). Keep the two
files' service topology in sync. `make prod-up` builds + starts (also the
redeploy command; migrations run as a one-shot before the API). Secrets +
`DOMAIN`/`ACME_EMAIL` come from `deploy/.env` (template: `deploy/.env.prod.example`);
`WEB_URL` is derived as `https://$DOMAIN`, so the bot's site button and inline
rich-message media work with zero code change. Use a **separate bot token** from
local dev (one token can't long-poll from two places).

## Status / what's stubbed

Done: Phase 0, checker + full test suite, student solve flow (M1 backbone),
the assigned-test flow (assign → solve the exact variant → done + notify toggle),
stats + feeds endpoints, random-variant generator, bot conversation, scheduler
(asynq), dataset ingest (file/URL, JSON/JSONL), score-forecast placeholder,
docker stack incl. web, the full React frontend (all 11 screens wired), and
real JWT auth (register/login per role, bot on tokens), and the media pipeline
(MinIO upload/serve, images+files rendered in the web), and in-app web
notifications (the bell: assigned → student, solved → teacher, click-through
to the test), the multi-user overhaul (admin panel: users CRUD + platform
stats; teacher classes with the color grid; per-subject teacher scoping;
profiles; no self-registration), and PDF export of composed variants
(`GET /api/admin/tests/{id}/export.pdf`, `?answers=1` appends the key page;
`internal/pdf` renders statements, pipe-table grids and MinIO images with
embedded DejaVu; web buttons «PDF для ученика» / «PDF с ответами» on the test
detail page — never hand the answers file to a student).
TODO: run/validate the Python fetcher against live РЕШУ/FIPI (it's a template),
Telegram deep-link handoff, LLM-assisted answers + progressive hints
(part-2), real ФИПИ primary→test score tables.
