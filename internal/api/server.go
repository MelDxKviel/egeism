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
	store      *store.Store
	scheduler  AssignmentScheduler // optional; nil disables enqueue
	jwtSecret  string
	media      *media.Store // optional; nil disables media serving
	fetcherURL string       // optional; empty disables button-driven fetch
}

// NewServer builds a Server over the given store. scheduler and mediaStore may
// be nil; fetcherURL may be empty.
func NewServer(st *store.Store, sched AssignmentScheduler, jwtSecret string, mediaStore *media.Store, fetcherURL string) *Server {
	return &Server{store: st, scheduler: sched, jwtSecret: jwtSecret, media: mediaStore, fetcherURL: fetcherURL}
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
		// Auth: credential login for the web, telegram entry for the bot.
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/telegram", s.handleTelegramAuth)

		// Public-ish content reads.
		r.Get("/subjects", s.handleListSubjects)
		r.Get("/tasks", s.handleListTasks)
		r.Get("/tasks/{taskID}", s.handleGetTask)

		// Acting-user required (Bearer session token, see auth.go).
		r.Group(func(r chi.Router) {
			r.Use(s.withUser)

			// Current user + teacher's students.
			r.Get("/auth/me", s.handleMe)
			r.Get("/students", s.handleListStudents)

			// Student solve flow (§6 WS-A).
			r.Post("/practice", s.handleStartPractice)
			r.Get("/practice/tasks", s.handlePracticeTasks)
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
			r.Post("/admin/tests", s.handleCreateTest)
			r.Patch("/admin/tests/{testID}", s.handleRenameTest)
			r.Delete("/admin/tests/{testID}", s.handleDeleteTest)
			r.Post("/admin/tests/generate", s.handleGenerateVariant)
			r.Post("/admin/tests/{testID}/items", s.handleAddTestItem)
			r.Post("/admin/assignments", s.handleCreateAssignment)
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
