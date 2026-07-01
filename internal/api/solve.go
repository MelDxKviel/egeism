package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/checker"
)

type startAttemptReq struct {
	TestID       uuid.UUID  `json:"test_id"`
	AssignmentID *uuid.UUID `json:"assignment_id,omitempty"`
}

func (s *Server) handleStartAttempt(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	var req startAttemptReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TestID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "test_id is required")
		return
	}
	att, err := s.store.StartAttempt(r.Context(), user.ID, req.TestID, req.AssignmentID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, att)
}

type submitAnswerReq struct {
	TaskID      uuid.UUID `json:"task_id"`
	RawAnswer   string    `json:"raw_answer"`
	TimeSpentMS int64     `json:"time_spent_ms"`
}

type submitAnswerResp struct {
	IsCorrect bool     `json:"is_correct"`
	AnswerID  string   `json:"answer_id"`
	// Solution is revealed only on a wrong answer (post-commit), so the client
	// can show a разбор without ever seeing the answer before submitting (§3.4).
	Solution []string `json:"solution,omitempty"`
}

// handleSubmitAnswer is the spine of the product (M1): load the task, run the
// checker server-side, and record the verdict. The client never decides
// correctness.
func (s *Server) handleSubmitAnswer(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())

	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	var req submitAnswerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TaskID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "task_id is required")
		return
	}

	// The attempt must belong to the acting student.
	att, err := s.store.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if att.StudentID != user.ID {
		writeErr(w, http.StatusForbidden, "attempt does not belong to user")
		return
	}

	task, err := s.store.GetTask(r.Context(), req.TaskID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	correct := checker.Check(task.AnswerSchema, req.RawAnswer)

	ans, err := s.store.RecordAnswer(r.Context(), attemptID, req.TaskID, req.RawAnswer, correct, req.TimeSpentMS)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	resp := submitAnswerResp{IsCorrect: ans.IsCorrect, AnswerID: ans.ID.String()}
	if !correct {
		resp.Solution = task.AnswerSchema.Correct
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleFinishAttempt(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	att, err := s.store.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if att.StudentID != user.ID {
		writeErr(w, http.StatusForbidden, "attempt does not belong to user")
		return
	}
	finished, err := s.store.FinishAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, finished)
}

func (s *Server) handleListAttemptAnswers(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	answers, err := s.store.ListAnswersForAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, answers)
}
