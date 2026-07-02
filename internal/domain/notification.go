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
)

// Notification is one in-app (web) notification, enriched with its assignment
// context so the bell feed can render it and jump straight to the test.
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
	AssignmentStatus AssignmentStatus `json:"assignment_status"`
	ReadAt           *time.Time       `json:"read_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}
