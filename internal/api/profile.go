package api

import (
	"encoding/json"
	"net/http"

	"egeism/internal/domain"
)

// profileResp is the "личный профиль" payload (переработка №4). Students see
// their personal info + which classes (and whose) they're in; teachers see
// their subject scope, their classes and roster size. Per-student stats stay
// on the existing /students/{id}/stats endpoints — the profile is identity.
type profileResp struct {
	User    domain.User    `json:"user"`
	Classes []domain.Class `json:"classes"`
	// Teachers is the student's full teacher list (enrollments) — a student may
	// have several, including ones with no class (the репетитор case).
	Teachers []domain.User `json:"teachers,omitempty"`
	// StudentsCount is the teacher's roster size (enrolled students).
	StudentsCount int `json:"students_count,omitempty"`
}

// handleSetAvatar — PUT /api/profile/avatar: replace the acting user's avatar
// config (any role — ученики, учителя и даже админы). `{"avatar": null}`
// clears it back to the deterministic client-side default. The config is
// presentation-only, so beyond Validate() the API doesn't interpret it.
func (s *Server) handleSetAvatar(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	var body struct {
		Avatar *domain.Avatar `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Avatar != nil {
		if err := body.Avatar.Validate(); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	updated, err := s.store.SetUserAvatar(r.Context(), user.ID, body.Avatar)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	resp := profileResp{User: user, Classes: []domain.Class{}}
	switch user.Role {
	case domain.RoleStudent:
		classes, err := s.store.ListClassesForStudent(r.Context(), user.ID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		resp.Classes = classes
		teachers, err := s.store.ListTeachersForStudent(r.Context(), user.ID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		resp.Teachers = teachers
	case domain.RoleTeacher:
		classes, err := s.store.ListClassesForTeacher(r.Context(), user.ID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		resp.Classes = classes
		students, err := s.store.ListStudentsForTeacher(r.Context(), user.ID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		resp.StudentsCount = len(students)
	}
	writeJSON(w, http.StatusOK, resp)
}
