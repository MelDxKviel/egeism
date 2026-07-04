package domain

import (
	"time"

	"github.com/google/uuid"
)

// HeatmapCell is one day of activity for the github-style heatmap.
type HeatmapCell struct {
	Day     time.Time `json:"day"`
	Total   int64     `json:"total"`
	Correct int64     `json:"correct"`
}

// NumberMastery is success on one task number (per-task mastery grid).
type NumberMastery struct {
	Number    int   `json:"number"`
	Total     int64 `json:"total"`
	Correct   int64 `json:"correct"`
	AvgTimeMS int64 `json:"avg_time_ms"`
}

// Accuracy returns the correct ratio in [0,1], or 0 when nothing was answered.
func (m NumberMastery) Accuracy() float64 {
	if m.Total == 0 {
		return 0
	}
	return float64(m.Correct) / float64(m.Total)
}

// WeakSpot is a task number the student struggles with.
type WeakSpot struct {
	Number    int     `json:"number"`
	Total     int64   `json:"total"`
	Correct   int64   `json:"correct"`
	AvgTimeMS int64   `json:"avg_time_ms"`
	Accuracy  float64 `json:"accuracy"`
}

// DayAnswer is one answer in a heatmap day drill-down.
type DayAnswer struct {
	AnswerID    uuid.UUID `json:"answer_id"`
	TaskID      uuid.UUID `json:"task_id"`
	Number      int       `json:"number"`
	SubjectID   uuid.UUID `json:"subject_id"`
	RawAnswer   string    `json:"raw_answer"`
	IsCorrect   bool      `json:"is_correct"`
	TimeSpentMS int64     `json:"time_spent_ms"`
	AnsweredAt  time.Time `json:"answered_at"`
}

// AssignmentCard is a scheduled test as shown in "Назначено тебе" and the
// assigned-tests history. Beyond the schedule it carries the result of the
// latest finished attempt for the assignment, so the student can see whether,
// what, and how each assigned test was solved. AttemptID/FinishedAt are nil and
// Correct/Total are 0 until the assignment has been solved at least once.
type AssignmentCard struct {
	ID          uuid.UUID        `json:"id"`
	TestID      uuid.UUID        `json:"test_id"`
	Title       string           `json:"title"`
	Kind        TestKind         `json:"kind"`
	SubjectID   uuid.UUID        `json:"subject_id"`
	ScheduledAt time.Time        `json:"scheduled_at"`
	NotifiedAt  *time.Time       `json:"notified_at,omitempty"`
	Status      AssignmentStatus `json:"status"`
	TaskCount   int64            `json:"task_count"`
	// Result of the latest finished attempt (the assigned test's history).
	AttemptID  *uuid.UUID `json:"attempt_id,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Correct    int64      `json:"correct"`
	Total      int64      `json:"total"`
}

// AttemptSummary is one row of the attempts feed with its score.
type AttemptSummary struct {
	ID         uuid.UUID  `json:"id"`
	TestID     uuid.UUID  `json:"test_id"`
	Title      string     `json:"title"`
	Kind       TestKind   `json:"kind"`
	SubjectID  uuid.UUID  `json:"subject_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Total      int64      `json:"total"`
	Correct    int64      `json:"correct"`
	TimeMS     int64      `json:"time_ms"`
}

// MasteryPoint is one (number, week) success bucket for the mastery line.
type MasteryPoint struct {
	Number  int       `json:"number"`
	Week    time.Time `json:"week"`
	Total   int64     `json:"total"`
	Correct int64     `json:"correct"`
}

// SubjectAccuracy is the overall solved ratio for one subject, the raw input to
// the score forecast (§11 M5): primary score = accuracy × max primary points.
type SubjectAccuracy struct {
	Total    int64   `json:"total"`
	Correct  int64   `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

// ClassNumberStat is one cell of the class mastery grid: how one student does
// on one task number.
type ClassNumberStat struct {
	Number  int   `json:"number"`
	Total   int64 `json:"total"`
	Correct int64 `json:"correct"`
}

// ClassStudentStats is one row of the class overview grid: a member with their
// overall accuracy and the per-number breakdown, so the teacher sees at a
// glance which students lag and which task numbers the class struggles with.
type ClassStudentStats struct {
	StudentID uuid.UUID         `json:"student_id"`
	Name      string            `json:"name"`
	Total     int64             `json:"total"`
	Correct   int64             `json:"correct"`
	ByNumber  []ClassNumberStat `json:"by_number"`
}

// SubjectActivity is one row of the admin's per-subject platform breakdown.
type SubjectActivity struct {
	Code        SubjectCode `json:"code"`
	ActiveTasks int64       `json:"active_tasks"`
	Answers     int64       `json:"answers"`
	Correct     int64       `json:"correct"`
}

// PlatformStats is the admin dashboard payload: platform-wide counts plus the
// per-subject activity breakdown.
type PlatformStats struct {
	Students       int64             `json:"students"`
	Teachers       int64             `json:"teachers"`
	Admins         int64             `json:"admins"`
	InactiveUsers  int64             `json:"inactive_users"`
	Classes        int64             `json:"classes"`
	Tasks          int64             `json:"tasks"`
	ActiveTasks    int64             `json:"active_tasks"`
	Tests          int64             `json:"tests"`
	Assignments    int64             `json:"assignments"`
	Attempts       int64             `json:"attempts"`
	Answers        int64             `json:"answers"`
	CorrectAnswers int64             `json:"correct_answers"`
	Answers7d      int64             `json:"answers_7d"`
	Subjects       []SubjectActivity `json:"subjects"`
}
