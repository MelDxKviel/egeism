package api

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// authResp is the login/register/telegram response: a session token + the user.
type authResp struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

type registerReq struct {
	Role     domain.Role `json:"role"`
	Name     string      `json:"name"`
	Username string      `json:"username"`
	Password string      `json:"password"`
}

// handleRegister creates a username/password account for a student or teacher
// and returns a session token. Stage-1 family setup: create the two accounts
// once.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if req.Role != domain.RoleStudent && req.Role != domain.RoleTeacher {
		writeErr(w, http.StatusBadRequest, "role must be student or teacher")
		return
	}
	if len(req.Username) < 3 {
		writeErr(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if len(req.Password) < 6 {
		writeErr(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}
	name := req.Name
	if name == "" {
		name = req.Username
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, err := s.store.CreateUserWithCredentials(r.Context(), req.Role, name, req.Username, string(hash))
	if err != nil {
		// Unique-violation on username -> friendly conflict.
		writeErr(w, http.StatusConflict, "username already taken")
		return
	}
	s.respondWithToken(w, user, http.StatusCreated)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin verifies credentials and returns a session token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	creds, err := s.store.GetCredentialsByUsername(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeStoreErr(w, err)
		return
	}
	if creds.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.respondWithToken(w, creds.User, http.StatusOK)
}

// handleMe returns the currently authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	writeJSON(w, http.StatusOK, user)
}

// handleListStudents returns the students a teacher oversees (stage-1: all).
func (s *Server) handleListStudents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	students, err := s.store.ListStudents(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, students)
}

type telegramAuthReq struct {
	TelegramID int64  `json:"telegram_id"`
	Name       string `json:"name"`
}

// handleTelegramAuth resolves a Telegram user to a student and returns a session
// token, so the bot authenticates the same way as the web (Bearer token) — it
// just enters via telegram_id instead of a password.
func (s *Server) handleTelegramAuth(w http.ResponseWriter, r *http.Request) {
	var req telegramAuthReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TelegramID == 0 {
		writeErr(w, http.StatusBadRequest, "telegram_id is required")
		return
	}
	name := req.Name
	if name == "" {
		name = "Ученик"
	}
	user, _, err := s.store.GetOrCreateStudentByTelegram(r.Context(), req.TelegramID, name)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.respondWithToken(w, user, http.StatusOK)
}

// respondWithToken mints a token for the user and writes the auth response.
func (s *Server) respondWithToken(w http.ResponseWriter, user domain.User, status int) {
	token, err := s.issueToken(user)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, status, authResp{Token: token, User: user})
}
