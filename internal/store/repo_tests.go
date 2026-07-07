package store

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

func (s *Store) CreateTest(ctx context.Context, subjectID uuid.UUID, kind domain.TestKind, title string, createdBy uuid.UUID) (domain.Test, error) {
	t, err := s.q.CreateTest(ctx, sqlc.CreateTestParams{
		SubjectID: subjectID, Kind: string(kind), Title: title, CreatedBy: createdBy,
	})
	if err != nil {
		return domain.Test{}, mapErr(err)
	}
	return toDomainTest(t), nil
}

// GetOrCreatePracticeTest returns the owner's ad-hoc practice test for a
// subject, creating it on first use. Free-solve answers are recorded against an
// attempt of this test; stats aggregate by task number regardless of test_items.
func (s *Store) GetOrCreatePracticeTest(ctx context.Context, subjectID, ownerID uuid.UUID) (domain.Test, error) {
	t, err := s.q.GetPracticeTest(ctx, sqlc.GetPracticeTestParams{SubjectID: subjectID, CreatedBy: ownerID})
	if err == nil {
		return toDomainTest(t), nil
	}
	if !errors.Is(mapErr(err), ErrNotFound) {
		return domain.Test{}, mapErr(err)
	}
	return s.CreateTest(ctx, subjectID, domain.TestDrill, "__practice__", ownerID)
}

func (s *Store) GetTest(ctx context.Context, id uuid.UUID) (domain.Test, error) {
	t, err := s.q.GetTest(ctx, id)
	if err != nil {
		return domain.Test{}, mapErr(err)
	}
	return toDomainTest(t), nil
}

// RenameTest updates a test's title so a teacher can tell distinct variants
// apart. Returns ErrNotFound if the test is gone.
func (s *Store) RenameTest(ctx context.Context, id uuid.UUID, title string) (domain.Test, error) {
	t, err := s.q.UpdateTestTitle(ctx, sqlc.UpdateTestTitleParams{ID: id, Title: title})
	if err != nil {
		return domain.Test{}, mapErr(err)
	}
	return toDomainTest(t), nil
}

