-- name: GetBankSyncJob :one
SELECT * FROM bank_sync_jobs WHERE id = $1;

-- name: RecentBankSyncJob :one
SELECT * FROM bank_sync_jobs
WHERE subject = $1 AND number = $2 AND active = $3
  AND (state IN ('queued', 'running') OR created_at >= $4)
  AND kind = $5
ORDER BY (state IN ('queued', 'running')) DESC, created_at DESC LIMIT 1;

-- name: CountPendingBankSyncJobs :one
SELECT count(*) FROM bank_sync_jobs WHERE state IN ('queued', 'running');

-- name: InsertBankSyncJob :one
INSERT INTO bank_sync_jobs(subject, number, task_limit, active, kind) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ReapBankSyncJobs :exec
UPDATE bank_sync_jobs
SET state = CASE WHEN attempts < 3 THEN 'queued' ELSE 'failed' END,
    started_at = NULL, lease_until = NULL,
    finished_at = CASE WHEN attempts >= 3 THEN now() ELSE NULL END,
    error = CASE WHEN attempts >= 3 THEN 'Обновление прервано перезапуском сервиса. Повторите позже.' ELSE '' END
WHERE state = 'running' AND lease_until < now();

-- name: PruneBankSyncJobs :exec
DELETE FROM bank_sync_jobs WHERE state NOT IN ('queued', 'running') AND finished_at < now() - interval '7 days';

-- name: ClaimBankSyncJob :one
UPDATE bank_sync_jobs SET state = 'running', started_at = now(), attempts = attempts + 1,
    lease_until = now() + interval '6 minutes'
WHERE id = (
    SELECT j.id FROM bank_sync_jobs j
    WHERE j.state = 'queued'
      AND (SELECT count(*) FROM bank_sync_jobs WHERE state = 'running') < 2
      AND NOT EXISTS (SELECT 1 FROM bank_sync_jobs r WHERE r.state = 'running' AND r.subject = j.subject)
    ORDER BY j.created_at LIMIT 1 FOR UPDATE SKIP LOCKED
) RETURNING *;

-- name: FinishBankSyncJob :exec
UPDATE bank_sync_jobs SET state = $3, result = $4, error = $5, finished_at = now(), lease_until = NULL
WHERE id = $1 AND attempts = $2 AND state = 'running';

-- name: CancelBankSyncJob :one
UPDATE bank_sync_jobs SET state = CASE WHEN state IN ('queued', 'running') THEN 'cancelled' ELSE state END,
    finished_at = CASE WHEN state IN ('queued', 'running') THEN now() ELSE finished_at END,
    lease_until = NULL
WHERE id = $1 RETURNING *;
