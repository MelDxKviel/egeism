# Production deploy

The whole stack runs in Docker behind **Caddy**, which gets and renews a
**Let's Encrypt** TLS cert automatically. Only Caddy is exposed to the internet
(80/443); Postgres, Redis, MinIO, the API, worker, bot and fetcher live only on
the internal Docker network.

## What you need

- **A VPS** (any provider) running Linux with Docker + Docker Compose v2.
  Sizing for stage 1 (1 student + 1 teacher, the whole stack incl. Postgres,
  Redis, MinIO, headless-free Python fetcher): **2 vCPU / 4 GB RAM / ~40 GB SSD**
  is comfortable. 2 GB RAM works but Go image builds are tight — either build
  with 4 GB then downscale, or build images elsewhere. Pick a region close to
  your users.
- **A domain** (or subdomain) you control, with a DNS **A record → the VPS
  public IP**. Set this BEFORE the first deploy — Caddy needs the name to resolve
  to the box to pass the ACME HTTP challenge.
- **Two Telegram bot tokens** from @BotFather: one for production (this server),
  a separate one for local dev — a single token can't long-poll from two places.
- Firewall: allow inbound **22, 80, 443** only.

## First deploy

```sh
# on the server
git clone <repo> egeism && cd egeism
cp deploy/.env.prod.example deploy/.env
# edit deploy/.env: DOMAIN, ACME_EMAIL, JWT_SECRET (openssl rand -hex 32),
#                   MINIO_ACCESS_KEY/SECRET_KEY, TELEGRAM_TOKEN, TELEGRAM_BOT_USERNAME
make prod-config     # sanity-check compose + env interpolation
make prod-up         # build + start everything; Caddy issues the cert on first hit
make prod-ps         # all services up? (migrate/minio-init exit 0 — that's expected)
```

Open `https://<DOMAIN>` — the SPA loads over HTTPS. Register the teacher +
student accounts, link the bot (sidebar → «Привязать Telegram»). The bank starts
empty; pull real tasks with the «Подтянуть задания» button.

> First cert not issued? `make prod-logs` and look at the `caddy` lines. The
> usual cause is DNS not yet pointing at the box, or port 80 blocked. To avoid
> Let's Encrypt rate limits while debugging, uncomment the staging `acme_ca`
> line in `deploy/Caddyfile`, `make prod-up`, confirm it works, then remove it
> and `make prod-up` again for a real cert.

## Updating (redeploy)

```sh
git pull
make prod-up         # rebuilds changed images, recreates only what changed
```

Migrations run automatically (the one-shot `migrate` service before the API).

## Media in the bot's rich messages

Works out of the box: figures are fetched by Telegram from
`https://<DOMAIN>/api/media/<key>` (public, unguessable content hashes), served
through Caddy → web → API → MinIO. To skip the API hop at scale, serve media
straight from MinIO: uncomment the `handle_path /media/*` block in
`deploy/Caddyfile` and set `MEDIA_PUBLIC_URL=https://<DOMAIN>/media` in
`deploy/.env`.

## Backups

The data lives in Docker volumes `pgdata` (Postgres) and `miniodata` (task
media). At minimum, a nightly `pg_dump`:

```sh
docker compose -f deploy/docker-compose.prod.yml exec -T postgres \
  pg_dump -U egeism egeism | gzip > backup-$(date +%F).sql.gz
```

Keep the `caddy_data` volume too — it holds the TLS certs and ACME account.
