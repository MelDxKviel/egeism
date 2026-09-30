package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"egeism/internal/checker"
	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
	"github.com/google/uuid"
)

// ValidationError is safe to display to the user.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }
func invalid(message string) error      { return ValidationError{message} }

func submissionTask(ctx context.Context, q *sqlc.Queries, att sqlc.Attempt, taskID uuid.UUID) (domain.Task, error) {
	test, err := q.GetTest(ctx, att.TestID)
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	row, err := q.GetTask(ctx, taskID)
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	if row.SubjectID != test.SubjectID {
		return domain.Task{}, invalid("Задание из другого предмета")
	}
	if test.Title == "__practice__" && test.CreatedBy == att.StudentID {
		if row.Status != "active" {
			return domain.Task{}, invalid("Задание недоступно для тренировки")
		}
		task, err := toDomainTask(row)
		if err != nil {
			return domain.Task{}, err
		}
		if !task.Source.Current(time.Now()) {
			return domain.Task{}, invalid("Актуальность задания не подтверждена. Выбери новую тренировку.")
		}
	} else {
		items, err := q.ListTestItems(ctx, test.ID)
		if err != nil {
			return domain.Task{}, err
		}
		found := false
		for _, item := range items {
			if item.TaskID == taskID {
				found = true
				break
			}
		}
		if !found {
			return domain.Task{}, invalid("Задание не входит в этот тест")
		}
	}
	return toDomainTask(row)
}

