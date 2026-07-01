-- name: HeatmapForStudent :many
-- Activity per day (github-style heatmap): total answers and how many correct.
SELECT date_trunc('day', a.answered_at)::date AS day,
       count(*)                          AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
WHERE att.student_id = $1
  AND a.answered_at >= $2
GROUP BY day
ORDER BY day;

-- name: MasteryByNumber :many
-- Success per task number for a subject (per-task mastery grid).
SELECT t.number,
       count(*)                          AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct,
       avg(a.time_spent_ms)::bigint      AS avg_time_ms
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE att.student_id = $1 AND t.subject_id = $2
GROUP BY t.number
ORDER BY t.number;

-- name: MasterySeries :many
-- Per-number success over time (weekly buckets) for the mastery line chart.
SELECT t.number,
       date_trunc('week', a.answered_at)::date AS week,
       count(*)                          AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE att.student_id = $1 AND t.subject_id = $2
GROUP BY t.number, week
ORDER BY t.number, week;

-- name: WeakSpots :many
-- Worst task numbers by success rate (>= min attempts), for "слабые места".
SELECT t.number,
       count(*)                          AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct,
       avg(a.time_spent_ms)::bigint      AS avg_time_ms
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE att.student_id = $1 AND t.subject_id = $2
GROUP BY t.number
HAVING count(*) >= sqlc.arg('min_attempts')
ORDER BY (count(*) FILTER (WHERE a.is_correct))::float / count(*) ASC
LIMIT sqlc.arg('lim');

-- name: AnswersOnDay :many
-- Heatmap drill-down: what was solved on a given day.
SELECT a.id, a.task_id, a.raw_answer, a.is_correct, a.time_spent_ms, a.answered_at,
       t.number, t.subject_id
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE att.student_id = $1
  AND a.answered_at >= sqlc.arg('day_start')
  AND a.answered_at <  sqlc.arg('day_end')
ORDER BY a.answered_at;

-- name: SubjectAccuracy :one
-- Overall success rate for a subject: input to the score forecast (§11 M5).
SELECT count(*)                          AS total,
       count(*) FILTER (WHERE a.is_correct) AS correct
FROM answers a
JOIN attempts att ON att.id = a.attempt_id
JOIN tasks t      ON t.id = a.task_id
WHERE att.student_id = $1 AND t.subject_id = $2;
