package store

import (
	"context"

	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store/sqlc"
)

// ---- Notifications (in-app, the web bell) ----

// CreateNotification records an in-app notification about an assignment event.
// Recipient: the student for assignment_created, the assigning teacher for
// assignment_done.
func (s *Store) CreateNotification(ctx context.Context, userID uuid.UUID, kind domain.NotificationKind, assignmentID uuid.UUID) error {
	err := s.q.CreateNotification(ctx, sqlc.CreateNotificationParams{
		UserID: userID, Kind: string(kind), AssignmentID: assignmentID,
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// ListNotifications returns a user's notifications, newest first, enriched with
// their assignment/test context.
func (s *Store) ListNotifications(ctx context.Context, userID uuid.UUID, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.q.ListNotificationsForUser(ctx, sqlc.ListNotificationsForUserParams{
		UserID: userID, Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.Notification, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Notification{
			ID:               r.ID,
			Kind:             domain.NotificationKind(r.Kind),
			AssignmentID:     r.AssignmentID,
			TestID:           r.TestID,
			TestTitle:        r.TestTitle,
			SubjectID:        r.SubjectID,
			StudentID:        r.StudentID,
			StudentName:      r.StudentName,
			ScheduledAt:      r.ScheduledAt,
			AssignmentStatus: domain.AssignmentStatus(r.AssignmentStatus),
			ReadAt:           r.ReadAt,
			CreatedAt:        r.CreatedAt,
		})
	}
	return out, nil
}

// CountUnreadNotifications returns the exact unread count for the bell badge
// (the list itself is limit-bounded).
func (s *Store) CountUnreadNotifications(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := s.q.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

// MarkNotificationRead stamps one notification read. The user id scopes the
// update, so a user can't mark someone else's notification; an unknown id
// returns ErrNotFound.
func (s *Store) MarkNotificationRead(ctx context.Context, id, userID uuid.UUID) error {
	n, err := s.q.MarkNotificationRead(ctx, sqlc.MarkNotificationReadParams{ID: id, UserID: userID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead stamps all of a user's unread notifications read.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.q.MarkAllNotificationsRead(ctx, userID); err != nil {
		return mapErr(err)
	}
	return nil
}
