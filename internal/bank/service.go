// Package bank runs the durable source synchronization queue independently of
// HTTP requests. Jobs have a bounded lifetime and can be cancelled from any API
// replica; database leases recover work after an interrupted process.
package bank

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

type Repository interface {
	ClaimBankSync(context.Context) (domain.BankSyncJob, error)
	GetBankSyncJob(context.Context, uuid.UUID) (domain.BankSyncJob, error)
	FinishBankSync(context.Context, domain.BankSyncJob, *domain.BankSyncResult, string) error
}

type Executor func(context.Context, domain.BankSyncJob) (*domain.BankSyncResult, error)

type Service struct {
	repo     Repository
	execute  Executor
	wake     chan struct{}
	poll     time.Duration
	deadline time.Duration
}

func New(repo Repository, execute Executor) *Service {
	return &Service{repo: repo, execute: execute, wake: make(chan struct{}, 1), poll: 2 * time.Second, deadline: 5 * time.Minute}
}

func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(s.poll)
	defer tick.Stop()
	for ctx.Err() == nil {
		job, err := s.repo.ClaimBankSync(ctx)
		if err == nil {
			s.runJob(ctx, job)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) && ctx.Err() == nil {
			slog.Error("claim bank sync", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-tick.C:
		}
	}
}

func (s *Service) runJob(parent context.Context, job domain.BankSyncJob) {
	ctx, cancel := context.WithTimeout(parent, s.deadline)
	defer cancel()
	// Cancellation is durable and works even when the cancel HTTP request lands
	// on a different replica. Also stop a stale worker after its lease is replaced.
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(s.poll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := s.repo.GetBankSyncJob(ctx, job.ID)
				if err == nil && (current.State != "running" || current.Attempts != job.Attempts) {
					cancel()
					return
				}
			}
		}
	}()
	result, err := s.execute(ctx, job)
	message := ""
	if err != nil {
		slog.Warn("bank sync failed", "job", job.ID, "subject", job.Subject, "err", err)
		message = "Источник не вернул подходящих актуальных заданий. Повторите обновление позже."
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = "Источник не ответил вовремя. Обновление можно повторить."
		}
	}
	cancel()
	<-watchDone
	// During shutdown leave the lease intact for another worker to recover.
	if parent.Err() != nil {
		return
	}
	finishCtx, finishCancel := context.WithTimeout(parent, 5*time.Second)
	defer finishCancel()
	if err := s.repo.FinishBankSync(finishCtx, job, result, message); err != nil {
		slog.Error("finish bank sync", "job", job.ID, "err", err)
	}
}
