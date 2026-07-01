package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// Heatmap returns per-day activity since `since` for the github-style grid.
func (s *Store) Heatmap(ctx context.Context, studentID uuid.UUID, since time.Time) ([]domain.HeatmapCell, error) {
	rows, err := s.q.HeatmapForStudent(ctx, sqlc.HeatmapForStudentParams{
		StudentID: studentID, AnsweredAt: since,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.HeatmapCell, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.HeatmapCell{Day: r.Day.Time, Total: r.Total, Correct: r.Correct})
	}
	return out, nil
}

// MasteryByNumber returns success per task number for a subject.
func (s *Store) MasteryByNumber(ctx context.Context, studentID, subjectID uuid.UUID) ([]domain.NumberMastery, error) {
	rows, err := s.q.MasteryByNumber(ctx, sqlc.MasteryByNumberParams{
		StudentID: studentID, SubjectID: subjectID,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.NumberMastery, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.NumberMastery{
			Number: int(r.Number), Total: r.Total, Correct: r.Correct, AvgTimeMS: r.AvgTimeMs,
		})
	}
	return out, nil
}

// WeakSpots returns up to `limit` worst task numbers with >= minAttempts tries.
func (s *Store) WeakSpots(ctx context.Context, studentID, subjectID uuid.UUID, minAttempts, limit int) ([]domain.WeakSpot, error) {
	rows, err := s.q.WeakSpots(ctx, sqlc.WeakSpotsParams{
		StudentID:   studentID,
		SubjectID:   subjectID,
		MinAttempts: int32(minAttempts),
		Lim:         int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.WeakSpot, 0, len(rows))
	for _, r := range rows {
		ws := domain.WeakSpot{
			Number: int(r.Number), Total: r.Total, Correct: r.Correct, AvgTimeMS: r.AvgTimeMs,
		}
		if r.Total > 0 {
			ws.Accuracy = float64(r.Correct) / float64(r.Total)
		}
		out = append(out, ws)
	}
	return out, nil
}

// SubjectAccuracy returns the overall solved ratio for one subject.
func (s *Store) SubjectAccuracy(ctx context.Context, studentID, subjectID uuid.UUID) (domain.SubjectAccuracy, error) {
	r, err := s.q.SubjectAccuracy(ctx, sqlc.SubjectAccuracyParams{
		StudentID: studentID, SubjectID: subjectID,
	})
	if err != nil {
		return domain.SubjectAccuracy{}, mapErr(err)
	}
	acc := domain.SubjectAccuracy{Total: r.Total, Correct: r.Correct}
	if r.Total > 0 {
		acc.Accuracy = float64(r.Correct) / float64(r.Total)
	}
	return acc, nil
}

// MasterySeries returns weekly per-number success buckets for the line chart.
func (s *Store) MasterySeries(ctx context.Context, studentID, subjectID uuid.UUID) ([]domain.MasteryPoint, error) {
	rows, err := s.q.MasterySeries(ctx, sqlc.MasterySeriesParams{StudentID: studentID, SubjectID: subjectID})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.MasteryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.MasteryPoint{
			Number: int(r.Number), Week: r.Week.Time, Total: r.Total, Correct: r.Correct,
		})
	}
	return out, nil
}

// AnswersOnDay is the heatmap drill-down for a single calendar day [start,end).
func (s *Store) AnswersOnDay(ctx context.Context, studentID uuid.UUID, start, end time.Time) ([]domain.DayAnswer, error) {
	rows, err := s.q.AnswersOnDay(ctx, sqlc.AnswersOnDayParams{
		StudentID: studentID, DayStart: start, DayEnd: end,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.DayAnswer, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.DayAnswer{
			AnswerID:    r.ID,
			TaskID:      r.TaskID,
			Number:      int(r.Number),
			SubjectID:   r.SubjectID,
			RawAnswer:   r.RawAnswer,
			IsCorrect:   r.IsCorrect,
			TimeSpentMS: r.TimeSpentMs,
			AnsweredAt:  r.AnsweredAt,
		})
	}
	return out, nil
}
