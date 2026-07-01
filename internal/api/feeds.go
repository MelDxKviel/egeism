package api

import "net/http"

// handleStudentAssignments lists a student's assignments with test info, for the
// "Назначено тебе" card and the teacher dashboard.
func (s *Server) handleStudentAssignments(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	cards, err := s.store.ListAssignmentCards(r.Context(), studentID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

// handleStudentAttempts lists a student's recent attempts with scores, for the
// "Недавние решения" / "Свежие попытки" feed.
func (s *Server) handleStudentAttempts(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	limit := queryInt(r, "limit", 20)
	items, err := s.store.ListAttemptSummaries(r.Context(), studentID, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleMasterySeries returns weekly per-number success points for the mastery
// line chart (contract gap #3 in the design handoff).
func (s *Server) handleMasterySeries(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	points, err := s.store.MasterySeries(r.Context(), studentID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, points)
}
