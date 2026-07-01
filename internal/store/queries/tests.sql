-- name: CreateTest :one
INSERT INTO tests (subject_id, kind, title, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetTest :one
SELECT * FROM tests WHERE id = $1;

-- name: GetPracticeTest :one
-- Reusable ad-hoc practice test per (subject, owner) for bot/web free-solving.
SELECT * FROM tests
WHERE subject_id = $1 AND created_by = $2 AND kind = 'drill' AND title = '__practice__'
LIMIT 1;

-- name: ListTests :many
SELECT * FROM tests
WHERE (sqlc.narg('subject_id')::uuid IS NULL OR subject_id = sqlc.narg('subject_id'))
ORDER BY created_at DESC;

-- name: DeleteTest :execrows
DELETE FROM tests WHERE id = $1;

-- name: TestHasAttempts :one
-- A test that has been attempted (assigned+solved or self-practice) is in use
-- and must not be silently deleted — deleting would orphan student history.
SELECT EXISTS (SELECT 1 FROM attempts WHERE test_id = $1);

-- name: DeleteAssignmentsByTest :exec
DELETE FROM assignments WHERE test_id = $1;

-- name: AddTestItem :one
INSERT INTO test_items (test_id, task_id, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTestItems :many
SELECT ti.*, t.subject_id, t.number, t.statement, t.media, t.answer_schema, t.status
FROM test_items ti
JOIN tasks t ON t.id = ti.task_id
WHERE ti.test_id = $1
ORDER BY ti.position;
