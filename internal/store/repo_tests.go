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
	return s.assembleVariant(ctx, subjectID, domain.TestClassic, title, createdBy, ids)
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
	return s.assembleVariant(ctx, subjectID, domain.TestDrill, title, createdBy, ids)
}

// assembleVariant creates the test and its items in one transaction.
func (s *Store) assembleVariant(ctx context.Context, subjectID uuid.UUID, kind domain.TestKind, title string, createdBy uuid.UUID, taskIDs []uuid.UUID) (GeneratedVariant, error) {
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
		CreatedAt: t.CreatedAt,
	}
}
