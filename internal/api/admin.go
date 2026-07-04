package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
)

// requireTeacher returns the acting user if they are a teacher, else 403.
func (s *Server) requireTeacher(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	user, _ := userFrom(r.Context())
	if user.Role != domain.RoleTeacher {
		writeErr(w, http.StatusForbidden, "teacher role required")
		return domain.User{}, false
	}
	return user, true
}

// requireAdmin returns the acting user if they are an admin, else 403.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	user, _ := userFrom(r.Context())
	if user.Role != domain.RoleAdmin {
		writeErr(w, http.StatusForbidden, "admin role required")
		return domain.User{}, false
	}
	return user, true
}

// subjectInScope enforces a teacher's subject scope: a subject-bound teacher
// (user.Subject set) may only touch that subject; a super-teacher (nil) may
// touch any. Writes the 403 itself.
func (s *Server) subjectInScope(w http.ResponseWriter, user domain.User, code domain.SubjectCode) bool {
	if user.Subject == nil || *user.Subject == code {
		return true
	}
	writeErr(w, http.StatusForbidden, "предмет вне вашей роли: вы ведёте только «"+string(*user.Subject)+"»")
	return false
}

// testInScope loads a test and enforces the teacher's subject scope on it, for
// endpoints addressed by test id (detail, rename, delete, export, items,
// assignment). Returns the test on success.
func (s *Server) testInScope(w http.ResponseWriter, r *http.Request, user domain.User, testID uuid.UUID) (domain.Test, bool) {
	test, err := s.store.GetTest(r.Context(), testID)
	if err != nil {
		writeStoreErr(w, err)
		return domain.Test{}, false
	}
	if user.Subject != nil {
		sub, err := s.store.GetSubjectByCode(r.Context(), *user.Subject)
		if err != nil {
			writeStoreErr(w, err)
			return domain.Test{}, false
		}
		if test.SubjectID != sub.ID {
			writeErr(w, http.StatusForbidden, "тест по чужому предмету")
			return domain.Test{}, false
		}
	}
	return test, true
}

// testOwned guards MUTATING test operations (delete/rename/items): with many
// teachers sharing a subject's test list, only the creator may change or
// destroy a test — reading, exporting and assigning stay shared.
func (s *Server) testOwned(w http.ResponseWriter, teacher domain.User, test domain.Test) bool {
	if test.CreatedBy == teacher.ID {
		return true
	}
	writeErr(w, http.StatusForbidden, "тест создан другим учителем")
	return false
}

// taskInScope enforces the teacher's subject scope on a task id (bank edits).
func (s *Server) taskInScope(w http.ResponseWriter, r *http.Request, user domain.User, taskID uuid.UUID) bool {
	if user.Subject == nil {
		return true
	}
	task, err := s.store.GetTask(r.Context(), taskID)
	if err != nil {
		writeStoreErr(w, err)
		return false
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), *user.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return false
	}
	if task.SubjectID != sub.ID {
		writeErr(w, http.StatusForbidden, "задание по чужому предмету")
		return false
	}
	return true
}

// classOwned loads a class and checks it belongs to the acting teacher.
func (s *Server) classOwned(w http.ResponseWriter, r *http.Request, user domain.User, classID uuid.UUID) (domain.Class, bool) {
	class, err := s.store.GetClass(r.Context(), classID)
	if err != nil {
		writeStoreErr(w, err)
		return domain.Class{}, false
	}
	if class.TeacherID != user.ID {
		writeErr(w, http.StatusForbidden, "это не ваш класс")
		return domain.Class{}, false
	}
	return class, true
}

// studentOfTeacher checks the enrollment link (teacher may act on the student).
func (s *Server) studentOfTeacher(w http.ResponseWriter, r *http.Request, teacher domain.User, studentID uuid.UUID) bool {
	ok, err := s.store.IsTeacherOfStudent(r.Context(), teacher.ID, studentID)
	if err != nil {
		writeStoreErr(w, err)
		return false
	}
	if !ok {
		writeErr(w, http.StatusForbidden, "это не ваш ученик")
		return false
	}
	return true
}

