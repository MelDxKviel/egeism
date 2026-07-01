-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: CreateTask :one
INSERT INTO tasks (subject_id, number, statement, media, answer_schema, source, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListTasks :many
SELECT * FROM tasks
WHERE (sqlc.narg('subject_id')::uuid IS NULL OR subject_id = sqlc.narg('subject_id'))
  AND (sqlc.narg('number')::int      IS NULL OR number = sqlc.narg('number'))
  AND (sqlc.narg('status')::text     IS NULL OR status = sqlc.narg('status'))
ORDER BY number, created_at DESC
LIMIT $1 OFFSET $2;

-- name: RandomTasksOnePerNumber :many
-- One random active task per number for a subject: a classic random variant.
SELECT DISTINCT ON (number) id, number
FROM tasks
WHERE subject_id = $1 AND status = 'active'
ORDER BY number, random();

-- name: RandomTasksForNumber :many
-- N random active tasks of one number: a drill variant.
SELECT id FROM tasks
WHERE subject_id = $1 AND number = $2 AND status = 'active'
ORDER BY random()
LIMIT $3;

-- name: PracticeTasks :many
-- Active tasks for a subject that the student has NOT yet solved correctly
-- `mastered` times — so mastered tasks stop repeating in practice. Random order.
SELECT t.* FROM tasks t
WHERE t.subject_id = sqlc.arg('subject_id') AND t.status = 'active'
  AND (
    SELECT count(*) FROM answers a
    JOIN attempts att ON att.id = a.attempt_id
    WHERE att.student_id = sqlc.arg('student_id') AND a.task_id = t.id AND a.is_correct
  ) < sqlc.arg('mastered')::bigint
ORDER BY random()
LIMIT sqlc.arg('lim');

-- name: UpdateTaskAnswer :one
UPDATE tasks SET answer_schema = $2 WHERE id = $1 RETURNING *;

-- name: UpdateTaskContent :one
-- Refresh a task's statement + media in place (re-fetch/upgrade), leaving its
-- curated answer_schema and status untouched.
UPDATE tasks SET statement = $2, media = $3 WHERE id = $1 RETURNING *;

-- name: SetTaskStatus :one
UPDATE tasks SET status = $2 WHERE id = $1 RETURNING *;

-- name: TaskExistsBySource :one
SELECT EXISTS (
    SELECT 1 FROM tasks
    WHERE source ->> 'provider'  = sqlc.arg('provider')::text
      AND source ->> 'extern_id' = sqlc.arg('extern_id')::text
);

-- name: CountTasksBySubject :one
SELECT COUNT(*) FROM tasks WHERE subject_id = $1;

-- name: DeleteTestItemsForUnansweredTasksBySubject :exec
-- Detach the about-to-be-cleared bank tasks from any tests first: test_items
-- has no ON DELETE CASCADE to tasks, so it must go before the task delete.
-- Only tasks with no recorded answers are touched (answered ones are kept).
DELETE FROM test_items
WHERE task_id IN (
    SELECT id FROM tasks
    WHERE subject_id = $1
      AND NOT EXISTS (SELECT 1 FROM answers a WHERE a.task_id = tasks.id)
);

-- name: DeleteUnansweredTasksBySubject :execrows
-- Clear the bank for a subject, preserving any task that carries student
-- history (has a recorded answer) so attempts/stats never orphan.
DELETE FROM tasks
WHERE subject_id = $1
  AND NOT EXISTS (SELECT 1 FROM answers a WHERE a.task_id = tasks.id);