func (s *Store) SubmitAnswer(ctx context.Context, attemptID, taskID uuid.UUID, raw string, timeMS int64) (domain.Answer, error) {
	if utf8.RuneCountInString(raw) > 10000 || timeMS < 0 {
		return domain.Answer{}, invalid("Некорректный ответ или время решения")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Answer{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	att, err := q.LockAttempt(ctx, attemptID)
	if err != nil {
		return domain.Answer{}, mapErr(err)
	}
	if att.FinishedAt != nil {
		return domain.Answer{}, invalid("Тест уже сдан")
	}
	task, err := submissionTask(ctx, q, att, taskID)
	if err != nil {
		return domain.Answer{}, err
	}
	answers, err := q.ListAnswersForAttempt(ctx, attemptID)
	if err != nil {
		return domain.Answer{}, err
	}
	test, err := q.GetTest(ctx, att.TestID)
	if err != nil {
		return domain.Answer{}, err
	}
	// Free practice can revisit a short-answer task in the same session.
	// Assigned tests and written solutions are submitted only once.
	repeatable := test.Title == "__practice__" && test.CreatedBy == att.StudentID && task.GradingMode == "auto"
	for _, a := range answers {
		if a.TaskID == taskID && !repeatable {
			return domain.Answer{}, invalid("Ответ уже отправлен")
		}
	}
	status := "auto"
	points := int32(0)
	var score *int32 = &points
	correct := false
	if task.GradingMode == "manual" {
		status, score = "pending", nil
		photos, err := q.ListSolutionPhotos(ctx, attemptID)
		if err != nil {
			return domain.Answer{}, err
		}
		hasPhoto := false
		for _, p := range photos {
			if p.TaskID == taskID {
				hasPhoto = true
			}
		}
		if att.AssignmentID != nil {
			asg, err := q.GetAssignment(ctx, *att.AssignmentID)
			if err != nil {
				return domain.Answer{}, err
			}
			if asg.RequireSolution && !hasPhoto {
				return domain.Answer{}, invalid("Прикрепи фотографию решения второй части")
			}
		}
		if strings.TrimSpace(raw) == "" && !hasPhoto {
			return domain.Answer{}, invalid("Напиши ответ или прикрепи фотографию решения")
		}
	} else {
		if strings.TrimSpace(raw) == "" {
			return domain.Answer{}, invalid("Введите ответ")
		}
		correct = checker.Check(task.AnswerSchema, raw)
		if correct {
			points = int32(task.MaxPoints)
		}
	}
	row, err := q.InsertSubmission(ctx, sqlc.InsertSubmissionParams{AttemptID: attemptID, TaskID: taskID, RawAnswer: raw, IsCorrect: correct, TimeSpentMs: timeMS, ReviewStatus: status, Points: score, MaxPoints: int32(task.MaxPoints)})
	if err != nil {
		return domain.Answer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Answer{}, err
	}
	return toDomainAnswer(row), nil
}

func (s *Store) FinishSubmission(ctx context.Context, id uuid.UUID) (domain.Attempt, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Attempt{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	att, err := q.LockAttempt(ctx, id)
	if err != nil {
		return domain.Attempt{}, mapErr(err)
	}
	if att.FinishedAt != nil {
		return toDomainAttempt(att), nil
	}
	required := false
	if att.AssignmentID != nil {
		asg, err := q.GetAssignment(ctx, *att.AssignmentID)
		if err != nil {
			return domain.Attempt{}, err
		}
		required = asg.RequireSolution
	}
	items, err := q.ListTestItems(ctx, att.TestID)
	if err != nil {
		return domain.Attempt{}, err
	}
	photos, err := q.ListSolutionPhotos(ctx, id)
	if err != nil {
		return domain.Attempt{}, err
	}
	photoTasks := map[uuid.UUID]bool{}
	for _, p := range photos {
		photoTasks[p.TaskID] = true
	}
	answers, err := q.ListAnswersForAttempt(ctx, id)
	if err != nil {
		return domain.Attempt{}, err
	}
	answered := map[uuid.UUID]bool{}
	for _, a := range answers {
		answered[a.TaskID] = true
	}
	for _, item := range items {
		if item.GradingMode == "manual" && required && !photoTasks[item.TaskID] {
			return domain.Attempt{}, invalid(fmt.Sprintf("Прикрепи решение задания №%d перед сдачей теста", item.Number))
		}
		if answered[item.TaskID] {
			continue
		}
		status, points := "auto", int32(0)
		var score *int32 = &points
		if item.GradingMode == "manual" {
			status, score = "pending", nil
		}
		if _, err := q.InsertSubmission(ctx, sqlc.InsertSubmissionParams{AttemptID: id, TaskID: item.TaskID, ReviewStatus: status, Points: score, MaxPoints: item.MaxPoints}); err != nil {
			return domain.Attempt{}, err
		}
		answered[item.TaskID] = true
	}
	// Ad-hoc practice has no test_items. Preserve an uploaded solution even
	// when the student presses Finish before separately submitting its answer.
	for taskID := range photoTasks {
		if answered[taskID] {
			continue
		}
		task, err := submissionTask(ctx, q, att, taskID)
		if err != nil {
			return domain.Attempt{}, err
		}
		if _, err := q.InsertSubmission(ctx, sqlc.InsertSubmissionParams{AttemptID: id, TaskID: taskID, ReviewStatus: "pending", MaxPoints: int32(task.MaxPoints)}); err != nil {
			return domain.Attempt{}, err
		}
	}
	att, err = q.FinishAttempt(ctx, id)
	if err != nil {
		return domain.Attempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Attempt{}, err
	}
	return toDomainAttempt(att), nil
}

func toPhoto(p sqlc.SolutionPhoto) domain.SolutionPhoto {
	return domain.SolutionPhoto{ID: p.ID, AttemptID: p.AttemptID, TaskID: p.TaskID, ObjectKey: p.ObjectKey, ContentType: p.ContentType, SizeBytes: p.SizeBytes}
}
func (s *Store) SolutionPhoto(ctx context.Context, id uuid.UUID) (domain.SolutionPhoto, error) {
	p, err := s.q.GetSolutionPhoto(ctx, id)
	return toPhoto(p), mapErr(err)
}
func (s *Store) SolutionPhotos(ctx context.Context, id uuid.UUID) ([]domain.SolutionPhoto, error) {
	rows, err := s.q.ListSolutionPhotos(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []domain.SolutionPhoto{}
	for _, p := range rows {
		out = append(out, toPhoto(p))
	}
	return out, nil
}

// mutatePhoto shares the attempt lock with submission/finish, so attachments
// cannot change after the answer has been submitted, even with concurrent tabs.
func (s *Store) MutateSolutionPhoto(ctx context.Context, p domain.SolutionPhoto, remove bool) (domain.SolutionPhoto, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.SolutionPhoto{}, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	att, err := q.LockAttempt(ctx, p.AttemptID)
	if err != nil {
		return domain.SolutionPhoto{}, mapErr(err)
	}
	if att.FinishedAt != nil {
		return domain.SolutionPhoto{}, invalid("Тест уже сдан")
	}
	task, err := submissionTask(ctx, q, att, p.TaskID)
	if err != nil {
		return domain.SolutionPhoto{}, err
	}
	if task.GradingMode != "manual" {
		return domain.SolutionPhoto{}, invalid("Фотографии доступны для заданий с проверкой учителем")
	}
	answers, err := q.ListAnswersForAttempt(ctx, p.AttemptID)
	if err != nil {
		return domain.SolutionPhoto{}, err
	}
	for _, a := range answers {
		if a.TaskID == p.TaskID {
			return domain.SolutionPhoto{}, invalid("Ответ уже отправлен")
		}
	}
	if remove {
		err = q.DeleteSolutionPhoto(ctx, p.ID)
	} else {
		photos, e := q.ListSolutionPhotos(ctx, p.AttemptID)
		if e != nil {
			return domain.SolutionPhoto{}, e
		}
		count := 0
		for _, photo := range photos {
			if photo.TaskID == p.TaskID {
				count++
			}
		}
		if count >= 5 {
			return domain.SolutionPhoto{}, invalid("Не больше 5 фотографий на задание")
		}
		var row sqlc.SolutionPhoto
		row, err = q.AddSolutionPhoto(ctx, sqlc.AddSolutionPhotoParams{AttemptID: p.AttemptID, TaskID: p.TaskID, ObjectKey: p.ObjectKey, ContentType: p.ContentType, SizeBytes: p.SizeBytes})
		p = toPhoto(row)
	}
	if err != nil {
		return domain.SolutionPhoto{}, err
	}
	return p, tx.Commit(ctx)
}

func (s *Store) ReviewAttempt(ctx context.Context, attemptID, teacherID uuid.UUID, grades []domain.AnswerGrade) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	att, err := q.LockAttempt(ctx, attemptID)
	if err != nil {
		return mapErr(err)
	}
	if att.FinishedAt == nil {
		return invalid("Ученик ещё не сдал тест")
	}
	answers, err := q.ListAnswersForAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	manual := map[uuid.UUID]sqlc.Answer{}
	for _, a := range answers {
		if a.ReviewStatus != "auto" {
			manual[a.ID] = a
		}
	}
	if len(manual) == 0 || len(grades) != len(manual) {
		return invalid("Выставь баллы за все задания второй части")
	}
	changed := false
	for _, g := range grades {
		a, ok := manual[g.AnswerID]
		if !ok {
			return invalid("Некорректное или повторное задание для проверки")
		}
		delete(manual, g.AnswerID)
		if g.Points == nil || *g.Points < 0 || *g.Points > a.MaxPoints || utf8.RuneCountInString(g.Comment) > 5000 {
			return invalid("Проверь баллы и длину комментария (до 5000 символов)")
		}
		if a.ReviewStatus == "reviewed" && a.Points != nil && *a.Points == *g.Points && a.TeacherComment == g.Comment {
			continue
		}
		changed = true
		if _, err := q.GradeAnswer(ctx, sqlc.GradeAnswerParams{AttemptID: attemptID, ID: g.AnswerID, Points: g.Points, TeacherComment: g.Comment, ReviewedBy: &teacherID}); err != nil {
			return err
		}
	}
	if changed {
		if err := q.CreateReviewNotification(ctx, sqlc.CreateReviewNotificationParams{UserID: att.StudentID, AssignmentID: att.AssignmentID, AttemptID: &attemptID}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecentNumberPerformance(ctx context.Context, studentID, subjectID uuid.UUID) ([]domain.RecentNumber, error) {
	rows, err := s.q.RecentNumberPerformance(ctx, sqlc.RecentNumberPerformanceParams{StudentID: studentID, SubjectID: subjectID})
	if err != nil {
		return nil, err
	}
	out := []domain.RecentNumber{}
	for _, row := range rows {
		out = append(out, domain.RecentNumber{Number: int(row.Number), Total: row.Total, Correct: row.Correct})
	}
	return out, nil
}
func (s *Store) ClaimPracticeFetch(ctx context.Context, subjectID uuid.UUID) (bool, error) {
	n, err := s.q.ClaimPracticeFetch(ctx, subjectID)
	return n > 0, err
}
