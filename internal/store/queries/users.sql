-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByTelegram :one
SELECT * FROM users WHERE telegram_id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: CreateUserWithCredentials :one
INSERT INTO users (role, name, username, password_hash, subject)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListStudents :many
SELECT * FROM users WHERE role = 'student' ORDER BY name;

-- name: ListUsers :many
-- Admin panel: every account, admins/teachers first.
SELECT * FROM users
ORDER BY CASE role WHEN 'admin' THEN 0 WHEN 'teacher' THEN 1 ELSE 2 END, name;

-- name: SetUserActive :one
UPDATE users SET is_active = $2 WHERE id = $1 RETURNING *;

-- name: SetUserName :one
UPDATE users SET name = $2 WHERE id = $1 RETURNING *;

-- name: SetUserRoleSubject :one
UPDATE users SET role = $2, subject = $3 WHERE id = $1 RETURNING *;

-- name: SetUserSubject :one
UPDATE users SET subject = $2 WHERE id = $1 RETURNING *;

-- name: SetUserPassword :execrows
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE role = 'admin' AND is_active;

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
WHERE u.role = 'student' AND u.is_active
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

-- name: IsTeacherOfStudent :one
-- The teacher↔student link every per-student authorization runs on.
SELECT EXISTS(
    SELECT 1 FROM enrollments WHERE teacher_id = $1 AND student_id = $2
) AS enrolled;

-- name: ListTeacherIDsForStudent :many
-- Reverse enrollment lookup: who teaches this student (the recipients of the
-- «забыл пароль» notification).
SELECT teacher_id FROM enrollments WHERE student_id = $1;

-- name: ListActiveAdminIDs :many
SELECT id FROM users WHERE role = 'admin' AND is_active;
