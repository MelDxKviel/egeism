package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// linkCodeTTL is how long a Telegram link code stays valid after issue.
const linkCodeTTL = 15 * time.Minute

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
	if !s.allowRegistration {
		writeErr(w, http.StatusForbidden, "регистрация отключена")
		return
	}
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

// handleTelegramAuth resolves an already-linked Telegram id to its account and
// returns a session token — the bot's "login". It no longer auto-provisions an
// anonymous student: an unlinked telegram_id gets 404 so the bot prompts the
// user to link their web account first (see handleTelegramLink).
func (s *Server) handleTelegramAuth(w http.ResponseWriter, r *http.Request) {
	var req telegramAuthReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TelegramID == 0 {
		writeErr(w, http.StatusBadRequest, "telegram_id is required")
		return
	}
	user, err := s.store.GetUserByTelegram(r.Context(), req.TelegramID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "telegram not linked")
			return
		}
		writeStoreErr(w, err)
		return
	}
	s.respondWithToken(w, user, http.StatusOK)
}

// linkCodeResp is the payload the web shows so the user can complete linking in
// the bot: the raw code plus a ready-made deep link (when the bot username is set).
type linkCodeResp struct {
	Code      string    `json:"code"`
	DeepLink  string    `json:"deep_link,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleTelegramLinkCode issues a one-time code for the logged-in web user to
// bind their account to Telegram. The bot redeems it via handleTelegramLink.
func (s *Server) handleTelegramLinkCode(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	code, expires, err := s.store.CreateTelegramLinkCode(r.Context(), user.ID, linkCodeTTL)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	resp := linkCodeResp{Code: code, ExpiresAt: expires}
	if s.botUsername != "" {
		resp.DeepLink = "https://t.me/" + s.botUsername + "?start=" + code
	}
	writeJSON(w, http.StatusOK, resp)
}

type telegramLinkReq struct {
	Code       string `json:"code"`
	TelegramID int64  `json:"telegram_id"`
	Name       string `json:"name"`
}

// handleTelegramLink redeems a link code from the bot: it binds the chat's
// telegram_id to the code's account and returns a session token, so the bot then
// acts as that real (web) user — student or teacher.
func (s *Server) handleTelegramLink(w http.ResponseWriter, r *http.Request) {
	var req telegramLinkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" || req.TelegramID == 0 {
		writeErr(w, http.StatusBadRequest, "code and telegram_id are required")
		return
	}
	user, err := s.store.RedeemTelegramLinkCode(r.Context(), req.Code, req.TelegramID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeErr(w, http.StatusBadRequest, "код недействителен или истёк")
		case errors.Is(err, store.ErrTelegramTaken):
			writeErr(w, http.StatusConflict, "этот Telegram уже привязан к другому аккаунту")
		default:
			writeStoreErr(w, err)
		}
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
