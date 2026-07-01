-- name: ListSubjects :many
SELECT * FROM subjects ORDER BY code;

-- name: GetSubject :one
SELECT * FROM subjects WHERE id = $1;

-- name: GetSubjectByCode :one
SELECT * FROM subjects WHERE code = $1;
