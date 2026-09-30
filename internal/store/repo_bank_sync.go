package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

var ErrBankQueueFull = errors.New("bank sync queue is full")

// EnqueueBankSync deduplicates pending work and rate-limits completed attempts
// across users AND API replicas. The short transaction never contacts a source.
func (s *Store) EnqueueBankSync(ctx context.Context, subject domain.SubjectCode, number, limit int, active bool, cooldown time.Duration, kind string) (domain.BankSyncJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.BankSyncJob{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize queue bookkeeping only; the lock is released before source I/O.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(69138421)`); err != nil {
		return domain.BankSyncJob{}, err
	}
	q := s.q.WithTx(tx)
	row, err := q.RecentBankSyncJob(ctx, sqlc.RecentBankSyncJobParams{Subject: string(subject), Number: int32(number), Active: active, CreatedAt: time.Now().Add(-cooldown), Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		pending, countErr := q.CountPendingBankSyncJobs(ctx)
		if countErr != nil {
			return domain.BankSyncJob{}, countErr
		}
		if pending >= 32 {
			return domain.BankSyncJob{}, ErrBankQueueFull
		}
		row, err = q.InsertBankSyncJob(ctx, sqlc.InsertBankSyncJobParams{Subject: string(subject), Number: int32(number), TaskLimit: int32(limit), Active: active, Kind: kind})
	}
	if err != nil {
		return domain.BankSyncJob{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.BankSyncJob{}, err
	}
	return toBankSyncJob(row), nil
}

func (s *Store) GetBankSyncJob(ctx context.Context, id uuid.UUID) (domain.BankSyncJob, error) {
	row, err := s.q.GetBankSyncJob(ctx, id)
	return toBankSyncJob(row), mapErr(err)
}

// ClaimBankSync allows at most two source jobs globally, and at most one per
// subject to keep ingest dedup safe. Expired leases recover after a crash.
func (s *Store) ClaimBankSync(ctx context.Context) (domain.BankSyncJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.BankSyncJob{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(69138421)`); err != nil {
		return domain.BankSyncJob{}, err
	}
	q := s.q.WithTx(tx)
	if err = q.ReapBankSyncJobs(ctx); err != nil {
		return domain.BankSyncJob{}, err
	}
	row, err := q.ClaimBankSyncJob(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.BankSyncJob{}, err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return domain.BankSyncJob{}, commitErr
	}
	return toBankSyncJob(row), mapErr(err)
}

func (s *Store) FinishBankSync(ctx context.Context, job domain.BankSyncJob, result *domain.BankSyncResult, message string) error {
	state := "succeeded"
	if message != "" {
		state = "failed"
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.q.FinishBankSyncJob(ctx, sqlc.FinishBankSyncJobParams{ID: job.ID, Attempts: int32(job.Attempts), State: state, Result: data, Error: message})
}

func (s *Store) CancelBankSync(ctx context.Context, id uuid.UUID) (domain.BankSyncJob, error) {
	row, err := s.q.CancelBankSyncJob(ctx, id)
	return toBankSyncJob(row), mapErr(err)
}

func (s *Store) PruneBankSync(ctx context.Context) error { return s.q.PruneBankSyncJobs(ctx) }

func toBankSyncJob(row sqlc.BankSyncJob) domain.BankSyncJob {
	j := domain.BankSyncJob{ID: row.ID, Subject: domain.SubjectCode(row.Subject), Number: int(row.Number), Limit: int(row.TaskLimit), Active: row.Active, Kind: row.Kind, State: row.State, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Error: row.Error, Attempts: int(row.Attempts)}
	if len(row.Result) > 0 {
		_ = json.Unmarshal(row.Result, &j.Result)
	}
	return j
}
