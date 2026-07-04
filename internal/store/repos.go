package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// ---- Users ----

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	u, err := s.q.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

func (s *Store) GetUserByTelegram(ctx context.Context, tgID int64) (domain.User, error) {
	u, err := s.q.GetUserByTelegram(ctx, &tgID)
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

func (s *Store) CreateUser(ctx context.Context, role domain.Role, tgID *int64, name string) (domain.User, error) {
	u, err := s.q.CreateUser(ctx, sqlc.CreateUserParams{Role: string(role), TelegramID: tgID, Name: name})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

// GetOrCreateStudentByTelegram resolves a Telegram user to a student row,
// provisioning one on first contact. Returns created=true if a new row was made.
func (s *Store) GetOrCreateStudentByTelegram(ctx context.Context, tgID int64, name string) (domain.User, bool, error) {
	u, err := s.GetUserByTelegram(ctx, tgID)
	if err == nil {
		return u, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.User{}, false, err
	}
	created, err := s.CreateUser(ctx, domain.RoleStudent, &tgID, name)
	if err != nil {
		return domain.User{}, false, err
	}
	return created, true, nil
}

// ---- Telegram account linking ----

// CreateTelegramLinkCode issues a one-time code that, when redeemed in the bot,
// binds a Telegram account to this user. The code expires after ttl.
func (s *Store) CreateTelegramLinkCode(ctx context.Context, userID uuid.UUID, ttl time.Duration) (string, time.Time, error) {
	code, err := randomCode()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(ttl)
	if _, err := s.q.CreateTelegramLinkCode(ctx, sqlc.CreateTelegramLinkCodeParams{
		Code: code, UserID: userID, ExpiresAt: expires,
	}); err != nil {
		return "", time.Time{}, mapErr(err)
	}
	return code, expires, nil
}

// RedeemTelegramLinkCode validates a link code and binds tgID to the code's user
// in one transaction. Returns ErrNotFound for an unknown/expired/used code and
// ErrTelegramTaken if the Telegram id is already linked to another account.
func (s *Store) RedeemTelegramLinkCode(ctx context.Context, code string, tgID int64) (domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	lc, err := qtx.GetValidTelegramLinkCode(ctx, code)
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	u, err := qtx.LinkTelegramToUser(ctx, sqlc.LinkTelegramToUserParams{ID: lc.UserID, TelegramID: &tgID})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, ErrTelegramTaken
		}
		return domain.User{}, mapErr(err)
	}
	if err := qtx.MarkTelegramLinkCodeUsed(ctx, code); err != nil {
		return domain.User{}, mapErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
}

// randomCode returns a short URL-safe code (8 hex chars) for Telegram deep links.
func randomCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) ListStudentsForTeacher(ctx context.Context, teacherID uuid.UUID) ([]domain.User, error) {
	rows, err := s.q.ListStudentsForTeacher(ctx, teacherID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainUser(r))
	}
	return out, nil
}

// Credentials pairs a user with its stored password hash, for login only.
type Credentials struct {
	User         domain.User
	PasswordHash string
}

// GetCredentialsByUsername loads a user + password hash for login verification.
func (s *Store) GetCredentialsByUsername(ctx context.Context, username string) (Credentials, error) {
	u, err := s.q.GetUserByUsername(ctx, &username)
	if err != nil {
		return Credentials{}, mapErr(err)
	}
	hash := ""
	if u.PasswordHash != nil {
		hash = *u.PasswordHash
	}
	return Credentials{User: toDomainUser(u), PasswordHash: hash}, nil
}

// CreateUserWithCredentials creates a username/password account (admin panel /
// teacher-created student). subject scopes a teacher; nil = super-teacher or a
// non-teacher role. Returns ErrUsernameTaken on a username collision.
func (s *Store) CreateUserWithCredentials(ctx context.Context, role domain.Role, name, username, passwordHash string, subject *domain.SubjectCode) (domain.User, error) {
	u, err := s.q.CreateUserWithCredentials(ctx, sqlc.CreateUserWithCredentialsParams{
		Role:         string(role),
		Name:         name,
		Username:     &username,
		PasswordHash: &passwordHash,
		Subject:      subjectPtr(subject),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, ErrUsernameTaken
		}
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

// subjectPtr converts an optional domain subject to the sqlc string pointer.
func subjectPtr(subject *domain.SubjectCode) *string {
	if subject == nil {
		return nil
	}
	sc := string(*subject)
	return &sc
}

// ListStudents returns all students (stage-1: the teacher oversees all of them).
func (s *Store) ListStudents(ctx context.Context) ([]domain.User, error) {
	rows, err := s.q.ListStudents(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainUser(r))
	}
	return out, nil
}

// IdleStudent is a student who has gone quiet, for streak nudges.
type IdleStudent struct {
	ID         uuid.UUID
	TelegramID *int64
	Name       string
}

// IdleStudents returns students who answered before but not since `since`.
func (s *Store) IdleStudents(ctx context.Context, since time.Time) ([]IdleStudent, error) {
	rows, err := s.q.IdleStudents(ctx, since)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]IdleStudent, 0, len(rows))
	for _, r := range rows {
		out = append(out, IdleStudent{ID: r.ID, TelegramID: r.TelegramID, Name: r.Name})
	}
	return out, nil
}

