package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// taskView is the student-facing task shape. It deliberately omits
// answer_schema.correct so the client never sees the right answer — checking is
// server-side. answer_kind + bot_solvable let the UI pick the right input
// widget (§3.3, §8).
type taskView struct {
	Part        int               `json:"part"`
	GradingMode string            `json:"grading_mode"`
	MaxPoints   int               `json:"max_points"`
	ID          uuid.UUID         `json:"id"`
	SubjectID   uuid.UUID         `json:"subject_id"`
	Number      int               `json:"number"`
	Statement   string            `json:"statement"`
	Media       []domain.Media    `json:"media"`
	Status      domain.TaskStatus `json:"status"`
	AnswerKind  domain.AnswerType `json:"answer_kind"`
	BotSolvable bool              `json:"bot_solvable"`
}

func toTaskView(t domain.Task) taskView {
	media := t.Media
	if media == nil {
		media = []domain.Media{}
	}
	return taskView{
		Part: t.Part, GradingMode: t.GradingMode, MaxPoints: t.MaxPoints,
		ID:          t.ID,
		SubjectID:   t.SubjectID,
		Number:      t.Number,
		Statement:   t.Statement,
		Media:       media,
		Status:      t.Status,
		AnswerKind:  t.AnswerSchema.Type,
		BotSolvable: t.BotSolvable(),
	}
}

func (s *Server) handleListSubjects(w http.ResponseWriter, r *http.Request) {
	subs, err := s.store.ListSubjects(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	active := domain.TaskActive
	f := store.TaskFilter{Limit: 50, Status: &active, CurrentOnly: true}

	if code := q.Get("subject"); code != "" {
		sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		f.SubjectID = &sub.ID
	}
	if n := q.Get("number"); n != "" {
		num, err := strconv.Atoi(n)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "number must be an integer")
			return
		}
		f.Number = &num
	}
	if st := q.Get("status"); st != "" {
		if st != string(domain.TaskActive) {
			writeJSON(w, http.StatusOK, []taskView{})
			return
		}
	}
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			f.Limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			f.Offset = v
		}
	}

	tasks, err := s.store.ListTasks(r.Context(), f)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, toTaskView(t))
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id")
		return
	}
	t, err := s.store.GetTask(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if t.Status != domain.TaskActive || !t.Source.Current(time.Now()) {
		writeErr(w, http.StatusNotFound, "Задание недоступно для новой тренировки")
		return
	}
	writeJSON(w, http.StatusOK, toTaskView(t))
}

// handleListTestTasks returns a test's tasks in order as student-safe views (no
// answers) — what a student loads to solve an assigned variant. The full detail
// with answers stays teacher-only (GET /api/admin/tests/{id}).
func (s *Server) handleListTestTasks(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	tasks, err := s.store.ListTestTasks(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, toTaskView(t))
	}
	writeJSON(w, http.StatusOK, views)
}
