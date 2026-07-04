-- +goose Up
-- +goose StatementBegin
-- Multi-user overhaul: the platform grows an 'admin' role (user management +
-- platform stats), account activation, and per-subject teacher scoping.
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('student', 'teacher', 'admin'));
-- Deactivated accounts keep all their history but can't log in or act.
ALTER TABLE users ADD COLUMN is_active boolean NOT NULL DEFAULT true;
-- A teacher's subject scope. NULL on a teacher = "сверхучитель" (any subject);
-- always NULL for students and admins. Values mirror subjects.code.
ALTER TABLE users ADD COLUMN subject text CHECK (subject IN ('rus', 'math', 'inf', 'soc'));
-- +goose StatementEnd

-- +goose StatementBegin
-- A teacher's class (группа учеников). Students may also exist with no class
-- (репетитор case) — membership is optional. Deleting a class or a teacher
-- unlinks students, never deletes them.
CREATE TABLE classes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_classes_teacher ON classes (teacher_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Class membership (m2m: a student can be in classes of different teachers,
-- e.g. русский and математика). Adding a member also creates an enrollments
-- row (the teacher↔student link stats/assignment authorization run on).
CREATE TABLE class_members (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id   uuid NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (class_id, student_id)
);
CREATE INDEX idx_class_members_student ON class_members (student_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS class_members;
DROP TABLE IF EXISTS classes;
ALTER TABLE users DROP COLUMN IF EXISTS subject;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('student', 'teacher'));
-- +goose StatementEnd
