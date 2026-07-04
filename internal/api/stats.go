package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/scoring"
)

// resolveStudent parses the studentID path param and enforces access: a student
// may only read their own stats; a teacher only their enrolled students'
// (class members and students they created); an admin — anyone's.
func (s *Server) resolveStudent(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "studentID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid student id")
		return uuid.Nil, false
	}
	user, _ := userFrom(r.Context())
	switch user.Role {
	case domain.RoleAdmin:
		return id, true
	case domain.RoleTeacher:
		if !s.studentOfTeacher(w, r, user, id) {
			return uuid.Nil, false
		}
		return id, true
	case domain.RoleStudent:
		if user.ID != id {
			writeErr(w, http.StatusForbidden, "cannot read another student's stats")
			return uuid.Nil, false
		}
		return id, true
	default:
		writeErr(w, http.StatusForbidden, "forbidden")
		return uuid.Nil, false
	}
}

func (s *Server) subjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	code := r.URL.Query().Get("subject")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "subject query param is required")
		return uuid.Nil, false
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
	if err != nil {
		writeStoreErr(w, err)
		return uuid.Nil, false
	}
	return sub.ID, true
}

func (s *Server) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	since := time.Now().AddDate(-1, 0, 0) // default: last year
	if raw := r.URL.Query().Get("since"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			since = t
		}
	}
	cells, err := s.store.Heatmap(r.Context(), studentID, since)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cells)
}

func (s *Server) handleMastery(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	rows, err := s.store.MasteryByNumber(r.Context(), studentID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleWeakSpots(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	minAttempts := queryInt(r, "min", 3)
	limit := queryInt(r, "limit", 5)
	rows, err := s.store.WeakSpots(r.Context(), studentID, subjectID, minAttempts, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleForecast(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	code := r.URL.Query().Get("subject")
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	acc, err := s.store.SubjectAccuracy(r.Context(), studentID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	forecast := scoring.Predict(domain.SubjectCode(code), acc.Accuracy)
	writeJSON(w, http.StatusOK, forecast)
}

func (s *Server) handleDayDrilldown(w http.ResponseWriter, r *http.Request) {
	studentID, ok := s.resolveStudent(w, r)
	if !ok {
		return
	}
	raw := r.URL.Query().Get("date")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "date query param is required (YYYY-MM-DD)")
		return
	}
	day, err := time.ParseInLocation("2006-01-02", raw, time.UTC)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	rows, err := s.store.AnswersOnDay(r.Context(), studentID, day, day.AddDate(0, 0, 1))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
