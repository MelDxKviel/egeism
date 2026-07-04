package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// The admin panel (переработка №1): create/activate/deactivate/delete users,
// change roles and teacher subject scope, reset passwords, and watch
// platform-wide stats. Everything here is gated on RoleAdmin.

// handleAdminListUsers returns every account, admins/teachers first.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type adminCreateUserReq struct {
	Role     domain.Role `json:"role"`
	Name     string      `json:"name"`
	Username string      `json:"username"`
	Password string      `json:"password"`
	// Subject scopes a teacher to one subject; empty = сверхучитель. Ignored
	// for other roles.
	Subject *domain.SubjectCode `json:"subject,omitempty"`
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var req adminCreateUserReq
	if !decodeJSON(w, r, &req) {
		return
	}
	subject, ok := validRoleSubject(w, req.Role, req.Subject)
	if !ok {
		return
	}
	user, ok := s.createAccount(w, r, req.Role, req.Name, req.Username, req.Password, subject)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

type adminUpdateUserReq struct {
	Name *string `json:"name,omitempty"`
	// Role + Subject travel together: only teachers carry a subject, and an
	// empty subject on a teacher means сверхучитель.
	Role     *domain.Role        `json:"role,omitempty"`
	Subject  *domain.SubjectCode `json:"subject,omitempty"`
	IsActive *bool               `json:"is_active,omitempty"`
	Password *string             `json:"password,omitempty"`
}

// handleAdminUpdateUser applies partial account edits. Self-guards keep the
// panel from locking itself out: an admin can't deactivate, demote or delete
// their own account.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req adminUpdateUserReq
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if id == admin.ID && (req.Role != nil || (req.IsActive != nil && !*req.IsActive)) {
		writeErr(w, http.StatusBadRequest, "нельзя менять роль или отключать собственный аккаунт")
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "имя не может быть пустым")
			return
		}
		if user, err = s.store.SetUserName(r.Context(), id, name); err != nil {
			writeStoreErr(w, err)
			return
		}
	}
	if req.Role != nil || req.Subject != nil {
		role := user.Role
		if req.Role != nil {
			role = *req.Role
		}
		subject := req.Subject
		if req.Subject == nil && role == user.Role {
			subject = user.Subject // subject untouched unless sent or role changed
		}
		subject, ok := validRoleSubject(w, role, subject)
		if !ok {
			return
		}
		if user, err = s.store.SetUserRoleSubject(r.Context(), id, role, subject); err != nil {
			writeStoreErr(w, err)
			return
		}
	}
	if req.IsActive != nil {
		if user, err = s.store.SetUserActive(r.Context(), id, *req.IsActive); err != nil {
			writeStoreErr(w, err)
			return
		}
	}
	if req.Password != nil {
		if _, msg := validateNewCredentials("___", *req.Password); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		hash, err := hashPassword(*req.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not hash password")
			return
		}
		if err := s.store.SetUserPassword(r.Context(), id, hash); err != nil {
			writeStoreErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, user)
}

// handleAdminDeleteUser removes an account outright. Accounts with solve
// history / created content are refused (409) — deactivate them instead, so
// student data never silently disappears.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if id == admin.ID {
		writeErr(w, http.StatusBadRequest, "нельзя удалить собственный аккаунт")
		return
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrInUse) {
			writeErr(w, http.StatusConflict, "у пользователя есть история решений или созданные тесты — отключите аккаунт вместо удаления")
			return
		}
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// handleAdminStats returns platform-wide counters + per-subject activity.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	stats, err := s.store.PlatformStats(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAdminListClasses lists every class with its teacher (admin view).
func (s *Server) handleAdminListClasses(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	classes, err := s.store.ListAllClasses(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, classes)
}

// validRoleSubject validates a role + teacher-subject pair: role must be one of
// the three, subject only rides on teachers (nil = сверхучитель) and must be a
// known code. Writes the 400 itself.
func validRoleSubject(w http.ResponseWriter, role domain.Role, subject *domain.SubjectCode) (*domain.SubjectCode, bool) {
	switch role {
	case domain.RoleStudent, domain.RoleTeacher, domain.RoleAdmin:
	default:
		writeErr(w, http.StatusBadRequest, "role must be student, teacher or admin")
		return nil, false
	}
	if role != domain.RoleTeacher || subject == nil || *subject == "" {
		return nil, true
	}
	switch *subject {
	case domain.SubjectRus, domain.SubjectMath, domain.SubjectInf, domain.SubjectSoc:
		return subject, true
	default:
		writeErr(w, http.StatusBadRequest, "unknown subject")
		return nil, false
	}
}
