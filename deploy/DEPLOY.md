# Production deploy (k3s)

Production `https://egeism.ru` runs in the existing **k3s** cluster, namespace
and Helm release `egeism`, behind Envoy Gateway. The chart originally lived on
`codex/k3s-bootstrap`; it is now included in `main` with the application code.

Publishing to `main` runs CI (including PostgreSQL integration tests and Helm
validation), then builds six GHCR images tagged with the tested commit SHA.
Deploy connects with the existing `SSH_HOST`, `SSH_USER`, `SSH_KEY`, `SSH_PORT`
secrets and runs `deploy/k3s-deploy.sh` on the node. It uses the local k3s
kubeconfig; credentials never leave the node.

The script backs up the cluster database to `/var/backups/egeism`, downloads
a checksum-pinned temporary Helm binary and upgrades with `--reuse-values`.
This preserves production TLS, storage and secret references. Pre-upgrade
jobs initialize private photo storage and apply migrations **before** new pods
start. Failed rollouts restore the previous Helm release; additive database
migrations are not reversed automatically.

The job verifies all five application Deployment image tags and checks the
public `/health`, `/version.json` and `/api/config` endpoints. The latter two
must report the exact deployed SHA. It then stops the obsolete Compose stack
without deleting its volumes. This prevents duplicate Telegram/worker processes.

Inspect a deployment with the GitHub Actions `Deploy` run, or on the node:

```sh
sudo k3s kubectl -n egeism get deployments,pods,jobs
curl -fsS https://egeism.ru/version.json
curl -fsS https://egeism.ru/api/config
```

The following Compose instructions are retained for local use and the old
installation. **They are not the production publishing path.**

# Legacy Docker Compose installation

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
If you don't have `make` on the server, the raw command is the same:
`docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env up --build -d`.

## Legacy Compose setup

- **CI** (`.github/workflows/ci.yml`) runs on every push/PR to `main`: Go
  build/vet/test (including migrations and written-review integration tests on
  PostgreSQL 16), the Python fetcher tests, and web tests/build/typecheck.
- **CD** now updates k3s as described above. Running `make prod-up` only updates
  the legacy Compose stack and does not publish to the current site.

To enable CD, add these repo **Secrets** (Settings → Secrets and variables →
Actions):

| Secret | Value |
|---|---|
| `SSH_HOST` | server IP (e.g. `193.247.81.179`) |
| `SSH_USER` | ssh user (e.g. `root`) |
| `SSH_KEY`  | the **private** key whose public half is in the server's `~/.ssh/authorized_keys` |
| `SSH_PORT` | optional, defaults to `22` |

Generate a dedicated CI key with `ssh-keygen -t ed25519 -f deploy_key`, put
`deploy_key.pub` on the server, and paste `deploy_key` (private) into `SSH_KEY`.
The deploy fetches from `origin`, so a **public** repo works out of the box; for
a private repo add a read deploy key on the server too.

## Media in the bot's rich messages

Works out of the box: figures are fetched by Telegram from
`https://<DOMAIN>/api/media/<key>` (public, unguessable content hashes), served
through Caddy → web → API → MinIO. To skip the API hop at scale, serve media
straight from MinIO: uncomment the `handle_path /media/*` block in
`deploy/Caddyfile` and set `MEDIA_PUBLIC_URL=https://<DOMAIN>/media` in
`deploy/.env`.

## Backups

Written-solution photos use the separate private bucket `egeism-media-solutions`
inside the existing MinIO volume. The API creates it on startup and removes any
anonymous policy; its MinIO credentials must allow bucket creation and policy
management. Never expose this bucket through the public task-media route.
Migration `00011` preserves historical grades and enables teacher grading for
new math part-2 submissions. Nginx permits the 10 MiB photo plus multipart overhead.

The data lives in Docker volumes `pgdata` (Postgres) and `miniodata` (task
media). At minimum, a nightly `pg_dump`:

```sh
docker compose -f deploy/docker-compose.prod.yml exec -T postgres \
  pg_dump -U egeism egeism | gzip > backup-$(date +%F).sql.gz
```

Keep the `caddy_data` volume too — it holds the TLS certs and ACME account.
