-- name: StartAttempt :one
INSERT INTO attempts (assignment_id, test_id, student_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAttempt :one
SELECT * FROM attempts WHERE id = $1;

-- name: FinishAttempt :one
UPDATE attempts SET finished_at = now()
WHERE id = $1 AND finished_at IS NULL
RETURNING *;

-- name: RecordAnswer :one
INSERT INTO answers (attempt_id, task_id, raw_answer, is_correct, time_spent_ms)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListAnswersForAttempt :many
SELECT * FROM answers WHERE attempt_id = $1 ORDER BY answered_at;

-- name: ListAttemptsForStudent :many
-- Attempts feed ("Недавние решения" / "Свежие попытки") with per-attempt score.
SELECT att.id, att.test_id, att.started_at, att.finished_at,
       t.title, t.kind, t.subject_id,
       count(ans.id)                              AS total,
       count(ans.id) FILTER (WHERE ans.is_correct) AS correct,
       coalesce(sum(ans.time_spent_ms), 0)::bigint AS time_ms
FROM attempts att
JOIN tests t       ON t.id = att.test_id
LEFT JOIN answers ans ON ans.attempt_id = att.id
WHERE att.student_id = $1
GROUP BY att.id, t.title, t.kind, t.subject_id
ORDER BY att.started_at DESC
LIMIT $2;
