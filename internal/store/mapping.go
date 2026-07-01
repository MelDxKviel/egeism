package store

import (
	"encoding/json"
	"fmt"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// toDomainTask converts a sqlc row into a domain.Task, decoding JSONB columns.
func toDomainTask(t sqlc.Task) (domain.Task, error) {
	var media []domain.Media
	if len(t.Media) > 0 {
		if err := json.Unmarshal(t.Media, &media); err != nil {
			return domain.Task{}, fmt.Errorf("task %s media: %w", t.ID, err)
		}
	}
	schema, err := domain.ParseAnswerSchema(t.AnswerSchema)
	if err != nil {
		return domain.Task{}, fmt.Errorf("task %s: %w", t.ID, err)
	}
	var src *domain.Source
	if len(t.Source) > 0 {
		var s domain.Source
		if err := json.Unmarshal(t.Source, &s); err != nil {
			return domain.Task{}, fmt.Errorf("task %s source: %w", t.ID, err)
		}
		src = &s
	}
	return domain.Task{
		ID:           t.ID,
		SubjectID:    t.SubjectID,
		Number:       int(t.Number),
		Statement:    t.Statement,
		Media:        media,
		AnswerSchema: schema,
		Source:       src,
		Status:       domain.TaskStatus(t.Status),
		CreatedAt:    t.CreatedAt,
	}, nil
}

func toDomainTasks(rows []sqlc.Task) ([]domain.Task, error) {
	out := make([]domain.Task, 0, len(rows))
	for _, r := range rows {
		t, err := toDomainTask(r)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func toDomainUser(u sqlc.User) domain.User {
	return domain.User{
		ID:         u.ID,
		Role:       domain.Role(u.Role),
		TelegramID: u.TelegramID,
		Username:   u.Username,
		Name:       u.Name,
		CreatedAt:  u.CreatedAt,
	}
}

func toDomainSubject(s sqlc.Subject) domain.Subject {
	return domain.Subject{ID: s.ID, Code: domain.SubjectCode(s.Code), Title: s.Title}
}

func toDomainAttempt(a sqlc.Attempt) domain.Attempt {
	return domain.Attempt{
		ID:           a.ID,
		AssignmentID: a.AssignmentID,
		TestID:       a.TestID,
		StudentID:    a.StudentID,
		StartedAt:    a.StartedAt,
		FinishedAt:   a.FinishedAt,
	}
}

func toDomainAnswer(a sqlc.Answer) domain.Answer {
	return domain.Answer{
		ID:          a.ID,
		AttemptID:   a.AttemptID,
		TaskID:      a.TaskID,
		RawAnswer:   a.RawAnswer,
		IsCorrect:   a.IsCorrect,
		TimeSpentMS: a.TimeSpentMs,
		AnsweredAt:  a.AnsweredAt,
	}
}

// unmarshalMedia decodes a media JSONB blob into dst.
func unmarshalMedia(blob []byte, dst *[]domain.Media) error {
	if err := json.Unmarshal(blob, dst); err != nil {
		return fmt.Errorf("decode media: %w", err)
	}
	return nil
}

// mustJSON marshals a value known to be serializable (media/source/schema).
func mustJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal jsonb: %w", err)
	}
	return b, nil
}
