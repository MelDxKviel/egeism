// Package api is the HTTP layer: routing plus thin handlers over the store and
// checker. All business logic (answer checking, scoring) lives in the domain
// packages; handlers only translate HTTP <-> domain. The bot and web hit these
// same endpoints (§3).
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"egeism/internal/media"
	"egeism/internal/store"
)

// AssignmentScheduler enqueues a notification for a newly-created assignment.
// Implemented by scheduler.Enqueuer; kept as an interface so the API doesn't
// depend on asynq/redis directly and can run without a worker (nil = skip).
type AssignmentScheduler interface {
	ScheduleAssignmentNotification(ctx context.Context, assignmentID uuid.UUID, processAt time.Time) error
}

// Server holds handler dependencies.
type Server struct {
	store       *store.Store
	scheduler   AssignmentScheduler // optional; nil disables enqueue
	jwtSecret   string
	media       *media.Store // optional; nil disables media serving
	fetcherURL  string       // optional; empty disables button-driven fetch
	botUsername string       // optional; empty omits deep_link in link-code responses
}

// NewServer builds a Server over the given store. scheduler and mediaStore may
// be nil; fetcherURL and botUsername may be empty.
func NewServer(st *store.Store, sched AssignmentScheduler, jwtSecret string, mediaStore *media.Store, fetcherURL, botUsername string) *Server {
	return &Server{store: st, scheduler: sched, jwtSecret: jwtSecret, media: mediaStore, fetcherURL: fetcherURL, botUsername: botUsername}
}