type createTaskReq struct {
	Subject      domain.SubjectCode  `json:"subject"`
	Number       int                 `json:"number"`
	Statement    string              `json:"statement"`
	Media        []domain.Media      `json:"media,omitempty"`
	AnswerSchema domain.AnswerSchema `json:"answer_schema"`
	Source       *domain.Source      `json:"source,omitempty"`
	Status       domain.TaskStatus   `json:"status,omitempty"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req createTaskReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.subjectInScope(w, teacher, req.Subject) {
		return
	}
	if err := req.AnswerSchema.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	status := req.Status
	if status == "" {
		status = domain.TaskActive // manual authoring is trusted, unlike ingest
	}
	task, err := s.store.CreateTask(r.Context(), domain.Task{
		SubjectID:    sub.ID,
		Number:       req.Number,
		Statement:    req.Statement,
		Media:        req.Media,
		AnswerSchema: req.AnswerSchema,
		Source:       req.Source,
		Status:       status,
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

type updateAnswerReq struct {
	AnswerSchema domain.AnswerSchema `json:"answer_schema"`
}

func (s *Server) handleUpdateTaskAnswer(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if !s.taskInScope(w, r, teacher, id) {
		return
	}
	var req updateAnswerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	task, err := s.store.UpdateTaskAnswer(r.Context(), id, req.AnswerSchema)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

type setStatusReq struct {
	Status domain.TaskStatus `json:"status"`
}

func (s *Server) handleSetTaskStatus(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if !s.taskInScope(w, r, teacher, id) {
		return
	}
	var req setStatusReq
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Status {
	case domain.TaskDraft, domain.TaskActive, domain.TaskRejected:
	default:
		writeErr(w, http.StatusBadRequest, "invalid status")
		return
	}
	task, err := s.store.SetTaskStatus(r.Context(), id, req.Status)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

type clearBankResp struct {
	Deleted int `json:"deleted"`
	Kept    int `json:"kept"` // tasks preserved because they carry answer history
}

// handleClearBank wipes the whole bank for one subject (?subject=math), keeping
// only tasks that have been answered so student history/stats never orphan.
func (s *Server) handleClearBank(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	code := r.URL.Query().Get("subject")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "subject is required")
		return
	}
	if !s.subjectInScope(w, teacher, domain.SubjectCode(code)) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	deleted, kept, err := s.store.ClearBank(r.Context(), sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clearBankResp{Deleted: deleted, Kept: kept})
}

type createTestReq struct {
	Subject domain.SubjectCode `json:"subject"`
	Kind    domain.TestKind    `json:"kind"`
	Title   string             `json:"title"`
}

func (s *Server) handleCreateTest(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req createTestReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Kind != domain.TestClassic && req.Kind != domain.TestDrill {
		writeErr(w, http.StatusBadRequest, "kind must be classic or drill")
		return
	}
	if !s.subjectInScope(w, teacher, req.Subject) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	test, err := s.store.CreateTest(r.Context(), sub.ID, req.Kind, req.Title, teacher.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, test)
}

// handleDeleteTest removes a teacher's test (and its items/assignments). Tests
// that have been attempted are protected (409) so student history is kept.
func (s *Server) handleDeleteTest(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	test, ok := s.testInScope(w, r, teacher, id)
	if !ok || !s.testOwned(w, teacher, test) {
		return
	}
	if err := s.store.DeleteTest(r.Context(), id); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

type renameTestReq struct {
	Title string `json:"title"`
}

// handleRenameTest lets a teacher rename a test/variant so distinct variants are
// easy to tell apart in the list.
func (s *Server) handleRenameTest(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	test, ok := s.testInScope(w, r, teacher, id)
	if !ok || !s.testOwned(w, teacher, test) {
		return
	}
	var req renameTestReq
	if !decodeJSON(w, r, &req) {
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	renamed, err := s.store.RenameTest(r.Context(), id, title)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, renamed)
}

type addItemReq struct {
	TaskID   uuid.UUID `json:"task_id"`
	Position int       `json:"position"`
}

func (s *Server) handleAddTestItem(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	testID, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	test, ok := s.testInScope(w, r, teacher, testID)
	if !ok || !s.testOwned(w, teacher, test) {
		return
	}
	var req addItemReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// The task must belong to the test's subject — otherwise a subject-scoped
	// teacher could smuggle a foreign-subject task into their test and read its
	// answer key through the test detail/export.
	task, err := s.store.GetTask(r.Context(), req.TaskID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if task.SubjectID != test.SubjectID {
		writeErr(w, http.StatusBadRequest, "задание другого предмета")
		return
	}
	item, err := s.store.AddTestItem(r.Context(), testID, req.TaskID, req.Position)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type createAssignmentReq struct {
	TestID uuid.UUID `json:"test_id"`
	// Exactly one target: a single student (must be the teacher's) or one of
	// the teacher's classes — the class fans out to every member.
	StudentID   *uuid.UUID `json:"student_id,omitempty"`
	ClassID     *uuid.UUID `json:"class_id,omitempty"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	// Notify controls the Telegram notification (design §4.4 toggle). Absent =
	// true, so older clients keep the previous always-notify behavior.
	Notify *bool `json:"notify,omitempty"`
	// Individual gives every target student their OWN generated variant — the
	// picked test's number structure with randomly drawn bank tasks — so a
	// class can't copy answers off each other. The picked test acts as the
	// template; students are assigned personal clones (tests.variant_of).
	Individual bool `json:"individual,omitempty"`
}

