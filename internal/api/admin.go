package api

import (
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	var req createTaskReq
	if !decodeJSON(w, r, &req) {
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id")
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id")
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	code := r.URL.Query().Get("subject")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "subject is required")
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
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
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
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
	test, err := s.store.RenameTest(r.Context(), id, title)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, test)
}

type addItemReq struct {
	TaskID   uuid.UUID `json:"task_id"`
	Position int       `json:"position"`
}

func (s *Server) handleAddTestItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	testID, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	var req addItemReq
	if !decodeJSON(w, r, &req) {
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
	TestID      uuid.UUID `json:"test_id"`
	StudentID   uuid.UUID `json:"student_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	// Notify controls the Telegram notification (design §4.4 toggle). Absent =
	// true, so older clients keep the previous always-notify behavior.
	Notify *bool `json:"notify,omitempty"`
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
	assignment, err := s.store.CreateAssignment(r.Context(), req.TestID, req.StudentID, teacher.ID, req.ScheduledAt)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if req.Notify != nil && !*req.Notify {
		// Teacher opted out: pre-stamp notified_at so neither the enqueued task
		// nor the worker's due-assignment sweep ever messages the student.
		if a, err := s.store.MarkAssignmentNotified(r.Context(), assignment.ID); err == nil {
			assignment = a
		} else {
			slog.Warn("mark assignment as opted-out failed", "assignment", assignment.ID, "err", err)
		}
		writeJSON(w, http.StatusCreated, assignment)
		return
	}
	// Enqueue the Telegram notification for the scheduled time. If the queue is
	// unavailable (or no worker configured) the worker's periodic sweep of due,
	// un-notified assignments is the safety net — so we don't fail the request.
	if s.scheduler != nil {
		if err := s.scheduler.ScheduleAssignmentNotification(r.Context(), assignment.ID, assignment.ScheduledAt); err != nil {
			slog.Warn("enqueue assignment notification failed; sweep will retry", "assignment", assignment.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusCreated, assignment)
}