// Router wires all routes and returns an http.Handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Generous cap: content fetch/import legitimately take longer than typical
	// reads. The fetcher enforces its own shorter deadline for unreachable sources.
	r.Use(middleware.Timeout(150 * time.Second))
	r.Use(corsDev)

	r.Get("/health", s.handleHealth)

	// Public media (task images/files). Keys are content hashes; no auth needed.
	r.Get("/api/media/*", s.handleGetMedia)

	r.Route("/api", func(r chi.Router) {
		// Public runtime flags. Self-registration is permanently gone; the
		// endpoint stays for older clients and always answers false.
		r.Get("/config", s.handleConfig)

		// Auth: credential login for the web, telegram entry for the bot.
		// There is no /auth/register: accounts are created by an admin (admin
		// panel) or by a teacher (their students).
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/telegram", s.handleTelegramAuth)      // bot: resolve a linked telegram_id → token
		r.Post("/auth/telegram/link", s.handleTelegramLink) // bot: redeem a link code → bind telegram_id

		// Password recovery (no email on the platform): «забыл пароль» notifies
		// the user's teachers/admins, who issue a one-hour reset link; the link
		// holder checks it and sets a new password here.
		r.Post("/auth/forgot-password", s.handleForgotPassword)
		r.Get("/auth/reset-password/{token}", s.handlePeekResetToken)
		r.Post("/auth/reset-password", s.handleResetPassword)

		// Public-ish content reads.
		r.Get("/subjects", s.handleListSubjects)
		r.Get("/tasks", s.handleListTasks)
		r.Get("/tasks/{taskID}", s.handleGetTask)

		// Acting-user required (Bearer session token, see auth.go).
		r.Group(func(r chi.Router) {
			r.Use(s.withUser)

			// Current user, profile and the teacher's roster.
			r.Get("/auth/me", s.handleMe)
			r.Post("/auth/telegram/link-code", s.handleTelegramLinkCode) // web: issue a code to link this account to Telegram
			r.Get("/profile", s.handleProfile)
			r.Get("/students", s.handleListStudents)
			r.Post("/students", s.handleCreateStudent) // teacher creates a student account
			// Enrollment link management: a student may have SEVERAL teachers
			// (m2m). Take an existing student onto my roster / drop them from it.
			r.Post("/students/{studentID}/enroll", s.handleEnrollStudent)
			r.Delete("/students/{studentID}/enroll", s.handleUnenrollStudent)
			// Reset link for a user: admin → anyone, teacher → their students.
			r.Post("/users/{userID}/password-reset-link", s.handleCreatePasswordResetLink)

			// Classes (teacher-owned groups of students).
			r.Get("/classes", s.handleListClasses)
			r.Post("/classes", s.handleCreateClass)
			r.Get("/classes/{classID}", s.handleGetClass)
			r.Patch("/classes/{classID}", s.handleRenameClass)
			r.Delete("/classes/{classID}", s.handleDeleteClass)
			r.Post("/classes/{classID}/members", s.handleAddClassMember)
			r.Delete("/classes/{classID}/members/{studentID}", s.handleRemoveClassMember)
			r.Get("/classes/{classID}/overview", s.handleClassOverview)

			// Student solve flow (§6 WS-A).
			r.Post("/practice", s.handleStartPractice)
			r.Get("/practice/tasks", s.handlePracticeTasks)
			r.Get("/tests/{testID}/tasks", s.handleListTestTasks) // student-safe: solve an assigned variant
			r.Post("/attempts", s.handleStartAttempt)
			r.Post("/attempts/{attemptID}/answers", s.handleSubmitAnswer)
			r.Post("/attempts/{attemptID}/finish", s.handleFinishAttempt)
			r.Get("/attempts/{attemptID}/answers", s.handleListAttemptAnswers)
			r.Get("/attempts/{attemptID}/review", s.handleAttemptReview)

			// Statistics (§6 WS-A/WS-C).
			r.Get("/students/{studentID}/stats/heatmap", s.handleHeatmap)
			r.Get("/students/{studentID}/stats/mastery", s.handleMastery)
			r.Get("/students/{studentID}/stats/mastery-series", s.handleMasterySeries)
			r.Get("/students/{studentID}/stats/weak-spots", s.handleWeakSpots)
			r.Get("/students/{studentID}/stats/forecast", s.handleForecast)
			r.Get("/students/{studentID}/stats/day", s.handleDayDrilldown)

			// Feeds the UI needs (design handoff contract gaps).
			r.Get("/students/{studentID}/assignments", s.handleStudentAssignments)
			r.Get("/students/{studentID}/attempts", s.handleStudentAttempts)

			// In-app notifications (the web bell): assigned tests for students,
			// completed assignments for teachers.
			r.Get("/notifications", s.handleListNotifications)
			r.Post("/notifications/read-all", s.handleMarkAllNotificationsRead)
			r.Post("/notifications/{notificationID}/read", s.handleMarkNotificationRead)

			// Admin/authoring (§6 WS-C). Kept in one package for stage 1.
			r.Get("/admin/tasks", s.handleListAdminTasks)
			r.Post("/admin/tasks", s.handleCreateTask)
			r.Delete("/admin/tasks", s.handleClearBank) // ?subject=<code>: wipe the bank
			r.Post("/admin/tasks/import", s.handleImportTasks)
			r.Post("/admin/tasks/fetch", s.handleFetchTasks)
			r.Post("/admin/tasks/refetch-formulas", s.handleRefetchFormulas)
			r.Patch("/admin/tasks/{taskID}/answer", s.handleUpdateTaskAnswer)
			r.Patch("/admin/tasks/{taskID}/status", s.handleSetTaskStatus)
			r.Get("/admin/tests", s.handleListTests)
			r.Get("/admin/tests/{testID}", s.handleGetTestDetail)
			r.Get("/admin/tests/{testID}/export.pdf", s.handleExportTestPDF)
			r.Post("/admin/tests", s.handleCreateTest)
			r.Patch("/admin/tests/{testID}", s.handleRenameTest)
			r.Delete("/admin/tests/{testID}", s.handleDeleteTest)
			r.Post("/admin/tests/generate", s.handleGenerateVariant)
			r.Post("/admin/tests/{testID}/items", s.handleAddTestItem)
			r.Post("/admin/assignments", s.handleCreateAssignment)

			// Admin panel: user management + platform-wide stats (admin role).
			r.Get("/admin/users", s.handleAdminListUsers)
			r.Post("/admin/users", s.handleAdminCreateUser)
			r.Patch("/admin/users/{userID}", s.handleAdminUpdateUser)
			r.Delete("/admin/users/{userID}", s.handleAdminDeleteUser)
			r.Get("/admin/stats", s.handleAdminStats)
			r.Get("/admin/classes", s.handleAdminListClasses)
		})
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleConfig exposes public runtime flags the web needs before authenticating.
// Self-registration was removed for good (accounts come from the admin panel or
// a teacher), so the flag is a constant kept only for older clients.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"allow_registration": false})
}

// corsDev is a permissive CORS policy for local web development. Tighten before
// exposing the API publicly.
func corsDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
