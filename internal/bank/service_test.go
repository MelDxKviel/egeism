package bank

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

type fakeRepository struct {
	mu       sync.Mutex
	job      domain.BankSyncJob
	finished chan string
}

func (r *fakeRepository) ClaimBankSync(context.Context) (domain.BankSyncJob, error) {
	return domain.BankSyncJob{}, store.ErrNotFound
}
func (r *fakeRepository) GetBankSyncJob(context.Context, uuid.UUID) (domain.BankSyncJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job, nil
}
func (r *fakeRepository) FinishBankSync(_ context.Context, _ domain.BankSyncJob, _ *domain.BankSyncResult, message string) error {
	r.finished <- message
	return nil
}

func TestWorkerObservesDurableCancellation(t *testing.T) {
	job := domain.BankSyncJob{ID: uuid.New(), State: "running", Attempts: 1}
	repo := &fakeRepository{job: job, finished: make(chan string, 1)}
	entered := make(chan struct{})
	exited := make(chan struct{})
	s := New(repo, func(ctx context.Context, _ domain.BankSyncJob) (*domain.BankSyncResult, error) {
		close(entered)
		<-ctx.Done()
		close(exited)
		return nil, ctx.Err()
	})
	s.poll = time.Millisecond
	go s.runJob(context.Background(), job)
	<-entered
	repo.mu.Lock()
	repo.job.State = "cancelled"
	repo.mu.Unlock()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("source call was not cancelled")
	}
	select {
	case <-repo.finished:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish")
	}
}

func TestWorkerDeadlineAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "deadline"
		if shutdown {
			name = "shutdown"
		}
		t.Run(name, func(t *testing.T) {
			job := domain.BankSyncJob{ID: uuid.New(), State: "running", Attempts: 1}
			repo := &fakeRepository{job: job, finished: make(chan string, 1)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := New(repo, func(ctx context.Context, _ domain.BankSyncJob) (*domain.BankSyncResult, error) {
				if shutdown {
					cancel()
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
			s.deadline = 10 * time.Millisecond
			s.runJob(ctx, job)
			if shutdown {
				select {
				case <-repo.finished:
					t.Fatal("shutdown must leave job lease for recovery")
				default:
				}
			} else {
				select {
				case message := <-repo.finished:
					if message == "" {
						t.Fatal("deadline recorded as success")
					}
				default:
					t.Fatal("deadline not persisted")
				}
			}
		})
	}
}
