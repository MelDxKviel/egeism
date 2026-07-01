-- name: CreateTelegramLinkCode :one
INSERT INTO telegram_link_codes (code, user_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetValidTelegramLinkCode :one
SELECT * FROM telegram_link_codes
WHERE code = $1 AND used_at IS NULL AND expires_at > now();

-- name: MarkTelegramLinkCodeUsed :exec
UPDATE telegram_link_codes SET used_at = now() WHERE code = $1;

-- name: LinkTelegramToUser :one
UPDATE users SET telegram_id = $2 WHERE id = $1
RETURNING *;
