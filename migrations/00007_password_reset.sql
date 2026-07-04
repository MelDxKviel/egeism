-- +goose Up
-- +goose StatementBegin
-- One-time password-reset tokens. A teacher (for their student) or an admin
-- (for anyone) issues a token; the reset page redeems it with a new password
-- and stamps used_at. Valid for 1 hour (resetTokenTTL) — an expired link means
-- issuing a fresh one. created_by records who issued it; SET NULL so deleting
-- the issuer never blocks or loses the student's pending reset.
CREATE TABLE password_reset_tokens (
    token      text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_pw_reset_tokens_user ON password_reset_tokens (user_id);

-- The «забыл пароль» notification is about a user, not an assignment: relax
-- assignment_id to nullable, add the subject user (whose password it is), and
-- keep integrity per kind — assignment kinds still require the assignment,
-- the password kind requires the subject user. ON DELETE CASCADE: a
-- notification about a deleted account is meaningless.
ALTER TABLE notifications ALTER COLUMN assignment_id DROP NOT NULL;
ALTER TABLE notifications ADD COLUMN subject_user_id uuid REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('assignment_created', 'assignment_done', 'password_reset_requested'));
ALTER TABLE notifications ADD CONSTRAINT notifications_ref_check
    CHECK (CASE WHEN kind = 'password_reset_requested'
                THEN subject_user_id IS NOT NULL
                ELSE assignment_id IS NOT NULL END);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM notifications WHERE kind = 'password_reset_requested';
ALTER TABLE notifications DROP CONSTRAINT notifications_ref_check;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('assignment_created', 'assignment_done'));
ALTER TABLE notifications DROP COLUMN subject_user_id;
ALTER TABLE notifications ALTER COLUMN assignment_id SET NOT NULL;
DROP TABLE IF EXISTS password_reset_tokens;
-- +goose StatementEnd
