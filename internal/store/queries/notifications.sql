-- name: CreateNotification :exec
INSERT INTO notifications (user_id, kind, assignment_id)
VALUES ($1, $2, $3);

-- name: ListNotificationsForUser :many
-- The web bell feed: notification joined with its assignment/test/student so
-- the UI can render «назначен тест …» / «N решил тест …» and jump to the test.
SELECT n.id, n.kind, n.assignment_id, n.read_at, n.created_at,
       a.test_id, a.student_id, a.scheduled_at, a.status AS assignment_status,
       t.title AS test_title, t.subject_id,
       su.name AS student_name
FROM notifications n
JOIN assignments a ON a.id = n.assignment_id
JOIN tests t ON t.id = a.test_id
JOIN users su ON su.id = a.student_id
WHERE n.user_id = $1
ORDER BY n.created_at DESC
LIMIT $2;

-- name: CountUnreadNotifications :one
-- Exact badge count (the list itself is limit-bounded).
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
-- user_id guards against marking someone else's notification.
UPDATE notifications SET read_at = now()
WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL;
