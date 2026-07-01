-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByTelegram :one
SELECT * FROM users WHERE telegram_id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: CreateUserWithCredentials :one
INSERT INTO users (role, name, username, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListStudents :many
SELECT * FROM users WHERE role = 'student' ORDER BY name;

-- name: CreateUser :one
INSERT INTO users (role, telegram_id, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: IdleStudents :many
-- Students who have answered before but not since $1 (for streak nudges).
SELECT u.id, u.telegram_id, u.name
FROM users u
JOIN attempts att ON att.student_id = u.id
JOIN answers  a   ON a.attempt_id = att.id
WHERE u.role = 'student'
GROUP BY u.id
HAVING max(a.answered_at) < sqlc.arg('idle_since');

-- name: ListStudentsForTeacher :many
SELECT u.* FROM users u
JOIN enrollments e ON e.student_id = u.id
WHERE e.teacher_id = $1
ORDER BY u.name;

-- name: CreateEnrollment :one
INSERT INTO enrollments (teacher_id, student_id)
VALUES ($1, $2)
ON CONFLICT (teacher_id, student_id) DO UPDATE SET teacher_id = EXCLUDED.teacher_id
RETURNING *;
