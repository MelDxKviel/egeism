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
-- An optional number narrows the pool to one задание (the server-side drill).
SELECT t.* FROM tasks t
WHERE t.subject_id = sqlc.arg('subject_id') AND t.status = 'active'
  AND (sqlc.narg('number')::int IS NULL OR t.number = sqlc.narg('number'))
  AND (
    SELECT count(*) FROM answers a
    JOIN attempts att ON att.id = a.attempt_id
    WHERE att.student_id = sqlc.arg('student_id') AND a.task_id = t.id AND a.is_correct
  ) < sqlc.arg('mastered')::bigint
ORDER BY random()
LIMIT sqlc.arg('lim');

-- name: MistakeTasks :many
-- «Работа над ошибками»: active tasks whose LATEST answer by this student is
-- wrong — answering one correctly (anywhere) drops it out of the queue.
-- Oldest mistakes first, so nothing rots at the bottom.
SELECT t.* FROM tasks t
JOIN LATERAL (
    SELECT a.is_correct, a.answered_at
    FROM answers a
    JOIN attempts att ON att.id = a.attempt_id
    WHERE att.student_id = sqlc.arg('student_id') AND a.task_id = t.id
    ORDER BY a.answered_at DESC
    LIMIT 1
) last ON NOT last.is_correct
WHERE t.subject_id = sqlc.arg('subject_id') AND t.status = 'active'
ORDER BY last.answered_at
LIMIT sqlc.arg('lim');

-- name: CountMistakeTasks :one
-- Size of the «работа над ошибками» queue (the dashboard badge).
SELECT count(*) FROM tasks t
JOIN LATERAL (
    SELECT a.is_correct
    FROM answers a
    JOIN attempts att ON att.id = a.attempt_id
    WHERE att.student_id = sqlc.arg('student_id') AND a.task_id = t.id
    ORDER BY a.answered_at DESC
    LIMIT 1
) last ON NOT last.is_correct
WHERE t.subject_id = sqlc.arg('subject_id') AND t.status = 'active';

-- name: PracticeNumbers :many
-- The student's training map: per задание-номер, how many active tasks the bank
-- holds, how many of those the student has mastered (solved correctly >=
-- `mastered` times), and their lifetime answer accuracy on the number. Numbers
-- whose tasks are all inactive still show as long as the rows exist, so history
-- never disappears from the map.
SELECT t.number,
       COUNT(*) FILTER (WHERE t.status = 'active')::bigint AS bank_active,
       COUNT(*) FILTER (WHERE t.status = 'active' AND st.correct_cnt >= sqlc.arg('mastered')::bigint)::bigint AS mastered,
       COALESCE(SUM(st.total_cnt), 0)::bigint   AS answers_total,
       COALESCE(SUM(st.correct_cnt), 0)::bigint AS answers_correct
FROM tasks t
LEFT JOIN LATERAL (
    SELECT count(*) AS total_cnt, count(*) FILTER (WHERE a.is_correct) AS correct_cnt
    FROM answers a
    JOIN attempts att ON att.id = a.attempt_id
    WHERE a.task_id = t.id AND att.student_id = sqlc.arg('student_id')
) st ON TRUE
WHERE t.subject_id = sqlc.arg('subject_id')
GROUP BY t.number
ORDER BY t.number;

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

-- name: ActivateDraftTaskBySource :execrows
-- Promote a dedup-hit DRAFT to active (the builder path ingests as active, so a
-- re-fetched task it needs must become usable). Drafts only — a task the
-- teacher rejected stays rejected.
UPDATE tasks SET status = 'active'
WHERE source ->> 'provider'  = sqlc.arg('provider')::text
  AND source ->> 'extern_id' = sqlc.arg('extern_id')::text
  AND status = 'draft';

-- name: CountTasksBySubject :one
SELECT COUNT(*) FROM tasks WHERE subject_id = $1;

-- name: TaskCountsByNumber :many
-- Per-номер bank availability for a subject: how many tasks total and how many
-- are ACTIVE (usable in a generated variant). Powers the composed-variant
-- builder's availability hints.
SELECT number,
       COUNT(*) FILTER (WHERE status = 'active')::bigint AS active,
       COUNT(*)::bigint AS total
FROM tasks
WHERE subject_id = $1
GROUP BY number
ORDER BY number;

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
