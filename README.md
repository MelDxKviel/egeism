# egeism — ЕГЭ-платформа (backend)

Платформа подготовки к ЕГЭ: **сайт + Telegram-бот + админка** на едином Go-API.
Первый этап — только часть 1 (ответы проверяются автоматически) по четырём
предметам: русский, математика, информатика, обществознание.

Полная спецификация — [`plan-claude-code.md`](plan-claude-code.md).
Архитектура и конвенции — [`CLAUDE.md`](CLAUDE.md). Контракт API —
[`api/openapi.yaml`](api/openapi.yaml).

## Стек

Backend: Go 1.26 · chi · PostgreSQL + sqlc + pgx · goose · asynq (Redis) · MinIO.
Frontend: React + Vite + TypeScript · TanStack Query · Recharts. Деплой: Docker.

## Быстрый старт (Docker)

```bash
cp deploy/.env.example deploy/.env      # задай JWT_SECRET (и TELEGRAM_TOKEN при желании)
docker compose -f deploy/docker-compose.yml up --build -d
# postgres, redis, minio, миграции, bucket, api (:8080), worker, web (:3000).
# compose сам подхватывает deploy/.env. Бот — с профилем bot и TELEGRAM_TOKEN.
curl localhost:8080/health   # {"status":"ok"}
# открой http://localhost:3000
```

## Быстрый старт (локально)

```bash
make dev                 # инфраструктура в docker (postgres/redis/minio)
make migrate             # миграции
make ingest              # загрузить демо-датасет заданий (как draft)
go run ./cmd/api         # API на :8080
cd web && npm install && npm run dev   # веб на :5173 (проксирует на :8080)
make test                # все тесты, включая safety-net чекера
```

Задания в банк попадают как `draft` — учитель одобряет их в разделе «Банк»
(курация авто-ингеста), после чего они идут в тесты и практику.

## Ингест контента (§9) — гибрид FIPI + РЕШУ

FIPI отдаёт условия и медиа, но **без ответов**; у РЕШУ (sdamgia) есть ответы +
номера + картинки (родом с FIPI). Поэтому скрейпинг вынесен в отдельный
**Python-фетчер** ([tools/fetch](tools/fetch/README.md), переиспользует либу
`sdamgia`), который выдаёт нормализованный JSONL. Go-часть остаётся
источник-агностичной: `cmd/ingest` читает этот JSONL и **скачивает медиа в
MinIO**.

```bash
# 1. собрать задания из источника → JSONL (локально, RU-сайты)
pip install -r tools/fetch/requirements.txt
python tools/fetch/fetch.py --subject inf --limit 50 --min-confidence 0.7 > inf.jsonl

# 2. загрузить как draft; картинки/файлы уедут в MinIO; ответы курирует учитель
go run ./cmd/ingest -source dataset -provider sdamgia -path inf.jsonl
```

Медиа отдаётся вебу через `GET /api/media/<key>`. Любой источник конвертируется в
`RawTask` — меняется только фетчер/пакет `ingest`. Демо-сид (без медиа) —
`deploy/seed/tasks.dataset.jsonl`.

**Кнопкой из интерфейса (основной способ).** В разделе **«Банк» → «Подтянуть
задания»** учитель выбирает предмет/количество и жмёт кнопку — сервер тянет
задания из источника (сервис `fetcher`: openfipi для информатики, РЕШУ/sdamgia
для остальных) и кладёт в банк (тот же ингест: медиа в MinIO, дедуп, `draft` на
курацию). Эндпоинт — `POST /api/admin/tasks/fetch`. Только **реальные** задания —
демо/заглушек нет: если источник недоступен, кнопка честно вернёт «источник не
вернул заданий», а не подсунет фейки. Запасной путь — загрузка файла
(`POST /api/admin/tasks/import`, «или загрузить файлом»).

## Демо-поток (M1 — хребет системы)

```bash
# 1. Регистрация ученика → session token
TOKEN=$(curl -s localhost:8080/api/auth/register \
  -d '{"role":"student","username":"anya","password":"secret1","name":"Аня"}' | jq -r .token)
AUTH="Authorization: Bearer $TOKEN"

# 2. Задания одобряются учителем в «Банке» (draft -> active); для демо — прямо в БД.

# 3. Старт практики по математике
ATTEMPT=$(curl -s localhost:8080/api/practice -H "$AUTH" \
  -d '{"subject":"math"}' | jq -r .attempt_id)

# 4. Берём задачу и отправляем ответ — checker проверит на сервере
TASK=$(curl -s "localhost:8080/api/tasks?subject=math&status=active" \
  -H "$AUTH" | jq -r '.[0].id')
curl -s localhost:8080/api/attempts/$ATTEMPT/answers -H "$AUTH" \
  -d "{\"task_id\":\"$TASK\",\"raw_answer\":\"3,5\"}"     # {"is_correct":true,...}
```

## Бинарники

| `cmd/` | Назначение |
|---|---|
| `api` | HTTP API — вся бизнес-логика |
| `bot` | Telegram-бот (тонкий клиент API) |
| `worker` | asynq-воркер: уведомления, нуджи, sweep назначений |
| `ingest` | загрузка контента из датасета (файл/URL, JSON/JSONL) |
| `migrate` | применение goose-миграций (встроены в бинарь) |

## Состояние

Реализовано: Phase 0, **checker с полным тест-сьютом (§7)**, поток решения
ученика (M1), статистика (heatmap / mastery / mastery-series / weak-spots /
forecast / drill-down), ленты назначений и попыток, **генератор случайного
варианта в один клик**, бот, шедулер (asynq), ингест датасета (файл/URL,
JSON/JSONL), прогноз балла (заглушка таблиц), docker-стек с вебом, и **весь
React-фронтенд** (11 экранов, провязан на API, светлая/тёмная тема).

Авторизация: **JWT-сессии**, отдельные аккаунты ученика и учителя (роль
привязана к аккаунту, без тумблера), бот — на токенах.

TODO: Telegram deep-link хендофф (привязка чата к веб-сессии), заливка медиа в
MinIO, PDF-экспорт бланка, LLM-подсказки и разбор части 2, официальные таблицы
перевода баллов ФИПИ.
