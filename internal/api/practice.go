package api

import (
	"net/http"

	"github.com/google/uuid"

	"egeism/internal/domain"
)

type practiceReq struct {
	Subject domain.SubjectCode `json:"subject"`
}

type practiceResp struct {
	TestID    uuid.UUID `json:"test_id"`
	AttemptID uuid.UUID `json:"attempt_id"`
}

// masteredThreshold: a task solved correctly this many times stops appearing in
// practice (§ user request: don't repeat what's already learned).
const masteredThreshold = 2

// handlePracticeTasks returns active tasks for the acting student to solve,
// EXCLUDING ones they've already mastered (solved correctly >= threshold),
// student-safe (no answers), random order.
func (s *Server) handlePracticeTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	code := r.URL.Query().Get("subject")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "subject query param is required")
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	limit := queryInt(r, "limit", 20)
	tasks, err := s.store.PracticeTasks(r.Context(), user.ID, sub.ID, masteredThreshold, limit)
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

// handleStartPractice opens an ad-hoc free-solve session for the acting student:
// it get-or-creates their practice test for the subject and starts an attempt.
// The client then submits answers to attempt_id for any task. Used by the bot
// and web quick-practice.
func (s *Server) handleStartPractice(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	var req practiceReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	test, err := s.store.GetOrCreatePracticeTest(r.Context(), sub.ID, user.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	att, err := s.store.StartAttempt(r.Context(), user.ID, test.ID, nil)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, practiceResp{TestID: test.ID, AttemptID: att.ID})
}
