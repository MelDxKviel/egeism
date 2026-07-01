-- name: CreateAssignment :one
INSERT INTO assignments (test_id, student_id, assigned_by, scheduled_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAssignment :one
SELECT * FROM assignments WHERE id = $1;

-- name: ListAssignmentsForStudent :many
SELECT * FROM assignments
WHERE student_id = $1
ORDER BY scheduled_at DESC;

-- name: ListAssignmentsWithTestForStudent :many
-- Dashboard "Назначено тебе": assignment joined with its test + task count.
SELECT a.id, a.test_id, a.scheduled_at, a.notified_at, a.status,
       t.title, t.kind, t.subject_id,
       (SELECT count(*) FROM test_items ti WHERE ti.test_id = t.id) AS task_count
FROM assignments a
JOIN tests t ON t.id = a.test_id
WHERE a.student_id = $1
ORDER BY a.scheduled_at DESC;

-- name: MarkAssignmentNotified :one
UPDATE assignments SET notified_at = now() WHERE id = $1 RETURNING *;

-- name: SetAssignmentStatus :one
UPDATE assignments SET status = $2 WHERE id = $1 RETURNING *;

-- name: ListDueAssignments :many
-- Scheduler safety net: assignments past their time never notified.
SELECT * FROM assignments
WHERE notified_at IS NULL AND scheduled_at <= $1
ORDER BY scheduled_at;
