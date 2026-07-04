package store

import (
	"context"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// ---- Classes (a teacher's groups of students) ----

func (s *Store) CreateClass(ctx context.Context, teacherID uuid.UUID, name string) (domain.Class, error) {
	c, err := s.q.CreateClass(ctx, sqlc.CreateClassParams{TeacherID: teacherID, Name: name})
	if err != nil {
		return domain.Class{}, mapErr(err)
	}
	return toDomainClass(c), nil
}

func (s *Store) GetClass(ctx context.Context, id uuid.UUID) (domain.Class, error) {
	c, err := s.q.GetClass(ctx, id)
	if err != nil {
		return domain.Class{}, mapErr(err)
	}
	return toDomainClass(c), nil
}

func (s *Store) RenameClass(ctx context.Context, id uuid.UUID, name string) (domain.Class, error) {
	c, err := s.q.RenameClass(ctx, sqlc.RenameClassParams{ID: id, Name: name})
	if err != nil {
		return domain.Class{}, mapErr(err)
	}
	return toDomainClass(c), nil
}

// DeleteClass removes a class and its memberships; the students stay.
func (s *Store) DeleteClass(ctx context.Context, id uuid.UUID) error {
	n, err := s.q.DeleteClass(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListClassesForTeacher returns a teacher's classes with member counts.
func (s *Store) ListClassesForTeacher(ctx context.Context, teacherID uuid.UUID) ([]domain.Class, error) {
	rows, err := s.q.ListClassesForTeacher(ctx, teacherID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Class, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Class{
			ID: r.ID, TeacherID: r.TeacherID, Name: r.Name,
			CreatedAt: r.CreatedAt, MemberCount: r.MemberCount,
		})
	}
	return out, nil
}

// ListAllClasses returns every class with its teacher's name (admin view).
func (s *Store) ListAllClasses(ctx context.Context) ([]domain.Class, error) {
	rows, err := s.q.ListAllClasses(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Class, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Class{
			ID: r.ID, TeacherID: r.TeacherID, Name: r.Name, CreatedAt: r.CreatedAt,
			MemberCount: r.MemberCount, TeacherName: r.TeacherName,
		})
	}
	return out, nil
}

// AddClassMember puts a student into a class AND creates the teacher↔student
// enrollment in one transaction — membership implies the teacher may see the
// student's stats and assign to them. Idempotent on both rows.
func (s *Store) AddClassMember(ctx context.Context, classID, teacherID, studentID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	if err := qtx.AddClassMember(ctx, sqlc.AddClassMemberParams{ClassID: classID, StudentID: studentID}); err != nil {
		return mapErr(err)
	}
	if _, err := qtx.CreateEnrollment(ctx, sqlc.CreateEnrollmentParams{TeacherID: teacherID, StudentID: studentID}); err != nil {
		return mapErr(err)
	}
	return tx.Commit(ctx)
}

// RemoveClassMember unlinks a student from a class. The enrollment stays: the
// student remains "мой ученик без класса" until the teacher drops them fully.
func (s *Store) RemoveClassMember(ctx context.Context, classID, studentID uuid.UUID) error {
	n, err := s.q.RemoveClassMember(ctx, sqlc.RemoveClassMemberParams{ClassID: classID, StudentID: studentID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListClassMembers(ctx context.Context, classID uuid.UUID) ([]domain.User, error) {
	rows, err := s.q.ListClassMembers(ctx, classID)
	if err != nil {
		return nil, mapErr(err)
	}
	return toDomainUsers(rows), nil
}

// ListClassesForStudent returns the classes a student belongs to, with teacher
// names — the student profile's "мои классы".
func (s *Store) ListClassesForStudent(ctx context.Context, studentID uuid.UUID) ([]domain.Class, error) {
	rows, err := s.q.ListClassesForStudent(ctx, studentID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Class, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Class{
			ID: r.ID, TeacherID: r.TeacherID, Name: r.Name,
			CreatedAt: r.CreatedAt, TeacherName: r.TeacherName,
		})
	}
	return out, nil
}

// ClassMembership tags one student with one class they belong to.
type ClassMembership struct {
	StudentID uuid.UUID
	ClassID   uuid.UUID
	ClassName string
}

// ListClassMembershipsForTeacher returns all (student, class) pairs across one
// teacher's classes, to tag the roster with class names in one query.
func (s *Store) ListClassMembershipsForTeacher(ctx context.Context, teacherID uuid.UUID) ([]ClassMembership, error) {
	rows, err := s.q.ListClassMembershipsForTeacher(ctx, teacherID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]ClassMembership, 0, len(rows))
	for _, r := range rows {
		out = append(out, ClassMembership{StudentID: r.StudentID, ClassID: r.ClassID, ClassName: r.ClassName})
	}
	return out, nil
}

// ClassMastery returns the class overview grid for one subject: every member's
// per-number success, merged with the member list so students with no answers
// still show as empty rows.
func (s *Store) ClassMastery(ctx context.Context, classID, subjectID uuid.UUID) ([]domain.ClassStudentStats, error) {
	members, err := s.ListClassMembers(ctx, classID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ClassMastery(ctx, sqlc.ClassMasteryParams{ClassID: classID, SubjectID: subjectID})
	if err != nil {
		return nil, mapErr(err)
	}
	byStudent := make(map[uuid.UUID]*domain.ClassStudentStats, len(members))
	out := make([]domain.ClassStudentStats, 0, len(members))
	for _, m := range members {
		out = append(out, domain.ClassStudentStats{StudentID: m.ID, Name: m.Name, ByNumber: []domain.ClassNumberStat{}})
	}
	for i := range out {
		byStudent[out[i].StudentID] = &out[i]
	}
	for _, r := range rows {
		st, ok := byStudent[r.StudentID]
		if !ok {
			continue // a just-removed member's history; not a row anymore
		}
		st.Total += r.Total
		st.Correct += r.Correct
		st.ByNumber = append(st.ByNumber, domain.ClassNumberStat{
			Number: int(r.Number), Total: r.Total, Correct: r.Correct,
		})
	}
	return out, nil
}

func toDomainClass(c sqlc.Class) domain.Class {
	return domain.Class{ID: c.ID, TeacherID: c.TeacherID, Name: c.Name, CreatedAt: c.CreatedAt}
}
