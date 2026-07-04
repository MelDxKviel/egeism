package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// ---- Password reset (teacher/admin-issued one-time links) ----

// CreatePasswordResetToken issues a one-time reset token for userID, recording
// who issued it. The token expires after ttl (1 hour in the API); an expired
// link means issuing a fresh one.
func (s *Store) CreatePasswordResetToken(ctx context.Context, userID, createdBy uuid.UUID, ttl time.Duration) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(ttl)
	if _, err := s.q.CreatePasswordResetToken(ctx, sqlc.CreatePasswordResetTokenParams{
		Token: token, UserID: userID, CreatedBy: &createdBy, ExpiresAt: expires,
	}); err != nil {
		return "", time.Time{}, mapErr(err)
	}
	return token, expires, nil
}

// PeekPasswordResetToken loads the account a still-valid token belongs to, so
// the reset page can greet the user before they type a password. Returns
// ErrNotFound for an unknown/expired/used token.
func (s *Store) PeekPasswordResetToken(ctx context.Context, token string) (domain.User, time.Time, error) {
	prt, err := s.q.GetValidPasswordResetToken(ctx, token)
	if err != nil {
		return domain.User{}, time.Time{}, mapErr(err)
	}
	u, err := s.q.GetUser(ctx, prt.UserID)
	if err != nil {
		return domain.User{}, time.Time{}, mapErr(err)
	}
	return toDomainUser(u), prt.ExpiresAt, nil
}

// RedeemPasswordResetToken validates a reset token and swaps the account's
// password hash in one transaction, stamping the token used so it can never be
// replayed. Returns ErrNotFound for an unknown/expired/used token.
func (s *Store) RedeemPasswordResetToken(ctx context.Context, token, passwordHash string) (domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	prt, err := qtx.GetValidPasswordResetToken(ctx, token)
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	n, err := qtx.SetUserPassword(ctx, sqlc.SetUserPasswordParams{ID: prt.UserID, PasswordHash: &passwordHash})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	if n == 0 {
		return domain.User{}, ErrNotFound
	}
	if err := qtx.MarkPasswordResetTokenUsed(ctx, token); err != nil {
		return domain.User{}, mapErr(err)
	}
	u, err := qtx.GetUser(ctx, prt.UserID)
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return toDomainUser(u), nil
}

// ListTeacherIDsForStudent returns the teachers enrolled with a student — the
// recipients (besides admins) of the student's «забыл пароль» notification.
func (s *Store) ListTeacherIDsForStudent(ctx context.Context, studentID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.q.ListTeacherIDsForStudent(ctx, studentID)
	if err != nil {
		return nil, mapErr(err)
	}
	return ids, nil
}

// ListActiveAdminIDs returns every enabled admin account id.
func (s *Store) ListActiveAdminIDs(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := s.q.ListActiveAdminIDs(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	return ids, nil
}

// randomToken returns a 32-hex-char one-time token. Unlike the short Telegram
// link codes (typed by hand into a chat), reset tokens ride in a URL, so they
// can afford to be long enough to be unguessable outright.
func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
