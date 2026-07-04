package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// Password reset: there is no self-service email flow — accounts are handed
// out by a teacher/admin, so recovery goes through them too. The student hits
// «забыл пароль» on the login screen, which drops an in-app notification to
// their teachers and the admins; any of them issues a one-time reset link
// (valid resetTokenTTL) that lets the user set a new password themselves. An
// expired link is dead — they simply issue a fresh one.

// resetTokenTTL is how long a password-reset link stays valid after issue.
const resetTokenTTL = time.Hour

type forgotPasswordReq struct {
	Username string `json:"username"`
}

// handleForgotPassword is the public «забыл пароль» button. It never reveals
// whether the username exists (the response is identical either way) — it only
// notifies the people who can help: the student's teachers and active admins.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if req.Username == "" {
		writeErr(w, http.StatusBadRequest, "username is required")
		return
	}
	ok := map[string]bool{"ok": true}
	creds, err := s.store.GetCredentialsByUsername(r.Context(), req.Username)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("forgot-password lookup failed", "err", err)
		}
		writeJSON(w, http.StatusOK, ok)
		return
	}
	user := creds.User

	// Recipients: the student's teachers + every active admin; a teacher's (or
	// admin's) own request goes to admins only. Never notify the requester about
	// themselves. Duplicate unread notifications are suppressed in the store.
	recipients := map[uuid.UUID]bool{}
	if user.Role == domain.RoleStudent {
		teachers, err := s.store.ListTeacherIDsForStudent(r.Context(), user.ID)
		if err != nil {
			slog.Warn("forgot-password teacher lookup failed", "user", user.ID, "err", err)
		}
		for _, id := range teachers {
			recipients[id] = true
		}
	}
	admins, err := s.store.ListActiveAdminIDs(r.Context())
	if err != nil {
		slog.Warn("forgot-password admin lookup failed", "err", err)
	}
	for _, id := range admins {
		recipients[id] = true
	}
	delete(recipients, user.ID)
	for id := range recipients {
		if err := s.store.CreatePasswordResetNotification(r.Context(), id, user.ID); err != nil {
			slog.Warn("create forgot-password notification failed", "recipient", id, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, ok)
}

// resetLinkResp is what the issuing teacher/admin gets back: the raw token and
// its expiry. The web composes the absolute link from its own origin
// (`/#reset=<token>`), so the API needs no WEB_URL plumbing.
type resetLinkResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleCreatePasswordResetLink issues a one-time reset token for a user. An
// admin may reset anyone; a teacher only their own (enrolled) students.
func (s *Server) handleCreatePasswordResetLink(w http.ResponseWriter, r *http.Request) {
	actor, _ := userFrom(r.Context())
	targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	target, err := s.store.GetUser(r.Context(), targetID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	switch actor.Role {
	case domain.RoleAdmin:
		// any account
	case domain.RoleTeacher:
		if target.Role != domain.RoleStudent {
			writeErr(w, http.StatusForbidden, "учитель может сбросить пароль только ученику")
			return
		}
		enrolled, err := s.store.IsTeacherOfStudent(r.Context(), actor.ID, targetID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if !enrolled {
			writeErr(w, http.StatusForbidden, "это не ваш ученик")
			return
		}
	default:
		writeErr(w, http.StatusForbidden, "teacher or admin role required")
		return
	}
	token, expires, err := s.store.CreatePasswordResetToken(r.Context(), targetID, actor.ID, resetTokenTTL)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, resetLinkResp{Token: token, ExpiresAt: expires})
}

// resetPeekResp greets the link holder before they type a new password.
type resetPeekResp struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handlePeekResetToken tells the reset page whether a token is still usable
// (404 = unknown/expired/used → «попроси новую ссылку») and whom it is for.
func (s *Server) handlePeekResetToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}
	user, expires, err := s.store.PeekPasswordResetToken(r.Context(), token)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resetPeekResp{Name: user.Name, ExpiresAt: expires})
}

type resetPasswordReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handleResetPassword redeems a reset link: validates the token, swaps the
// password hash (one tx, token burned) and logs the user straight in. Same
// post-redeem deactivation guard as the Telegram link flow: the password
// change stands, but a disabled account gets no session token.
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		writeErr(w, http.StatusBadRequest, "token is required")
		return
	}
	if _, msg := validateNewCredentials("___", req.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, err := s.store.RedeemPasswordResetToken(r.Context(), req.Token, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusBadRequest, "ссылка недействительна или истекла — попроси новую")
			return
		}
		writeStoreErr(w, err)
		return
	}
	if !user.IsActive {
		writeErr(w, http.StatusForbidden, "аккаунт отключён — обратись к администратору")
		return
	}
	s.respondWithToken(w, user, http.StatusOK)
}
