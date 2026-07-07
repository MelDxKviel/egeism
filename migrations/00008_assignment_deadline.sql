-- +goose Up
-- +goose StatementBegin
-- Deadline (срок сдачи) for an assignment. NULL = no deadline — the student
-- may solve it any time (the «репетитор» / self-paced case). When set, the
-- worker's overdue sweep flips a still-scheduled assignment to 'missed' once
-- due_at passes, so the teacher's roster and the student's feed show what's
-- overdue. The deadline stays soft: a missed assignment can still be solved
-- (late); finishing it flips missed → done, and "late" is derivable at read
-- time (finished_at > due_at). scheduled_at (the notify time) stays separate.
ALTER TABLE assignments ADD COLUMN due_at timestamptz;

-- Fast lookup of scheduled assignments whose deadline has passed, for the
-- worker's periodic overdue sweep (MarkOverdueAssignments).
CREATE INDEX idx_assignments_overdue
    ON assignments (due_at)
    WHERE status = 'scheduled' AND due_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_assignments_overdue;
ALTER TABLE assignments DROP COLUMN IF EXISTS due_at;
-- +goose StatementEnd
