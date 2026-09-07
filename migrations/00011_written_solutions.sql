-- +goose Up
ALTER TABLE tasks ADD COLUMN part integer NOT NULL DEFAULT 1 CHECK (part IN (1, 2));
ALTER TABLE tasks ADD COLUMN grading_mode text NOT NULL DEFAULT 'auto' CHECK (grading_mode IN ('auto', 'manual'));
ALTER TABLE tasks ADD COLUMN max_points integer NOT NULL DEFAULT 1 CHECK (max_points > 0);
UPDATE tasks SET part = 2, grading_mode = 'manual',
    max_points = CASE WHEN number IN (13, 15, 16) THEN 2 WHEN number IN (14, 17) THEN 3 ELSE 4 END
WHERE subject_id IN (SELECT id FROM subjects WHERE code = 'math') AND number BETWEEN 13 AND 19;

ALTER TABLE assignments ADD COLUMN require_solution boolean NOT NULL DEFAULT false;
ALTER TABLE answers ADD COLUMN review_status text NOT NULL DEFAULT 'auto' CHECK (review_status IN ('auto', 'pending', 'reviewed'));
ALTER TABLE answers ADD COLUMN points integer;
ALTER TABLE answers ADD COLUMN max_points integer NOT NULL DEFAULT 1 CHECK (max_points > 0);
ALTER TABLE answers ADD COLUMN teacher_comment text NOT NULL DEFAULT '';
ALTER TABLE answers ADD COLUMN reviewed_by uuid REFERENCES users(id);
ALTER TABLE answers ADD COLUMN reviewed_at timestamptz;
ALTER TABLE answers ADD CONSTRAINT answers_points_check CHECK (points >= 0 AND points <= max_points);
UPDATE answers SET points = CASE WHEN is_correct THEN 1 ELSE 0 END;

CREATE TABLE solution_photos (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    task_id uuid NOT NULL REFERENCES tasks(id),
    object_key text NOT NULL UNIQUE,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX solution_photos_attempt_task ON solution_photos(attempt_id, task_id);
ALTER TABLE notifications ADD COLUMN attempt_id uuid REFERENCES attempts(id) ON DELETE CASCADE;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('assignment_created', 'assignment_done', 'password_reset_requested', 'attempt_reviewed'));
ALTER TABLE notifications DROP CONSTRAINT notifications_ref_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_ref_check CHECK (
    CASE WHEN kind = 'password_reset_requested' THEN subject_user_id IS NOT NULL
         WHEN kind = 'attempt_reviewed' THEN attempt_id IS NOT NULL
         ELSE assignment_id IS NOT NULL END);

CREATE TABLE practice_fetch_limits (
    subject_id uuid PRIMARY KEY REFERENCES subjects(id),
    requested_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE practice_fetch_limits;
DELETE FROM notifications WHERE kind = 'attempt_reviewed';
ALTER TABLE notifications DROP CONSTRAINT notifications_ref_check;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('assignment_created', 'assignment_done', 'password_reset_requested'));
ALTER TABLE notifications ADD CONSTRAINT notifications_ref_check CHECK (CASE WHEN kind = 'password_reset_requested' THEN subject_user_id IS NOT NULL ELSE assignment_id IS NOT NULL END);
ALTER TABLE notifications DROP COLUMN attempt_id;
DROP TABLE solution_photos;
ALTER TABLE answers DROP CONSTRAINT answers_points_check;
ALTER TABLE answers DROP COLUMN review_status, DROP COLUMN points, DROP COLUMN max_points, DROP COLUMN teacher_comment, DROP COLUMN reviewed_by, DROP COLUMN reviewed_at;
ALTER TABLE assignments DROP COLUMN require_solution;
ALTER TABLE tasks DROP COLUMN part, DROP COLUMN grading_mode, DROP COLUMN max_points;
