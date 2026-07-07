package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role is a user role. Roles are split now even though the first stage has one
// student and one teacher; the data model is multi-user from day one.
type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	// RoleAdmin manages accounts (create/activate/deactivate/delete, change
	// roles) and watches platform-wide stats. Self-registration is gone: only
	// an admin (any role) or a teacher (students) creates users.
	RoleAdmin Role = "admin"
)

// SubjectCode identifies one of the four supported subjects.
type SubjectCode string

const (
	SubjectRus  SubjectCode = "rus"  // русский
	SubjectMath SubjectCode = "math" // математика
	SubjectInf  SubjectCode = "inf"  // информатика
	SubjectSoc  SubjectCode = "soc"  // обществознание
)

// TaskStatus is the curation state of an auto-ingested task. Only active tasks
// go into tests; draft/rejected are held back for review.
type TaskStatus string

const (
	TaskDraft    TaskStatus = "draft"
	TaskActive   TaskStatus = "active"
	TaskRejected TaskStatus = "rejected"
)

// TestKind distinguishes an exam-like test from a single-number drill.
type TestKind string

const (
	TestClassic TestKind = "classic" // как на ЕГЭ
	TestDrill   TestKind = "drill"   // N задач одного номера
)

// AssignmentStatus tracks a teacher-scheduled test through its lifecycle.
type AssignmentStatus string

const (
	AssignmentScheduled AssignmentStatus = "scheduled"
	AssignmentDone      AssignmentStatus = "done"
	AssignmentMissed    AssignmentStatus = "missed"
)

// User is a person in the system. Web users log in with username+password; the
// bot authenticates by telegram_id. The password hash is never part of this
// type, so it can't leak through the API.
type User struct {
	ID         uuid.UUID `json:"id"`
	Role       Role      `json:"role"`
	TelegramID *int64    `json:"telegram_id,omitempty"`
	Username   *string   `json:"username,omitempty"`
	Name       string    `json:"name"`
	// Subject scopes a teacher to one exam subject; nil on a teacher means
	// "сверхучитель" (may work with any subject). Always nil for other roles.
	Subject *SubjectCode `json:"subject,omitempty"`
	// IsActive gates every authenticated action: a deactivated account keeps
	// its history but can't log in (web, bot) until an admin re-enables it.
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// Class is a teacher's group of students. Students may also stay classless
// (репетитор case); membership lives in class_members.
type Class struct {
	ID        uuid.UUID `json:"id"`
	TeacherID uuid.UUID `json:"teacher_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// Enriched at read time (list queries); zero-valued on bare rows.
	MemberCount int64  `json:"member_count"`
	TeacherName string `json:"teacher_name,omitempty"`
}

// Enrollment links a teacher to a student (m2m, one row for now).
type Enrollment struct {
	ID        uuid.UUID `json:"id"`
	TeacherID uuid.UUID `json:"teacher_id"`
	StudentID uuid.UUID `json:"student_id"`
}

// Subject is one exam subject.
type Subject struct {
	ID    uuid.UUID   `json:"id"`
	Code  SubjectCode `json:"code"`
	Title string      `json:"title"`
}

// Media is a reference to an object stored in MinIO (image/table/file).
type Media struct {
	Key  string `json:"key"`           // object key in the bucket
	Kind string `json:"kind"`          // "image" | "table" | "file"
	Alt  string `json:"alt,omitempty"` // accessibility / fallback text
	// Inline marks a formula/symbol rendered mid-sentence (РЕШУ's <img class=tex>).
	// The statement carries a ⟦img:N⟧ placeholder at its spot; the web swaps it
	// for a small baseline-aligned image. Block figures (Inline=false) render
	// under the statement as usual.
	Inline bool `json:"inline,omitempty"`
}

// Source records where a task came from, for ingest dedup and provenance.
type Source struct {
	Provider string `json:"provider"`  // e.g. "fipi", "sdamgia", "dataset"
	ExternID string `json:"extern_id"` // stable id at the provider
	URL      string `json:"url,omitempty"`
}

// Task is a single part-1 exercise.
type Task struct {
	ID           uuid.UUID    `json:"id"`
	SubjectID    uuid.UUID    `json:"subject_id"`
	Number       int          `json:"number"` // номер задания в ЕГЭ
	Statement    string       `json:"statement"`
	Media        []Media      `json:"media,omitempty"`
	AnswerSchema AnswerSchema `json:"answer_schema"`
	Source       *Source      `json:"source,omitempty"`
	Status       TaskStatus   `json:"status"`
	CreatedAt    time.Time    `json:"created_at"`
}

// BotSolvable reports whether the task can be solved inside a chat: short
// answers with no attached media (§8). Everything else is web-only.
func (t Task) BotSolvable() bool {
	if len(t.Media) > 0 {
		return false
	}
	switch t.AnswerSchema.Type {
	case AnswerNumber, AnswerString, AnswerSet, AnswerSequence:
		return true
	default:
		return false
	}
}

// Test is a named collection of tasks (classic exam layout or a drill).
type Test struct {
	ID        uuid.UUID `json:"id"`
	SubjectID uuid.UUID `json:"subject_id"`
	Kind      TestKind  `json:"kind"`
	Title     string    `json:"title"`
	CreatedBy uuid.UUID `json:"created_by"`
	// VariantOf marks a per-student clone generated for a class assignment
	// («каждому свой вариант»): it points at the source test and keeps the
	// clone out of the teacher's test library.
	VariantOf *uuid.UUID `json:"variant_of,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// TestItem places a task at a position within a test.
type TestItem struct {
	ID       uuid.UUID `json:"id"`
	TestID   uuid.UUID `json:"test_id"`
	TaskID   uuid.UUID `json:"task_id"`
	Position int       `json:"position"`
}

// Assignment is a teacher scheduling a test for a student at a time.
// ScheduledAt is when the student is notified/the work is set; DueAt is the
// optional deadline (NULL = no deadline, self-paced). The deadline is soft:
// passing it flips a still-unsolved assignment to "missed", but the student
// can still solve it late (finish then flips missed → done).
type Assignment struct {
	ID          uuid.UUID        `json:"id"`
	TestID      uuid.UUID        `json:"test_id"`
	StudentID   uuid.UUID        `json:"student_id"`
	AssignedBy  uuid.UUID        `json:"assigned_by"`
	ScheduledAt time.Time        `json:"scheduled_at"`
	NotifiedAt  *time.Time       `json:"notified_at,omitempty"`
	Status      AssignmentStatus `json:"status"`
	DueAt       *time.Time       `json:"due_at,omitempty"`
}

// Attempt is a student working through a test. AssignmentID is nullable so a
// student can practice on their own with no assignment.
type Attempt struct {
	ID           uuid.UUID  `json:"id"`
	AssignmentID *uuid.UUID `json:"assignment_id,omitempty"`
	TestID       uuid.UUID  `json:"test_id"`
	StudentID    uuid.UUID  `json:"student_id"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Answer is one submitted response, already checked. is_correct plus
// time_spent_ms feeds heatmap, weak-spots and per-task timing.
type Answer struct {
	ID          uuid.UUID `json:"id"`
	AttemptID   uuid.UUID `json:"attempt_id"`
	TaskID      uuid.UUID `json:"task_id"`
	RawAnswer   string    `json:"raw_answer"`
	IsCorrect   bool      `json:"is_correct"`
	TimeSpentMS int64     `json:"time_spent_ms"`
	AnsweredAt  time.Time `json:"answered_at"`
}
