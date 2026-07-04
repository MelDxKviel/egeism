package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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

// Self-registration is gone (переработка №6): accounts are created only by an
// admin (any role, admin panel) or by a teacher (their students). The shared
// validation lives in validateNewCredentials / createAccount below.

// validateNewCredentials normalizes and validates a username/password pair for
// a new account. Returns a message for the 400 response when invalid.
func validateNewCredentials(username, password string) (string, string) {
	username = strings.TrimSpace(strings.ToLower(username))
	if len(username) < 3 {
		return "", "логин должен быть не короче 3 символов"
	}
	if len(password) < 6 {
		return "", "пароль должен быть не короче 6 символов"
	}
	return username, ""
}

// createAccount hashes the password and inserts the user, mapping the username
// collision to a friendly 409. Shared by the admin panel and teacher-creates-
// student paths.
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request, role domain.Role, name, username, password string, subject *domain.SubjectCode) (domain.User, bool) {
	username, msg := validateNewCredentials(username, password)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return domain.User{}, false
	}
	if name = strings.TrimSpace(name); name == "" {
		name = username
	}
	hash, err := hashPassword(password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not hash password")
		return domain.User{}, false
	}
	user, err := s.store.CreateUserWithCredentials(r.Context(), role, name, username, hash, subject)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			writeErr(w, http.StatusConflict, "этот логин уже занят")
			return domain.User{}, false
		}
		writeStoreErr(w, err)
		return domain.User{}, false
	}
	return user, true
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
	if !creds.User.IsActive {
		writeErr(w, http.StatusForbidden, "аккаунт отключён — обратись к администратору")
		return
	}
	s.respondWithToken(w, creds.User, http.StatusOK)
}

// handleMe returns the currently authenticated user.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	writeJSON(w, http.StatusOK, user)
}

// classRef is a lightweight class tag on a student row.
type classRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// studentSummary is one roster row: the student plus which of the teacher's
// classes they belong to (empty = "ученик без класса", the репетитор case).
type studentSummary struct {
	domain.User
	Classes []classRef `json:"classes"`
}

// handleListStudents returns the roster. A teacher sees their own students
// (enrollments), tagged with their class names; ?scope=all widens to every
// student on the platform (the add-to-class picker). An admin always sees all.
func (s *Server) handleListStudents(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	if user.Role != domain.RoleTeacher && user.Role != domain.RoleAdmin {
		writeErr(w, http.StatusForbidden, "teacher role required")
		return
	}
	var (
		students []domain.User
		err      error
	)
	if user.Role == domain.RoleAdmin || r.URL.Query().Get("scope") == "all" {
		students, err = s.store.ListStudents(r.Context())
	} else {
		students, err = s.store.ListStudentsForTeacher(r.Context(), user.ID)
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	byStudent := map[uuid.UUID][]classRef{}
	if user.Role == domain.RoleTeacher {
		memberships, err := s.store.ListClassMembershipsForTeacher(r.Context(), user.ID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		for _, m := range memberships {
			byStudent[m.StudentID] = append(byStudent[m.StudentID], classRef{ID: m.ClassID, Name: m.ClassName})
		}
	}
	out := make([]studentSummary, 0, len(students))
	for _, st := range students {
		classes := byStudent[st.ID]
		if classes == nil {
			classes = []classRef{}
		}
		out = append(out, studentSummary{User: st, Classes: classes})
	}
	writeJSON(w, http.StatusOK, out)
}

type createStudentReq struct {
	Name     string     `json:"name"`
	Username string     `json:"username"`
	Password string     `json:"password"`
	ClassID  *uuid.UUID `json:"class_id,omitempty"`
}

// handleCreateStudent lets a teacher create a student account (переработка №2/№6):
// the student is enrolled to the teacher immediately and optionally dropped
// straight into one of the teacher's classes.
func (s *Server) handleCreateStudent(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req createStudentReq
	if !decodeJSON(w, r, &req) {
		return
	}
	// Validate the class up front so a bad id doesn't leave a half-set-up student.
	if req.ClassID != nil {
		if _, ok := s.classOwned(w, r, teacher, *req.ClassID); !ok {
			return
		}
	}
	student, ok := s.createAccount(w, r, domain.RoleStudent, req.Name, req.Username, req.Password, nil)
	if !ok {
		return
	}
	if err := s.store.CreateEnrollment(r.Context(), teacher.ID, student.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	if req.ClassID != nil {
		if err := s.store.AddClassMember(r.Context(), *req.ClassID, teacher.ID, student.ID); err != nil {
			writeStoreErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, student)
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
	if !user.IsActive {
		writeErr(w, http.StatusForbidden, "аккаунт отключён — обратись к администратору")
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
	// The account may have been deactivated between issuing the code and
	// redeeming it — don't hand a disabled account a session token.
	if !user.IsActive {
		writeErr(w, http.StatusForbidden, "аккаунт отключён — обратись к администратору")
		return
	}
	s.respondWithToken(w, user, http.StatusOK)
}

// hashPassword bcrypt-hashes a password for storage.
func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
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
