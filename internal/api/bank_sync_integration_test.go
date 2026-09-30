package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// These run against an isolated schema with EGEISM_TEST_DATABASE_URL (CI has
// PostgreSQL). They exercise real migrations, cross-request dedup and leases.
func TestBankQueueDedupLeaseAndCancellation(t *testing.T) {
	s := reviewTestServer(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := s.store.EnqueueBankSync(ctx, domain.SubjectMath, 0, 30, true, 15*time.Minute, "fetch")
			if err != nil {
				t.Error(err)
				return
			}
			ids <- job.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id uuid.UUID
	for got := range ids {
		if id == uuid.Nil {
			id = got
		}
		if got != id {
			t.Fatal("duplicate source jobs")
		}
	}
	first, err := s.store.ClaimBankSync(ctx)
	if err != nil || first.ID != id || first.Attempts != 1 {
		t.Fatalf("claim: %+v %v", first, err)
	}
	if _, err := s.store.ClaimBankSync(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("claimed running work: %v", err)
	}
	if _, err := s.store.Pool().Exec(ctx, "UPDATE bank_sync_jobs SET lease_until = now() - interval '1 minute' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	second, err := s.store.ClaimBankSync(ctx)
	if err != nil || second.ID != id || second.Attempts != 2 {
		t.Fatalf("lease recovery: %+v %v", second, err)
	}
	if err := s.store.FinishBankSync(ctx, first, &domain.BankSyncResult{Inserted: 99}, ""); err != nil {
		t.Fatal(err)
	}
	stillRunning, _ := s.store.GetBankSyncJob(ctx, id)
	if stillRunning.State != "running" || stillRunning.Result != nil {
		t.Fatal("stale worker overwrote replacement")
	}
	if _, err := s.store.CancelBankSync(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.store.FinishBankSync(ctx, second, &domain.BankSyncResult{}, ""); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := s.store.GetBankSyncJob(ctx, id)
	if cancelled.State != "cancelled" {
		t.Fatal("worker overwrote cancellation")
	}
}

func TestFetchRequestReturnsBeforeSlowSource(t *testing.T) {
	s := reviewTestServer(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer source.Close()
	defer close(release)
	s.fetcherURL = source.URL
	s.initBankSync()
	requestCtx, requestCancel := context.WithCancel(context.Background())
	user := domain.User{Role: domain.RoleTeacher}
	r := httptest.NewRequest(http.MethodPost, "/api/admin/tasks/fetch", bytes.NewBufferString(`{"subject":"math","limit":30,"active":true}`))
	r = r.WithContext(context.WithValue(requestCtx, userKey, user))
	w := httptest.NewRecorder()
	started := time.Now()
	s.handleFetchTasks(w, r)
	if w.Code != http.StatusAccepted || time.Since(started) > time.Second {
		t.Fatalf("request waited for source: %d %s", w.Code, w.Body.String())
	}
	var job domain.BankSyncJob
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	requestCancel() // the closed browser request must not own the job lifetime
	workerCtx, stopWorker := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.bankSync.Run(workerCtx) }()
	defer func() { stopWorker(); <-done }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("queued job did not run after request cancellation")
	}
	stored, err := s.store.GetBankSyncJob(context.Background(), job.ID)
	if err != nil || stored.State != "running" {
		t.Fatalf("durable job lost: %+v %v", stored, err)
	}
}

func TestBankQueueGlobalBounds(t *testing.T) {
	s := reviewTestServer(t)
	ctx := context.Background()
	for _, spec := range []struct {
		subject domain.SubjectCode
		number  int
	}{
		{domain.SubjectMath, 1}, {domain.SubjectMath, 2}, {domain.SubjectRus, 1}, {domain.SubjectInf, 1},
	} {
		if _, err := s.store.EnqueueBankSync(ctx, spec.subject, spec.number, 20, true, 0, "fetch"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.store.ClaimBankSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.store.ClaimBankSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Subject == second.Subject {
		t.Fatal("concurrent ingest for one subject")
	}
	if _, err := s.store.ClaimBankSync(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("global worker cap not enforced: %v", err)
	}
	// Four pending jobs already exist; accept only another 28 unique requests.
	for n := 3; n <= 30; n++ {
		if _, err := s.store.EnqueueBankSync(ctx, domain.SubjectMath, n, 20, true, 0, "fetch"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.store.EnqueueBankSync(ctx, domain.SubjectMath, 31, 20, true, 0, "fetch"); !errors.Is(err, store.ErrBankQueueFull) {
		t.Fatalf("queue cap not enforced: %v", err)
	}
}