type createAssignmentResp struct {
	Created     int                 `json:"created"`
	Assignments []domain.Assignment `json:"assignments"`
}

func (s *Server) handleCreateAssignment(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req createAssignmentReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if (req.StudentID == nil) == (req.ClassID == nil) {
		writeErr(w, http.StatusBadRequest, "укажи ровно одну цель: student_id или class_id")
		return
	}
	test, ok := s.testInScope(w, r, teacher, req.TestID)
	if !ok {
		return
	}
	// Resolve the target students: one enrolled student, or a class's members.
	var students []domain.User
	if req.StudentID != nil {
		if !s.studentOfTeacher(w, r, teacher, *req.StudentID) {
			return
		}
		student, err := s.store.GetUser(r.Context(), *req.StudentID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		students = []domain.User{student}
	} else {
		if _, ok := s.classOwned(w, r, teacher, *req.ClassID); !ok {
			return
		}
		members, err := s.store.ListClassMembers(r.Context(), *req.ClassID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		for _, m := range members {
			if !m.IsActive {
				continue // deactivated accounts must not receive work or pings
			}
			students = append(students, m)
		}
		if len(students) == 0 {
			writeErr(w, http.StatusUnprocessableEntity, "в классе нет активных учеников")
			return
		}
	}
	resp := createAssignmentResp{Assignments: make([]domain.Assignment, 0, len(students))}
	for _, student := range students {
		// Individual mode: clone the template into a personal random variant.
		// If generation fails the student still gets the shared test — an
		// assignment must never be lost to a thin bank or a hiccup.
		testID := req.TestID
		if req.Individual {
			gv, err := s.store.GenerateVariantLike(r.Context(), test, teacher.ID, test.Title+" · "+student.Name)
			if err != nil {
				slog.Warn("generate individual variant failed; assigning the shared test", "student", student.ID, "err", err)
			} else {
				testID = gv.Test.ID
			}
		}
		assignment, err := s.createAssignmentFor(r.Context(), testID, student.ID, teacher.ID, req.ScheduledAt, req.Notify)
		if err != nil {
			// Partial fan-out: log and keep going; report what was created.
			slog.Warn("create assignment failed mid-fanout", "student", student.ID, "err", err)
			continue
		}
		resp.Assignments = append(resp.Assignments, assignment)
	}
	resp.Created = len(resp.Assignments)
	if resp.Created == 0 {
		writeErr(w, http.StatusInternalServerError, "не удалось создать назначение")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// createAssignmentFor creates one assignment plus its in-app notification and
// Telegram scheduling — the single-student unit the class fan-out loops over.
func (s *Server) createAssignmentFor(ctx context.Context, testID, studentID, teacherID uuid.UUID, at time.Time, notify *bool) (domain.Assignment, error) {
	assignment, err := s.store.CreateAssignment(ctx, testID, studentID, teacherID, at)
	if err != nil {
		return domain.Assignment{}, err
	}
	// The in-app bell always learns about the new assignment (the notify toggle
	// governs Telegram only). Best-effort: the assignment itself already exists.
	if err := s.store.CreateNotification(ctx, studentID, domain.NotificationAssignmentCreated, assignment.ID); err != nil {
		slog.Warn("create assignment notification failed", "assignment", assignment.ID, "err", err)
	}
	if notify != nil && !*notify {
		// Teacher opted out: pre-stamp notified_at so neither the enqueued task
		// nor the worker's due-assignment sweep ever messages the student.
		if a, err := s.store.MarkAssignmentNotified(ctx, assignment.ID); err == nil {
			assignment = a
		} else {
			slog.Warn("mark assignment as opted-out failed", "assignment", assignment.ID, "err", err)
		}
		return assignment, nil
	}
	// Enqueue the Telegram notification for the scheduled time. If the queue is
	// unavailable (or no worker configured) the worker's periodic sweep of due,
	// un-notified assignments is the safety net — so we don't fail the request.
	if s.scheduler != nil {
		if err := s.scheduler.ScheduleAssignmentNotification(ctx, assignment.ID, assignment.ScheduledAt); err != nil {
			slog.Warn("enqueue assignment notification failed; sweep will retry", "assignment", assignment.ID, "err", err)
		}
	}
	return assignment, nil
}
