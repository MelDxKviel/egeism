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

// AssignmentCard is a scheduled test as shown in "Назначено тебе".
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