// DeleteTest removes a test and its items (test_items cascade). Any assignments
// of the test are removed too. A test that has been attempted is left intact and
// ErrInUse is returned, so student history is never orphaned. Runs in one tx.
func (s *Store) DeleteTest(ctx context.Context, id uuid.UUID) error {
	if _, err := s.q.GetTest(ctx, id); err != nil {
		return mapErr(err) // ErrNotFound → 404
	}
	used, err := s.q.TestHasAttempts(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if used {
		return ErrInUse
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	if err := qtx.DeleteAssignmentsByTest(ctx, id); err != nil {
		return mapErr(err)
	}
	if _, err := qtx.DeleteTest(ctx, id); err != nil {
		return mapErr(err)
	}
	return tx.Commit(ctx)
}

func (s *Store) ListTests(ctx context.Context, subjectID *uuid.UUID) ([]domain.Test, error) {
	rows, err := s.q.ListTests(ctx, subjectID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Test, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainTest(r))
	}
	return out, nil
}

func (s *Store) AddTestItem(ctx context.Context, testID, taskID uuid.UUID, position int) (domain.TestItem, error) {
	item, err := s.q.AddTestItem(ctx, sqlc.AddTestItemParams{
		TestID: testID, TaskID: taskID, Position: int32(position),
	})
	if err != nil {
		return domain.TestItem{}, mapErr(err)
	}
	return domain.TestItem{
		ID: item.ID, TestID: item.TestID, TaskID: item.TaskID, Position: int(item.Position),
	}, nil
}

// ListTestTasks returns the tasks of a test in position order (domain.Task).
func (s *Store) ListTestTasks(ctx context.Context, testID uuid.UUID) ([]domain.Task, error) {
	rows, err := s.q.ListTestItems(ctx, testID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Task, 0, len(rows))
	for _, r := range rows {
		schema, err := domain.ParseAnswerSchema(r.AnswerSchema)
		if err != nil {
			return nil, err
		}
		var media []domain.Media
		if len(r.Media) > 0 {
			if err := unmarshalMedia(r.Media, &media); err != nil {
				return nil, err
			}
		}
		out = append(out, domain.Task{
			ID:           r.TaskID,
			SubjectID:    r.SubjectID,
			Number:       int(r.Number),
			Statement:    r.Statement,
			Media:        media,
			AnswerSchema: schema,
			Status:       domain.TaskStatus(r.Status),
		})
	}
	return out, nil
}

// GeneratedVariant is the result of auto-building a test from the bank.
type GeneratedVariant struct {
	Test      domain.Test
	TaskCount int
}

// GenerateClassicVariant builds a "classic" test picking one random active task
// per number for the subject — a full exam-shaped variant in one click.
func (s *Store) GenerateClassicVariant(ctx context.Context, subjectID, createdBy uuid.UUID, title string) (GeneratedVariant, error) {
	rows, err := s.q.RandomTasksOnePerNumber(ctx, subjectID)
	if err != nil {
		return GeneratedVariant{}, mapErr(err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return s.assembleVariant(ctx, subjectID, domain.TestClassic, title, createdBy, ids, nil)
}

// GenerateDrillVariant builds a "drill" test of up to count random active tasks
// of a single number.
func (s *Store) GenerateDrillVariant(ctx context.Context, subjectID uuid.UUID, number, count int, createdBy uuid.UUID, title string) (GeneratedVariant, error) {
	if count <= 0 {
		count = 10
	}
	ids, err := s.q.RandomTasksForNumber(ctx, sqlc.RandomTasksForNumberParams{
		SubjectID: subjectID, Number: int32(number), Limit: int32(count),
	})
	if err != nil {
		return GeneratedVariant{}, mapErr(err)
	}
	return s.assembleVariant(ctx, subjectID, domain.TestDrill, title, createdBy, ids, nil)
}

// VariantSlot requests Count random active tasks of задание Number for a
// composed variant.
type VariantSlot struct {
	Number int
	Count  int
}

// GenerateComposedVariant builds a teacher-composed test: for each requested
// number it draws Count random ACTIVE bank tasks, laid out grouped by number in
// the order given (so "3 первых, 3 вторых, 3 третьих" comes out exactly that
// way). Numbers repeated across slots are summed and drawn once, so a task never
// appears twice. A number whose bank ran short simply yields fewer tasks (no
// padding) — the caller compares TaskCount to what was asked and warns.
func (s *Store) GenerateComposedVariant(ctx context.Context, subjectID uuid.UUID, slots []VariantSlot, createdBy uuid.UUID, title string) (GeneratedVariant, error) {
	need := map[int]int{}
	order := make([]int, 0, len(slots))
	for _, sl := range slots {
		if sl.Count <= 0 {
			continue
		}
		if _, ok := need[sl.Number]; !ok {
			order = append(order, sl.Number)
		}
		need[sl.Number] += sl.Count
	}
	taskIDs := make([]uuid.UUID, 0)
	for _, number := range order {
		ids, err := s.q.RandomTasksForNumber(ctx, sqlc.RandomTasksForNumberParams{
			SubjectID: subjectID, Number: int32(number), Limit: int32(need[number]),
		})
		if err != nil {
			return GeneratedVariant{}, mapErr(err)
		}
		taskIDs = append(taskIDs, ids...)
	}
	return s.assembleVariant(ctx, subjectID, domain.TestComposed, title, createdBy, taskIDs, nil)
}

// GenerateVariantLike builds a personal clone of a source test: the same
// subject, kind and number structure (position for position), but each slot
// drawn randomly from the ACTIVE bank — so a class assignment can hand every
// student their own variant («чтобы не списывали»). Slots whose number has no
// active bank tasks left fall back to the source task, so the clone never
// comes out shorter than the original.
func (s *Store) GenerateVariantLike(ctx context.Context, source domain.Test, createdBy uuid.UUID, title string) (GeneratedVariant, error) {
	items, err := s.q.ListTestItems(ctx, source.ID)
	if err != nil {
		return GeneratedVariant{}, mapErr(err)
	}
	if len(items) == 0 {
		return GeneratedVariant{}, ErrNotFound
	}
	// How many tasks of each number the clone needs.
	need := map[int32]int32{}
	for _, it := range items {
		need[it.Number]++
	}
	// Draw that many random active tasks per number in one query each.
	pool := map[int32][]uuid.UUID{}
	for number, count := range need {
		ids, err := s.q.RandomTasksForNumber(ctx, sqlc.RandomTasksForNumberParams{
			SubjectID: source.SubjectID, Number: number, Limit: count,
		})
		if err != nil {
			return GeneratedVariant{}, mapErr(err)
		}
		pool[number] = ids
	}
	// Fill the source's positions from the pools; dried-up pools keep the
	// source task so the structure survives a thin bank.
	taskIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		if ids := pool[it.Number]; len(ids) > 0 {
			taskIDs = append(taskIDs, ids[0])
			pool[it.Number] = ids[1:]
		} else {
			taskIDs = append(taskIDs, it.TaskID)
		}
	}
	return s.assembleVariant(ctx, source.SubjectID, source.Kind, title, createdBy, taskIDs, &source.ID)
}

// assembleVariant creates the test and its items in one transaction. variantOf
// marks the test as a per-student clone of a source test (nil = a normal test).
func (s *Store) assembleVariant(ctx context.Context, subjectID uuid.UUID, kind domain.TestKind, title string, createdBy uuid.UUID, taskIDs []uuid.UUID, variantOf *uuid.UUID) (GeneratedVariant, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GeneratedVariant{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	test, err := qtx.CreateTest(ctx, sqlc.CreateTestParams{
		SubjectID: subjectID, Kind: string(kind), Title: title, CreatedBy: createdBy,
	})
	if err != nil {
		return GeneratedVariant{}, mapErr(err)
	}
	if variantOf != nil {
		if err := qtx.SetTestVariantOf(ctx, sqlc.SetTestVariantOfParams{ID: test.ID, VariantOf: variantOf}); err != nil {
			return GeneratedVariant{}, mapErr(err)
		}
		test.VariantOf = variantOf
	}
	for i, taskID := range taskIDs {
		if _, err := qtx.AddTestItem(ctx, sqlc.AddTestItemParams{
			TestID: test.ID, TaskID: taskID, Position: int32(i + 1),
		}); err != nil {
			return GeneratedVariant{}, mapErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedVariant{}, err
	}
	return GeneratedVariant{Test: toDomainTest(test), TaskCount: len(taskIDs)}, nil
}

func toDomainTest(t sqlc.Test) domain.Test {
	return domain.Test{
		ID:        t.ID,
		SubjectID: t.SubjectID,
		Kind:      domain.TestKind(t.Kind),
		Title:     t.Title,
		CreatedBy: t.CreatedBy,
		VariantOf: t.VariantOf,
		CreatedAt: t.CreatedAt,
	}
}