// ---- Subjects ----

func (s *Store) ListSubjects(ctx context.Context) ([]domain.Subject, error) {
	rows, err := s.q.ListSubjects(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Subject, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainSubject(r))
	}
	return out, nil
}

func (s *Store) GetSubjectByCode(ctx context.Context, code domain.SubjectCode) (domain.Subject, error) {
	sub, err := s.q.GetSubjectByCode(ctx, string(code))
	if err != nil {
		return domain.Subject{}, mapErr(err)
	}
	return toDomainSubject(sub), nil
}

// ---- Tasks ----

func (s *Store) GetTask(ctx context.Context, id uuid.UUID) (domain.Task, error) {
	t, err := s.q.GetTask(ctx, id)
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	return toDomainTask(t)
}

// CreateTask inserts a task. ID/CreatedAt on the input are ignored (DB-assigned).
func (s *Store) CreateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	media, err := mustJSON(t.Media)
	if err != nil {
		return domain.Task{}, err
	}
	schema, err := mustJSON(t.AnswerSchema)
	if err != nil {
		return domain.Task{}, err
	}
	var source []byte
	if t.Source != nil {
		if source, err = mustJSON(t.Source); err != nil {
			return domain.Task{}, err
		}
	}
	status := t.Status
	if status == "" {
		status = domain.TaskDraft
	}
	row, err := s.q.CreateTask(ctx, sqlc.CreateTaskParams{
		SubjectID:    t.SubjectID,
		Number:       int32(t.Number),
		Statement:    t.Statement,
		Media:        media,
		AnswerSchema: schema,
		Source:       source,
		Status:       string(status),
	})
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	return toDomainTask(row)
}

// TaskFilter narrows ListTasks; zero fields mean "no filter".
type TaskFilter struct {
	SubjectID *uuid.UUID
	Number    *int
	Status    *domain.TaskStatus
	Limit     int
	Offset    int
}

