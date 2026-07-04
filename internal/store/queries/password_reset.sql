-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (token, user_id, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetValidPasswordResetToken :one
SELECT * FROM password_reset_tokens
WHERE token = $1 AND used_at IS NULL AND expires_at > now();

-- name: MarkPasswordResetTokenUsed :exec
UPDATE password_reset_tokens SET used_at = now() WHERE token = $1;
