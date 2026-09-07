-- name: LockAttempt :one
SELECT * FROM attempts WHERE id = $1 FOR UPDATE;

-- name: InsertSubmission :one
INSERT INTO answers (attempt_id, task_id, raw_answer, is_correct, time_spent_ms, review_status, points, max_points)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: AddSolutionPhoto :one
INSERT INTO solution_photos (attempt_id, task_id, object_key, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetSolutionPhoto :one
SELECT * FROM solution_photos WHERE id = $1;

-- name: ListSolutionPhotos :many
SELECT * FROM solution_photos WHERE attempt_id = $1 ORDER BY created_at, id;

-- name: DeleteSolutionPhoto :exec
DELETE FROM solution_photos WHERE id = $1;

-- name: GradeAnswer :one
UPDATE answers SET points = $3, teacher_comment = $4, review_status = 'reviewed',
    reviewed_by = $5, reviewed_at = now(), is_correct = ($3 = max_points)
WHERE attempt_id = $1 AND id = $2 AND review_status IN ('pending', 'reviewed') RETURNING *;

-- name: CreateReviewNotification :exec
INSERT INTO notifications (user_id, kind, assignment_id, attempt_id)
VALUES ($1, 'attempt_reviewed', $2, $3);

-- name: RecentNumberPerformance :many
WITH recent AS (
    SELECT t.number, a.is_correct,
           row_number() OVER (PARTITION BY t.number ORDER BY a.answered_at DESC, a.id) AS rank
    FROM answers a JOIN attempts att ON att.id = a.attempt_id JOIN tasks t ON t.id = a.task_id
    WHERE att.student_id = $1 AND t.subject_id = $2 AND a.review_status <> 'pending'
)
SELECT number, count(*) AS total, count(*) FILTER (WHERE is_correct) AS correct
FROM recent WHERE rank <= 10 GROUP BY number ORDER BY number;

-- name: ClaimPracticeFetch :execrows
INSERT INTO practice_fetch_limits (subject_id) VALUES ($1)
ON CONFLICT (subject_id) DO UPDATE SET requested_at = now()
WHERE practice_fetch_limits.requested_at < now() - interval '2 minutes';
