package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
)

// A student may be enrolled to several teachers at once (school teacher +
// репетитор, different subjects) — enrollments is m2m. These handlers manage
// the link itself: a teacher takes an EXISTING student onto their roster
// (before, the only paths were creating the account or adding to a class) and
// drops a student from it. Creating a student (POST /api/students) lives in
// telegram_auth.go next to the account machinery.

// studentParam parses {studentID} and loads the user, enforcing they are a
// student. The acting user must be a teacher.
func (s *Server) studentParam(w http.ResponseWriter, r *http.Request) (teacher, student domain.User, ok bool) {
	teacher, ok = s.requireTeacher(w, r)
	if !ok {
		return domain.User{}, domain.User{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "studentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid student id")
		return domain.User{}, domain.User{}, false
	}
	student, err = s.store.GetUser(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return domain.User{}, domain.User{}, false
	}
	if student.Role != domain.RoleStudent {
		writeErr(w, http.StatusBadRequest, "это не ученик")
		return domain.User{}, domain.User{}, false
	}
	return teacher, student, true
}

// handleEnrollStudent puts an existing student onto the acting teacher's
// roster ("взять ученика", the репетитор case — no class involved). Idempotent:
// re-enrolling an already-enrolled student is a no-op success.
func (s *Server) handleEnrollStudent(w http.ResponseWriter, r *http.Request) {
	teacher, student, ok := s.studentParam(w, r)
	if !ok {
		return
	}
	if err := s.store.CreateEnrollment(r.Context(), teacher.ID, student.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, student)
}

// handleUnenrollStudent drops the student from the acting teacher's roster and
// from that teacher's classes ("отчислить"). The account, solve history and
// other teachers' enrollments stay untouched.
func (s *Server) handleUnenrollStudent(w http.ResponseWriter, r *http.Request) {
	teacher, student, ok := s.studentParam(w, r)
	if !ok {
		return
	}
	if err := s.store.UnenrollStudent(r.Context(), teacher.ID, student.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
