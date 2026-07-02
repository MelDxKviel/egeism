-- +goose Up
-- +goose StatementBegin
-- In-app (web) notifications, one row per event per recipient. Stage 1 covers
-- the assignment lifecycle: the student is notified when a test is assigned,
-- the teacher when the student completes it. The row only references the
-- assignment — title/subject/names are joined at read time, so renames never
-- leave stale text. ON DELETE CASCADE: DeleteTest removes assignments outright
-- (repo_tests.go), and a notification without its assignment is meaningless.
CREATE TABLE notifications (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('assignment_created', 'assignment_done')),
    assignment_id uuid NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    read_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, created_at DESC);
-- fast unread-badge count.
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE read_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notifications;
-- +goose StatementEnd
