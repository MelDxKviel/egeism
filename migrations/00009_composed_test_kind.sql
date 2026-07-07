-- +goose Up
-- +goose StatementBegin
-- Allow the new «составной» variant kind. The composed builder assembles a
-- teacher-defined per-номер mix (e.g. 3×№1 + 3×№2 + 3×№3) and stores the test
-- with kind = 'composed'; the original inline CHECK (migration 00001) only
-- permitted classic/drill, so every composed build — and every individual
-- clone of one (GenerateVariantLike keeps source.Kind) — failed the constraint
-- with a 500. Widen it to include 'composed'.
ALTER TABLE tests DROP CONSTRAINT tests_kind_check;
ALTER TABLE tests ADD CONSTRAINT tests_kind_check
    CHECK (kind IN ('classic', 'drill', 'composed'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Composed tests can't exist under the narrower CHECK, so remove them and their
-- dependents first (answers → attempts → assignments; test_items cascade on the
-- test delete). variant_of clones of a composed test are themselves 'composed',
-- so this sweep covers them too.
DELETE FROM answers WHERE attempt_id IN (
    SELECT a.id FROM attempts a JOIN tests t ON t.id = a.test_id WHERE t.kind = 'composed'
);
DELETE FROM attempts WHERE test_id IN (SELECT id FROM tests WHERE kind = 'composed');
DELETE FROM assignments WHERE test_id IN (SELECT id FROM tests WHERE kind = 'composed');
DELETE FROM tests WHERE kind = 'composed';
ALTER TABLE tests DROP CONSTRAINT tests_kind_check;
ALTER TABLE tests ADD CONSTRAINT tests_kind_check
    CHECK (kind IN ('classic', 'drill'));
-- +goose StatementEnd
