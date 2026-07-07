package domain

import (
	"time"

	"github.com/google/uuid"
)

// NotificationKind says what happened; the web renders the text per kind.
type NotificationKind string

const (
	// NotificationAssignmentCreated — the student got a test assigned to them.
	NotificationAssignmentCreated NotificationKind = "assignment_created"
	// NotificationAssignmentDone — the student completed an assigned test; goes
	// to the teacher who assigned it.
	NotificationAssignmentDone NotificationKind = "assignment_done"
	// NotificationPasswordResetRequested — a user hit «забыл пароль» on the
	// login screen; goes to their teachers (enrollments) and active admins, who
	// can issue a one-hour reset link.
	NotificationPasswordResetRequested NotificationKind = "password_reset_requested"
)

// Notification is one in-app (web) notification, enriched with its context so
// the bell feed can render it and jump straight to the target. Assignment
// kinds carry the assignment/test fields; password_reset_requested carries
// only the subject user (whose password to reset).
type Notification struct {
	ID               uuid.UUID        `json:"id"`
	Kind             NotificationKind `json:"kind"`
	AssignmentID     uuid.UUID        `json:"assignment_id"`
	TestID           uuid.UUID        `json:"test_id"`
	TestTitle        string           `json:"test_title"`
	SubjectID        uuid.UUID        `json:"subject_id"`
	StudentID        uuid.UUID        `json:"student_id"`
	StudentName      string           `json:"student_name"`
	ScheduledAt      time.Time        `json:"scheduled_at"`
	DueAt            *time.Time       `json:"due_at,omitempty"`
	AssignmentStatus AssignmentStatus `json:"assignment_status"`
	SubjectUserID    uuid.UUID        `json:"subject_user_id,omitzero"`
	SubjectUserName  string           `json:"subject_user_name,omitempty"`
	ReadAt           *time.Time       `json:"read_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}