func (s *Store) ListTasks(ctx context.Context, f TaskFilter) ([]domain.Task, error) {
	limit := int32(f.Limit)
	if limit <= 0 {
		limit = 50
	}
	var number *int32
	if f.Number != nil {
		n := int32(*f.Number)
		number = &n
	}
	var status *string
	if f.Status != nil {
		st := string(*f.Status)
		status = &st
	}
	rows, err := s.q.ListTasks(ctx, sqlc.ListTasksParams{
		Limit:     limit,
		Offset:    int32(f.Offset),
		SubjectID: f.SubjectID,
		Number:    number,
		Status:    status,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomainTasks(rows)
}

// PracticeTasks returns active tasks for a subject the student hasn't mastered
// (solved correctly < `mastered` times), in random order — so mastered tasks
// stop repeating in practice.
func (s *Store) PracticeTasks(ctx context.Context, studentID, subjectID uuid.UUID, mastered, limit int) ([]domain.Task, error) {
	if limit <= 0 {
		limit = 15
	}
	rows, err := s.q.PracticeTasks(ctx, sqlc.PracticeTasksParams{
		SubjectID: subjectID, StudentID: studentID, Mastered: int64(mastered), Lim: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomainTasks(rows)
}

func (s *Store) SetTaskStatus(ctx context.Context, id uuid.UUID, status domain.TaskStatus) (domain.Task, error) {
	row, err := s.q.SetTaskStatus(ctx, sqlc.SetTaskStatusParams{ID: id, Status: string(status)})
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	return toDomainTask(row)
}

func (s *Store) UpdateTaskAnswer(ctx context.Context, id uuid.UUID, schema domain.AnswerSchema) (domain.Task, error) {
	if err := schema.Validate(); err != nil {
		return domain.Task{}, err
	}
	blob, err := mustJSON(schema)
	if err != nil {
		return domain.Task{}, err
	}
	row, err := s.q.UpdateTaskAnswer(ctx, sqlc.UpdateTaskAnswerParams{ID: id, AnswerSchema: blob})
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	return toDomainTask(row)
}

// UpdateTaskContent refreshes a task's statement + media in place (re-fetch /
// upgrade), leaving its curated answer_schema and status untouched.
func (s *Store) UpdateTaskContent(ctx context.Context, id uuid.UUID, statement string, media []domain.Media) (domain.Task, error) {
	blob, err := mustJSON(media)
	if err != nil {
		return domain.Task{}, err
	}
	row, err := s.q.UpdateTaskContent(ctx, sqlc.UpdateTaskContentParams{ID: id, Statement: statement, Media: blob})
	if err != nil {
		return domain.Task{}, mapErr(err)
	}
	return toDomainTask(row)
}

// TaskExistsBySource supports ingest dedup.
func (s *Store) TaskExistsBySource(ctx context.Context, provider, externID string) (bool, error) {
	return s.q.TaskExistsBySource(ctx, sqlc.TaskExistsBySourceParams{
		Provider: provider, ExternID: externID,
	})
}

// ActivateDraftTaskBySource promotes a dedup-hit draft to active (drafts only —
// a task the teacher rejected stays rejected). Returns whether a row changed.
func (s *Store) ActivateDraftTaskBySource(ctx context.Context, provider, externID string) (bool, error) {
	n, err := s.q.ActivateDraftTaskBySource(ctx, sqlc.ActivateDraftTaskBySourceParams{
		Provider: provider, ExternID: externID,
	})
	if err != nil {
		return false, mapErr(err)
	}
	return n > 0, nil
}

// ClearBank wipes the task bank for a subject, keeping any task that carries
// student history (has a recorded answer) so attempts/stats never orphan. The
// kept tasks are first detached from any tests (test_items). Runs in one tx and
// returns how many were deleted and how many were kept as in-use.
func (s *Store) ClearBank(ctx context.Context, subjectID uuid.UUID) (deleted, kept int, err error) {
	total, err := s.q.CountTasksBySubject(ctx, subjectID)
	if err != nil {
		return 0, 0, mapErr(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	if err := qtx.DeleteTestItemsForUnansweredTasksBySubject(ctx, subjectID); err != nil {
		return 0, 0, mapErr(err)
	}
	n, err := qtx.DeleteUnansweredTasksBySubject(ctx, subjectID)
	if err != nil {
		return 0, 0, mapErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return int(n), int(total) - int(n), nil
}

// ---- Attempts & answers ----

func (s *Store) StartAttempt(ctx context.Context, studentID, testID uuid.UUID, assignmentID *uuid.UUID) (domain.Attempt, error) {
	a, err := s.q.StartAttempt(ctx, sqlc.StartAttemptParams{
		AssignmentID: assignmentID, TestID: testID, StudentID: studentID,
	})
	if err != nil {
		return domain.Attempt{}, mapErr(err)
	}
	return toDomainAttempt(a), nil
}

func (s *Store) GetAttempt(ctx context.Context, id uuid.UUID) (domain.Attempt, error) {
	a, err := s.q.GetAttempt(ctx, id)
	if err != nil {
		return domain.Attempt{}, mapErr(err)
	}
	return toDomainAttempt(a), nil
}

func (s *Store) FinishAttempt(ctx context.Context, id uuid.UUID) (domain.Attempt, error) {
	a, err := s.q.FinishAttempt(ctx, id)
	if err != nil {
		return domain.Attempt{}, mapErr(err)
	}
	return toDomainAttempt(a), nil
}

func (s *Store) RecordAnswer(ctx context.Context, attemptID, taskID uuid.UUID, raw string, correct bool, timeMS int64) (domain.Answer, error) {
	a, err := s.q.RecordAnswer(ctx, sqlc.RecordAnswerParams{
		AttemptID:   attemptID,
		TaskID:      taskID,
		RawAnswer:   raw,
		IsCorrect:   correct,
		TimeSpentMs: timeMS,
	})
	if err != nil {
		return domain.Answer{}, mapErr(err)
	}
	return toDomainAnswer(a), nil
}

func (s *Store) ListAnswersForAttempt(ctx context.Context, attemptID uuid.UUID) ([]domain.Answer, error) {
	rows, err := s.q.ListAnswersForAttempt(ctx, attemptID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Answer, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainAnswer(r))
	}
	return out, nil
}

// ---- Assignments ----

func (s *Store) CreateAssignment(ctx context.Context, testID, studentID, assignedBy uuid.UUID, at time.Time) (domain.Assignment, error) {
	a, err := s.q.CreateAssignment(ctx, sqlc.CreateAssignmentParams{
		TestID: testID, StudentID: studentID, AssignedBy: assignedBy, ScheduledAt: at,
	})
	if err != nil {
		return domain.Assignment{}, mapErr(err)
	}
	return toDomainAssignment(a), nil
}

func (s *Store) GetAssignment(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	a, err := s.q.GetAssignment(ctx, id)
	if err != nil {
		return domain.Assignment{}, mapErr(err)
	}
	return toDomainAssignment(a), nil
}

func (s *Store) ListAssignmentsForStudent(ctx context.Context, studentID uuid.UUID) ([]domain.Assignment, error) {
	rows, err := s.q.ListAssignmentsForStudent(ctx, studentID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Assignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainAssignment(r))
	}
	return out, nil
}

// ListAssignmentCards returns a student's assignments with test info + count.
func (s *Store) ListAssignmentCards(ctx context.Context, studentID uuid.UUID) ([]domain.AssignmentCard, error) {
	rows, err := s.q.ListAssignmentsWithTestForStudent(ctx, studentID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.AssignmentCard, 0, len(rows))
	for _, r := range rows {
		card := domain.AssignmentCard{
			ID:          r.ID,
			TestID:      r.TestID,
			Title:       r.Title,
			Kind:        domain.TestKind(r.Kind),
			SubjectID:   r.SubjectID,
			ScheduledAt: r.ScheduledAt,
			NotifiedAt:  r.NotifiedAt,
			Status:      domain.AssignmentStatus(r.Status),
			TaskCount:   r.TaskCount,
		}
		// A finished attempt exists iff attempt_finished_at is set; only then is
		// the (COALESCE'd) attempt id / score meaningful.
		if r.AttemptFinishedAt != nil {
			id := r.AttemptID
			card.AttemptID = &id
			card.FinishedAt = r.AttemptFinishedAt
			card.Correct = r.Correct
			card.Total = r.Total
		}
		out = append(out, card)
	}
	return out, nil
}

// ListAttemptSummaries returns a student's recent attempts with scores.
func (s *Store) ListAttemptSummaries(ctx context.Context, studentID uuid.UUID, limit int) ([]domain.AttemptSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.q.ListAttemptsForStudent(ctx, sqlc.ListAttemptsForStudentParams{
		StudentID: studentID, Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.AttemptSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.AttemptSummary{
			ID:         r.ID,
			TestID:     r.TestID,
			Title:      r.Title,
			Kind:       domain.TestKind(r.Kind),
			SubjectID:  r.SubjectID,
			StartedAt:  r.StartedAt,
			FinishedAt: r.FinishedAt,
			Total:      r.Total,
			Correct:    r.Correct,
			TimeMS:     r.TimeMs,
		})
	}
	return out, nil
}

func (s *Store) MarkAssignmentNotified(ctx context.Context, id uuid.UUID) (domain.Assignment, error) {
	a, err := s.q.MarkAssignmentNotified(ctx, id)
	if err != nil {
		return domain.Assignment{}, mapErr(err)
	}
	return toDomainAssignment(a), nil
}

// SetAssignmentStatus moves an assignment through its lifecycle
// (scheduled → done/missed).
func (s *Store) SetAssignmentStatus(ctx context.Context, id uuid.UUID, status domain.AssignmentStatus) (domain.Assignment, error) {
	a, err := s.q.SetAssignmentStatus(ctx, sqlc.SetAssignmentStatusParams{
		ID: id, Status: string(status),
	})
	if err != nil {
		return domain.Assignment{}, mapErr(err)
	}
	return toDomainAssignment(a), nil
}

// ListDueAssignments returns assignments due by t that were never notified.
func (s *Store) ListDueAssignments(ctx context.Context, t time.Time) ([]domain.Assignment, error) {
	rows, err := s.q.ListDueAssignments(ctx, t)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Assignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainAssignment(r))
	}
	return out, nil
}

func toDomainAssignment(a sqlc.Assignment) domain.Assignment {
	return domain.Assignment{
		ID:          a.ID,
		TestID:      a.TestID,
		StudentID:   a.StudentID,
		AssignedBy:  a.AssignedBy,
		ScheduledAt: a.ScheduledAt,
		NotifiedAt:  a.NotifiedAt,
		Status:      domain.AssignmentStatus(a.Status),
	}
}
