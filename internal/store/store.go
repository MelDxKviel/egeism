// Package store is the data-access layer. It wraps the sqlc-generated queries
// behind a small, domain-typed API so the rest of the app never sees []byte
// JSONB blobs or sqlc row structs. Phase 0 freezes these signatures; feature
// work adds queries without changing the shape callers depend on.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"egeism/internal/store/sqlc"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// ErrInUse is returned when a delete is refused because the row is still
// referenced by protected data (e.g. a test that has been attempted).
var ErrInUse = errors.New("in use")

// ErrTelegramTaken is returned when a Telegram id is already linked to another
// account (users.telegram_id UNIQUE violation on link).
var ErrTelegramTaken = errors.New("telegram already linked")

// ErrUsernameTaken is returned when a username is already registered
// (users.username UNIQUE violation on account creation).
var ErrUsernameTaken = errors.New("username already taken")

// Store is the concrete data-access layer backed by a pgx pool.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// New connects to Postgres and returns a Store. The caller owns Close.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool, q: sqlc.New(pool)}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// Pool exposes the underlying pool for health checks and transactions.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping verifies the database is reachable (used by /health).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// mapErr converts pgx's no-rows sentinel into the package-level ErrNotFound.
func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation, so callers can turn a race on a UNIQUE column into a friendly error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isForeignKeyViolation reports whether err is a Postgres foreign-key (23503)
// violation — e.g. deleting a user who still owns attempts or tests.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
