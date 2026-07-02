package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/checker"
	"egeism/internal/domain"
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
	// An attempt may claim an assignment only if that assignment is really this
	// student's and really for this test — otherwise finishing the attempt would
	// mark someone else's assignment done.
	if req.AssignmentID != nil {
		asg, err := s.store.GetAssignment(r.Context(), *req.AssignmentID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if asg.StudentID != user.ID {
			writeErr(w, http.StatusForbidden, "assignment does not belong to user")
			return
		}
		if asg.TestID != req.TestID {
			writeErr(w, http.StatusBadRequest, "assignment is for a different test")
			return
		}
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
	// Finishing an assigned test completes the assignment (scheduled → done), so
	// the "Назначено тебе" feed and the teacher's overview reflect it. Best-effort:
	// the finished attempt itself is already the source of truth.
	if att.AssignmentID != nil {
		if _, err := s.store.SetAssignmentStatus(r.Context(), *att.AssignmentID, domain.AssignmentDone); err != nil {
			slog.Warn("mark assignment done failed", "assignment", *att.AssignmentID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, finished)
}

// attemptReadable guards attempt reads: the owning student or any teacher (the
// reviewer) may see an attempt's answers; other students may not.
func (s *Server) attemptReadable(w http.ResponseWriter, r *http.Request, attemptID uuid.UUID) bool {
	user, _ := userFrom(r.Context())
	if user.Role == domain.RoleTeacher {
		return true
	}
	att, err := s.store.GetAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return false
	}
	if att.StudentID != user.ID {
		writeErr(w, http.StatusForbidden, "attempt does not belong to user")
		return false
	}
	return true
}

func (s *Server) handleListAttemptAnswers(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	if !s.attemptReadable(w, r, attemptID) {
		return
	}
	answers, err := s.store.ListAnswersForAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, answers)
}

// attemptReviewItem is one answered task in an attempt, enriched with the task's
// condition + media and the correct answer, so a teacher can review what the
// student saw and how they answered (not just the bare number).
type attemptReviewItem struct {
	AnswerID    uuid.UUID         `json:"answer_id"`
	TaskID      uuid.UUID         `json:"task_id"`
	Number      int               `json:"number"`
	Statement   string            `json:"statement"`
	Media       []domain.Media    `json:"media"`
	AnswerKind  domain.AnswerType `json:"answer_kind"`
	RawAnswer   string            `json:"raw_answer"`
	IsCorrect   bool              `json:"is_correct"`
	Correct     []string          `json:"correct"`
	TimeSpentMS int64             `json:"time_spent_ms"`
	AnsweredAt  time.Time         `json:"answered_at"`
}

// handleAttemptReview returns an attempt's answers joined to their tasks — the
// reviewable variant (condition + correct answer per task). Works for any attempt,
// including free-practice ones that have no stored test items.
func (s *Server) handleAttemptReview(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	if !s.attemptReadable(w, r, attemptID) {
		return
	}
	answers, err := s.store.ListAnswersForAttempt(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	items := make([]attemptReviewItem, 0, len(answers))
	for _, a := range answers {
		it := attemptReviewItem{
			AnswerID:    a.ID,
			TaskID:      a.TaskID,
			RawAnswer:   a.RawAnswer,
			IsCorrect:   a.IsCorrect,
			TimeSpentMS: a.TimeSpentMS,
			AnsweredAt:  a.AnsweredAt,
			Media:       []domain.Media{},
			Correct:     []string{},
		}
		// The task may have been deleted since; keep the answer row regardless.
		if task, terr := s.store.GetTask(r.Context(), a.TaskID); terr == nil {
			it.Number = task.Number
			it.Statement = task.Statement
			if task.Media != nil {
				it.Media = task.Media
			}
			it.AnswerKind = task.AnswerSchema.Type
			it.Correct = task.AnswerSchema.Correct
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, items)
}
