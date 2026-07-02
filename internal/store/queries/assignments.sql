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
-- Dashboard "Назначено тебе" + the assigned-tests history: assignment joined with
-- its test, task count, and the result of the latest finished attempt for that
-- assignment (attempt id + correct/total), so the student can see whether, what,
-- and how each assigned test was solved. Result columns are NULL when the
-- assignment has no finished attempt yet (not solved).
-- sqlc doesn't infer NULL-ability for the LEFT-JOINed derived table's columns
-- (it only pointer-izes attempt_finished_at, which is nullable in the schema),
-- so COALESCE the id/counts to keep the scan safe for not-yet-solved rows;
-- attempt_finished_at stays the reliable "was it solved" signal (NULL = not).
SELECT a.id, a.test_id, a.scheduled_at, a.notified_at, a.status,
       t.title, t.kind, t.subject_id,
       (SELECT count(*) FROM test_items ti WHERE ti.test_id = t.id) AS task_count,
       COALESCE(res.attempt_id, '00000000-0000-0000-0000-000000000000'::uuid) AS attempt_id,
       res.attempt_finished_at,
       COALESCE(res.total, 0)   AS total,
       COALESCE(res.correct, 0) AS correct
FROM assignments a
JOIN tests t ON t.id = a.test_id
LEFT JOIN (
    SELECT DISTINCT ON (att.assignment_id)
           att.assignment_id,
           att.id          AS attempt_id,
           att.finished_at AS attempt_finished_at,
           (SELECT count(*) FROM answers ans WHERE ans.attempt_id = att.id)                    AS total,
           (SELECT count(*) FROM answers ans WHERE ans.attempt_id = att.id AND ans.is_correct) AS correct
    FROM attempts att
    WHERE att.assignment_id IS NOT NULL AND att.finished_at IS NOT NULL
    ORDER BY att.assignment_id, att.finished_at DESC
) res ON res.assignment_id = a.id
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
