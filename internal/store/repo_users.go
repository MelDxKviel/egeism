package store

import (
	"context"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// ---- User management (admin panel) ----

// ListUsers returns every account for the admin panel, admins/teachers first.
func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.q.ListUsers(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomainUsers(rows), nil
}

// SetUserActive toggles an account: a deactivated user keeps their history but
// can't log in or act until re-enabled.
func (s *Store) SetUserActive(ctx context.Context, id uuid.UUID, active bool) (domain.User, error) {
	u, err := s.q.SetUserActive(ctx, sqlc.SetUserActiveParams{ID: id, IsActive: active})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

// SetUserName renames an account.
func (s *Store) SetUserName(ctx context.Context, id uuid.UUID, name string) (domain.User, error) {
	u, err := s.q.SetUserName(ctx, sqlc.SetUserNameParams{ID: id, Name: name})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

// SetUserRoleSubject changes an account's role and teacher-subject scope in one
// step (the two go together: only teachers carry a subject).
func (s *Store) SetUserRoleSubject(ctx context.Context, id uuid.UUID, role domain.Role, subject *domain.SubjectCode) (domain.User, error) {
	u, err := s.q.SetUserRoleSubject(ctx, sqlc.SetUserRoleSubjectParams{
		ID: id, Role: string(role), Subject: subjectPtr(subject),
	})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toDomainUser(u), nil
}

// SetUserPassword replaces an account's password hash (admin reset).
func (s *Store) SetUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	n, err := s.q.SetUserPassword(ctx, sqlc.SetUserPasswordParams{ID: id, PasswordHash: &passwordHash})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes an account outright. Accounts that still carry protected
// history (attempts, created tests, assignments…) are refused with ErrInUse —
// deactivate those instead so no student data is ever lost by accident.
func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	n, err := s.q.DeleteUser(ctx, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrInUse
		}
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveAdmins reports how many enabled admin accounts exist (bootstrap +
// the "can't demote the last admin" guard).
func (s *Store) CountActiveAdmins(ctx context.Context) (int64, error) {
	return s.q.CountActiveAdmins(ctx)
}

// IsTeacherOfStudent reports whether the teacher↔student enrollment link
// exists — the check all per-student teacher authorization runs on.
func (s *Store) IsTeacherOfStudent(ctx context.Context, teacherID, studentID uuid.UUID) (bool, error) {
	ok, err := s.q.IsTeacherOfStudent(ctx, sqlc.IsTeacherOfStudentParams{TeacherID: teacherID, StudentID: studentID})
	if err != nil {
		return false, mapErr(err)
	}
	return ok, nil
}

// CreateEnrollment links a teacher to a student (idempotent upsert).
func (s *Store) CreateEnrollment(ctx context.Context, teacherID, studentID uuid.UUID) error {
	_, err := s.q.CreateEnrollment(ctx, sqlc.CreateEnrollmentParams{TeacherID: teacherID, StudentID: studentID})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// PlatformStats assembles the admin dashboard: platform-wide counters plus the
// per-subject activity breakdown.
func (s *Store) PlatformStats(ctx context.Context) (domain.PlatformStats, error) {
	row, err := s.q.PlatformStats(ctx)
	if err != nil {
		return domain.PlatformStats{}, mapErr(err)
	}
	subj, err := s.q.SubjectActivityStats(ctx)
	if err != nil {
		return domain.PlatformStats{}, mapErr(err)
	}
	out := domain.PlatformStats{
		Students:       row.Students,
		Teachers:       row.Teachers,
		Admins:         row.Admins,
		InactiveUsers:  row.InactiveUsers,
		Classes:        row.Classes,
		Tasks:          row.Tasks,
		ActiveTasks:    row.ActiveTasks,
		Tests:          row.Tests,
		Assignments:    row.Assignments,
		Attempts:       row.Attempts,
		Answers:        row.Answers,
		CorrectAnswers: row.CorrectAnswers,
		Answers7d:      row.Answers7d,
		Subjects:       make([]domain.SubjectActivity, 0, len(subj)),
	}
	for _, r := range subj {
		out.Subjects = append(out.Subjects, domain.SubjectActivity{
			Code:        domain.SubjectCode(r.Code),
			ActiveTasks: r.ActiveTasks,
			Answers:     r.Answers,
			Correct:     r.Correct,
		})
	}
	return out, nil
}
