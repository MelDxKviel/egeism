-- +goose Up
-- +goose StatementBegin
-- Per-student variants for class assignments («каждому свой вариант», anti-
-- cheating): assigning a test to a class can generate each member a personal
-- test with the same number structure but randomly drawn tasks. variant_of
-- points at the source test so these clones stay out of the builder's test
-- list; SET NULL keeps a student's variant (and their attempt history) alive
-- if the source test is later deleted.
ALTER TABLE tests ADD COLUMN variant_of uuid REFERENCES tests(id) ON DELETE SET NULL;
CREATE INDEX idx_tests_variant_of ON tests (variant_of) WHERE variant_of IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tests DROP COLUMN IF EXISTS variant_of;
-- +goose StatementEnd
