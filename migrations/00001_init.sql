-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    role        text NOT NULL CHECK (role IN ('student', 'teacher')),
    telegram_id bigint UNIQUE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- teacher<->student m2m; ready for many, one row on stage 1.
CREATE TABLE enrollments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (teacher_id, student_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE subjects (
    id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code  text NOT NULL UNIQUE CHECK (code IN ('rus', 'math', 'inf', 'soc')),
    title text NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE tasks (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id    uuid NOT NULL REFERENCES subjects(id),
    number        int  NOT NULL,                       -- номер задания в ЕГЭ
    statement     text NOT NULL,
    media         jsonb NOT NULL DEFAULT '[]'::jsonb,  -- ссылки на объекты в MinIO
    answer_schema jsonb NOT NULL,                      -- как сравнивать (§7)
    source        jsonb,                               -- откуда пришло (ингест/дедуп)
    status        text NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft', 'active', 'rejected')),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tasks_subject_number ON tasks (subject_id, number);
CREATE INDEX idx_tasks_status ON tasks (status);
-- ingest dedup: one row per (provider, extern_id).
CREATE UNIQUE INDEX idx_tasks_source
    ON tasks ((source ->> 'provider'), (source ->> 'extern_id'))
    WHERE source IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE tests (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES subjects(id),
    kind       text NOT NULL CHECK (kind IN ('classic', 'drill')),
    title      text NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE test_items (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    test_id  uuid NOT NULL REFERENCES tests(id) ON DELETE CASCADE,
    task_id  uuid NOT NULL REFERENCES tasks(id),
    position int  NOT NULL,
    UNIQUE (test_id, position)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE assignments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    test_id      uuid NOT NULL REFERENCES tests(id),
    student_id   uuid NOT NULL REFERENCES users(id),
    assigned_by  uuid NOT NULL REFERENCES users(id),
    scheduled_at timestamptz NOT NULL,
    notified_at  timestamptz,
    status       text NOT NULL DEFAULT 'scheduled'
                 CHECK (status IN ('scheduled', 'done', 'missed'))
);
CREATE INDEX idx_assignments_student ON assignments (student_id);
-- fast lookup of assignments the scheduler still has to notify about.
CREATE INDEX idx_assignments_due ON assignments (scheduled_at) WHERE notified_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE attempts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id uuid REFERENCES assignments(id),   -- nullable: self-practice
    test_id       uuid NOT NULL REFERENCES tests(id),
    student_id    uuid NOT NULL REFERENCES users(id),
    started_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz
);
CREATE INDEX idx_attempts_student ON attempts (student_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE answers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id    uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    task_id       uuid NOT NULL REFERENCES tasks(id),
    raw_answer    text NOT NULL,
    is_correct    boolean NOT NULL,
    time_spent_ms bigint  NOT NULL DEFAULT 0,
    answered_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_answers_attempt ON answers (attempt_id);
CREATE INDEX idx_answers_task ON answers (task_id);
-- +goose StatementEnd

-- +goose StatementBegin
-- Seed the four subjects; codes are the stable identifiers used across the app.
INSERT INTO subjects (code, title) VALUES
    ('rus',  'Русский язык'),
    ('math', 'Математика'),
    ('inf',  'Информатика'),
    ('soc',  'Обществознание');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS answers;
DROP TABLE IF EXISTS attempts;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS test_items;
DROP TABLE IF EXISTS tests;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS subjects;
DROP TABLE IF EXISTS enrollments;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
