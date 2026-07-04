package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
)

// Classes are teacher-owned groups of students (переработка №2/№5). Every
// mutating endpoint checks ownership via classOwned; adding a member also
// enrolls the student to the teacher (store.AddClassMember, one tx), which is
// what stats access and assignment targeting run on.

// handleListClasses returns the acting teacher's classes with member counts.
func (s *Server) handleListClasses(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	classes, err := s.store.ListClassesForTeacher(r.Context(), teacher.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, classes)
}

type classNameReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateClass(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req classNameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "укажи название класса")
		return
	}
	class, err := s.store.CreateClass(r.Context(), teacher.ID, name)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, class)
}

// classDetail is the class page payload: the class plus its members.
type classDetail struct {
	Class    domain.Class  `json:"class"`
	Students []domain.User `json:"students"`
}

func (s *Server) handleGetClass(w http.ResponseWriter, r *http.Request) {
	_, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	class, err := s.store.GetClass(r.Context(), classID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	members, err := s.store.ListClassMembers(r.Context(), classID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, classDetail{Class: class, Students: members})
}

func (s *Server) handleRenameClass(w http.ResponseWriter, r *http.Request) {
	_, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	var req classNameReq
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "укажи название класса")
		return
	}
	class, err := s.store.RenameClass(r.Context(), classID, name)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, class)
}

// handleDeleteClass removes the class and its memberships; students (and their
// enrollments to the teacher) stay.
func (s *Server) handleDeleteClass(w http.ResponseWriter, r *http.Request) {
	_, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteClass(r.Context(), classID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type addMemberReq struct {
	StudentID uuid.UUID `json:"student_id"`
}

// handleAddClassMember puts a student into the class and enrolls them to the
// teacher (one tx) — from then on the teacher sees their stats and may assign.
func (s *Server) handleAddClassMember(w http.ResponseWriter, r *http.Request) {
	teacher, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	var req addMemberReq
	if !decodeJSON(w, r, &req) {
		return
	}
	student, err := s.store.GetUser(r.Context(), req.StudentID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if student.Role != domain.RoleStudent {
		writeErr(w, http.StatusBadRequest, "в класс можно добавить только ученика")
		return
	}
	if err := s.store.AddClassMember(r.Context(), classID, teacher.ID, student.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, student)
}

// handleRemoveClassMember unlinks a student from the class. The enrollment
// stays, so the student remains visible as "ученик без класса".
func (s *Server) handleRemoveClassMember(w http.ResponseWriter, r *http.Request) {
	_, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	studentID, err := uuid.Parse(chi.URLParam(r, "studentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid student id")
		return
	}
	if err := s.store.RemoveClassMember(r.Context(), classID, studentID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// handleClassOverview returns the color grid the teacher scans for trouble:
// per-member per-number accuracy for one subject, plus overall totals — which
// students lag, and which task numbers the whole class struggles with.
func (s *Server) handleClassOverview(w http.ResponseWriter, r *http.Request) {
	_, classID, ok := s.ownClassParam(w, r)
	if !ok {
		return
	}
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	rows, err := s.store.ClassMastery(r.Context(), classID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// ownClassParam parses {classID} and enforces the acting user is a teacher who
// owns that class.
func (s *Server) ownClassParam(w http.ResponseWriter, r *http.Request) (domain.User, uuid.UUID, bool) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return domain.User{}, uuid.Nil, false
	}
	classID, err := uuid.Parse(chi.URLParam(r, "classID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid class id")
		return domain.User{}, uuid.Nil, false
	}
	if _, ok := s.classOwned(w, r, teacher, classID); !ok {
		return domain.User{}, uuid.Nil, false
	}
	return teacher, classID, true
}
