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
		UserID: userID, Kind: string(kind), AssignmentID: &assignmentID,
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// CreatePasswordResetNotification records «subjectUser забыл пароль» for one
// recipient (a teacher of the student or an admin). Idempotent while unread:
// a pending unread notification about the same user suppresses duplicates, so
// button-mashing «забыл пароль» never floods the bell.
func (s *Store) CreatePasswordResetNotification(ctx context.Context, recipientID, subjectUserID uuid.UUID) error {
	err := s.q.CreatePasswordResetNotification(ctx, sqlc.CreatePasswordResetNotificationParams{
		UserID: recipientID, SubjectUserID: &subjectUserID,
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// ListNotifications returns a user's notifications, newest first, enriched with
// their assignment/test context (or the subject user for password resets).
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
		// The assignment context is LEFT-JOINed (absent on password resets);
		// missing pieces collapse to zero values the JSON encoder omits or the
		// web ignores — clients key on Kind before touching them.
		n := domain.Notification{
			ID:        r.ID,
			Kind:      domain.NotificationKind(r.Kind),
			ReadAt:    r.ReadAt,
			CreatedAt: r.CreatedAt,
		}
		if r.AssignmentID != nil {
			n.AssignmentID = *r.AssignmentID
		}
		if r.TestID != nil {
			n.TestID = *r.TestID
		}
		if r.TestTitle != nil {
			n.TestTitle = *r.TestTitle
		}
		if r.SubjectID != nil {
			n.SubjectID = *r.SubjectID
		}
		if r.StudentID != nil {
			n.StudentID = *r.StudentID
		}
		if r.StudentName != nil {
			n.StudentName = *r.StudentName
		}
		if r.ScheduledAt != nil {
			n.ScheduledAt = *r.ScheduledAt
		}
		if r.AssignmentStatus != nil {
			n.AssignmentStatus = domain.AssignmentStatus(*r.AssignmentStatus)
		}
		if r.SubjectUserID != nil {
			n.SubjectUserID = *r.SubjectUserID
		}
		if r.SubjectUserName != nil {
			n.SubjectUserName = *r.SubjectUserName
		}
		out = append(out, n)
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
