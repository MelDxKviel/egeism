package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/bank"
	"egeism/internal/domain"
	"egeism/internal/store"
)

// RunBankSync is owned by the API process context, never by a browser request.
// The initial sweep fills a cold bank before students arrive; subsequent sweeps
// replenish low stock and re-check source availability without teacher action.
func (s *Server) RunBankSync(ctx context.Context) {
	if s.bankSync == nil {
		return
	}
	done := make(chan struct{})
	go func() { defer close(done); s.replenishBank(ctx) }()
	s.bankSync.Run(ctx)
	<-done
}

func (s *Server) executeBankSync(ctx context.Context, job domain.BankSyncJob) (*domain.BankSyncResult, error) {
	if job.Kind == "repair" {
		res, err := s.refetchFormulas(ctx, job.Subject)
		return &domain.BankSyncResult{Source: "real", Updated: res.Updated, Scanned: res.Scanned}, err
	}
	status := domain.TaskDraft
	if job.Active {
		status = domain.TaskActive
	}
	res, mode, err := s.fetchAndIngest(ctx, job.Subject, job.Limit, job.Number, status)
	result := &domain.BankSyncResult{Fetched: res.Fetched, Inserted: res.Inserted, Skipped: res.Skipped, Invalid: res.Invalid, Promoted: res.Promoted, Held: res.Held, Source: mode}
	if err == nil && (res.Fetched == 0 || res.Invalid == res.Fetched || (job.Active && res.Held+res.Invalid >= res.Fetched)) {
		err = fmt.Errorf("source returned no eligible tasks")
	}
	return result, err
}

func (s *Server) enqueueBankSync(ctx context.Context, subject domain.SubjectCode, number, limit int, active, automatic bool) (domain.BankSyncJob, error) {
	cooldown := 2 * time.Minute
	if automatic {
		cooldown = 15 * time.Minute
	}
	job, err := s.store.EnqueueBankSync(ctx, subject, number, min(max(limit, 1), 200), active, cooldown, "fetch")
	if err == nil && s.bankSync != nil {
		s.bankSync.Wake()
	}
	return job, err
}

// queuePracticeRefill adds only a short DB operation to a practice read. A
// failing source or full queue must never hide the tasks already in the bank.
func (s *Server) queuePracticeRefill(ctx context.Context, subject domain.SubjectCode, number int) {
	if s.bankSync == nil {
		return
	}
	limit := 80
	if number > 0 {
		limit = 20
	}
	queueCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := s.enqueueBankSync(queueCtx, subject, number, limit, true, true); err != nil && !errors.Is(err, store.ErrBankQueueFull) {
		slog.Warn("queue practice refill", "subject", subject, "err", err)
	}
}

func (s *Server) replenishBank(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		sweepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		subjects, err := s.store.ListSubjects(sweepCtx)
		if err == nil {
			for _, sub := range subjects {
				counts, countErr := s.store.TaskCountsByNumber(sweepCtx, sub.ID)
				if countErr != nil {
					continue
				}
				if needsBankRefill(counts, sub.Code) {
					s.queuePracticeRefill(sweepCtx, sub.Code, 0)
				}
			}
		}
		_ = s.store.PruneBankSync(sweepCtx)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func needsBankRefill(counts []domain.NumberAvailability, subjects ...domain.SubjectCode) bool {
	if len(subjects) > 0 {
		expected := map[domain.SubjectCode]int{domain.SubjectRus: 26, domain.SubjectMath: 19, domain.SubjectInf: 27, domain.SubjectSoc: 16}[subjects[0]]
		available := make(map[int]int, len(counts))
		for _, row := range counts {
			available[row.Number] = row.Active
		}
		for number := 1; number <= expected; number++ {
			if available[number] < 10 {
				return true
			}
		}
	}
	if len(counts) == 0 {
		return true
	}
	for _, row := range counts {
		if row.Active < 10 {
			return true
		}
	}
	return false
}

func (s *Server) writeBankSync(w http.ResponseWriter, r *http.Request, job domain.BankSyncJob, err error) {
	if errors.Is(err, store.ErrBankQueueFull) {
		w.Header().Set("Retry-After", "30")
		writeErr(w, http.StatusTooManyRequests, "Обновления уже выполняются. Повторите позже.")
		return
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	statusPath := "/api/practice/bank/fetch/"
	if strings.HasPrefix(r.URL.Path, "/api/admin/") {
		statusPath = "/api/admin/tasks/fetch/"
	}
	w.Header().Set("Location", statusPath+job.ID.String())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) readableBankSync(w http.ResponseWriter, r *http.Request) (domain.BankSyncJob, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid job id")
		return domain.BankSyncJob{}, false
	}
	job, err := s.store.GetBankSyncJob(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return domain.BankSyncJob{}, false
	}
	u, _ := userFrom(r.Context())
	if u.Role == domain.RoleStudent {
		if !job.Active {
			writeErr(w, http.StatusNotFound, "not found")
			return domain.BankSyncJob{}, false
		}
	} else if !s.subjectInScope(w, u, job.Subject) {
		return domain.BankSyncJob{}, false
	}
	return job, true
}

func (s *Server) handleBankSyncStatus(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/admin/") {
		if _, ok := s.requireTeacher(w, r); !ok {
			return
		}
	}
	job, ok := s.readableBankSync(w, r)
	if ok {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, job)
	}
}

func (s *Server) handleCancelBankSync(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	job, ok := s.readableBankSync(w, r)
	if !ok {
		return
	}
	job, err := s.store.CancelBankSync(r.Context(), job.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// Keep construction here so HTTP handlers depend on the bank service only via
// this one owned background worker.
func (s *Server) initBankSync() {
	if s.fetcherURL != "" {
		s.bankSync = bank.New(s.store, s.executeBankSync)
	}
}
