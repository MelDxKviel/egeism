-- +goose Up
CREATE TABLE bank_sync_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject text NOT NULL CHECK (subject IN ('rus', 'math', 'inf', 'soc')),
    number integer NOT NULL DEFAULT 0 CHECK (number BETWEEN 0 AND 99),
    task_limit integer NOT NULL CHECK (task_limit BETWEEN 1 AND 200),
    active boolean NOT NULL DEFAULT true,
    kind text NOT NULL DEFAULT 'fetch' CHECK (kind IN ('fetch', 'repair')),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    result jsonb,
    error text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX bank_sync_one_pending ON bank_sync_jobs(subject, number, active, kind)
    WHERE state IN ('queued', 'running');
CREATE INDEX bank_sync_queue ON bank_sync_jobs(created_at) WHERE state = 'queued';
CREATE INDEX bank_sync_recent ON bank_sync_jobs(subject, number, active, kind, created_at DESC);

-- +goose Down
DROP TABLE bank_sync_jobs;
