// Package scheduler owns asynq task definitions and handlers (§6 WS-D): delayed
// notifications for scheduled tests and periodic streak nudges. Enqueue happens
// at assignment-creation time; a periodic safety-net sweep catches anything the
// enqueue missed (e.g. API couldn't reach Redis).
package scheduler

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task type names (asynq routing keys).
const (
	TypeNotifyAssignment = "assignment:notify"
	TypeStreakNudge      = "student:streak_nudge"
)

// NotifyAssignmentPayload identifies the assignment to notify about.
type NotifyAssignmentPayload struct {
	AssignmentID uuid.UUID `json:"assignment_id"`
}

// NewNotifyAssignmentTask builds a notify task for an assignment.
func NewNotifyAssignmentTask(assignmentID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(NotifyAssignmentPayload{AssignmentID: assignmentID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeNotifyAssignment, payload), nil
}

// NewStreakNudgeTask builds the periodic streak-nudge sweep task.
func NewStreakNudgeTask() *asynq.Task {
	return asynq.NewTask(TypeStreakNudge, nil)
}
