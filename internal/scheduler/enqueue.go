package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Enqueuer schedules tasks onto the asynq queue. The API holds one to enqueue a
// notification when an assignment is created.
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer connects an asynq client to Redis.
func NewEnqueuer(redisAddr string) *Enqueuer {
	return &Enqueuer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// Close releases the client.
func (e *Enqueuer) Close() error { return e.client.Close() }

// ScheduleAssignmentNotification enqueues a notify task to fire at processAt.
// asynq dedups on the task ID so re-enqueuing the same assignment is safe.
func (e *Enqueuer) ScheduleAssignmentNotification(ctx context.Context, assignmentID uuid.UUID, processAt time.Time) error {
	task, err := NewNotifyAssignmentTask(assignmentID)
	if err != nil {
		return err
	}
	_, err = e.client.EnqueueContext(ctx, task,
		asynq.ProcessAt(processAt),
		asynq.TaskID("notify:"+assignmentID.String()),
		asynq.MaxRetry(5),
	)
	// Re-scheduling an already-queued assignment is not an error.
	if err == asynq.ErrTaskIDConflict {
		return nil
	}
	return err
}
