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
-- Per-student variant clones (variant_of set) are working copies, and tests a
-- student generated for themselves (пробники, __practice__) are private — the
-- teacher's builder/assign library holds neither.
SELECT * FROM tests
WHERE (sqlc.narg('subject_id')::uuid IS NULL OR subject_id = sqlc.narg('subject_id'))
  AND variant_of IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM users u WHERE u.id = tests.created_by AND u.role = 'student'
  )
ORDER BY created_at DESC;

-- name: SelfVariantsForStudent :many
-- The student's own generated пробники for a subject, newest first, each with
-- its task count and — once solved — the latest finished attempt's score.
SELECT te.id, te.subject_id, te.kind, te.title, te.created_at,
       (SELECT count(*) FROM test_items ti WHERE ti.test_id = te.id)::bigint AS task_count,
       -- LEFT JOIN makes att.id nullable but sqlc types it by the base column;
       -- fold NULL to the zero uuid and let the store map that back to nil.
       COALESCE(att.id, '00000000-0000-0000-0000-000000000000'::uuid) AS attempt_id,
       att.finished_at,
       COALESCE(ans.total, 0)::bigint   AS total,
       COALESCE(ans.correct, 0)::bigint AS correct
FROM tests te
LEFT JOIN LATERAL (
    SELECT a.id, a.finished_at FROM attempts a
    WHERE a.test_id = te.id AND a.student_id = sqlc.arg('student_id') AND a.finished_at IS NOT NULL
    ORDER BY a.finished_at DESC
    LIMIT 1
) att ON TRUE
LEFT JOIN LATERAL (
    SELECT count(*) AS total, count(*) FILTER (WHERE an.is_correct) AS correct
    FROM answers an WHERE an.attempt_id = att.id
) ans ON TRUE
WHERE te.created_by = sqlc.arg('student_id')
  AND te.subject_id = sqlc.arg('subject_id')
  AND te.title <> '__practice__'
  -- a husk left by a failed empty-bank cleanup has no items — never show it
  AND EXISTS (SELECT 1 FROM test_items ti WHERE ti.test_id = te.id)
ORDER BY te.created_at DESC
LIMIT sqlc.arg('lim');

-- name: CountSelfClassicTests :one
-- How many пробники the student has generated for a subject already — numbers
-- the next default title.
SELECT count(*) FROM tests
WHERE created_by = $1 AND subject_id = $2 AND kind = 'classic';

-- name: CountUnsolvedSelfVariants :one
-- Пробники the student generated but never finished — the generation cap
-- counts THESE, so solving one (as the cap message instructs) always unlocks
-- another. Empty husks (no items) are unsolvable and don't count.
SELECT count(*) FROM tests te
WHERE te.created_by = $1 AND te.subject_id = $2 AND te.kind = 'classic'
  AND EXISTS (SELECT 1 FROM test_items ti WHERE ti.test_id = te.id)
  AND NOT EXISTS (
    SELECT 1 FROM attempts a
    WHERE a.test_id = te.id AND a.student_id = $1 AND a.finished_at IS NOT NULL
  );

-- name: DeleteTest :execrows
DELETE FROM tests WHERE id = $1;

-- name: UpdateTestTitle :one
UPDATE tests SET title = $2 WHERE id = $1 RETURNING *;

-- name: TestHasAttempts :one
-- A test that has been attempted (assigned+solved or self-practice) is in use
-- and must not be silently deleted — deleting would orphan student history.
SELECT EXISTS (SELECT 1 FROM attempts WHERE test_id = $1);

-- name: DeleteAssignmentsByTest :exec
DELETE FROM assignments WHERE test_id = $1;

-- name: SetTestVariantOf :exec
UPDATE tests SET variant_of = $2 WHERE id = $1;

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
