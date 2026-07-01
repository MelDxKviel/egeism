package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// idleThreshold is how long without activity triggers a streak nudge.
const idleThreshold = 3 * 24 * time.Hour

// Handlers process asynq tasks. It carries the store and a Notifier; it has no
// HTTP surface of its own.
type Handlers struct {
	store    *store.Store
	notifier Notifier
}

// NewHandlers builds task handlers.
func NewHandlers(st *store.Store, n Notifier) *Handlers {
	return &Handlers{store: st, notifier: n}
}

// Register mounts the handlers on an asynq mux.
func (h *Handlers) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeNotifyAssignment, h.handleNotifyAssignment)
	mux.HandleFunc(TypeStreakNudge, h.handleStreakNudge)
}

// handleNotifyAssignment sends the "your test is scheduled" message and stamps
// notified_at. It is idempotent: an already-notified assignment is a no-op.
func (h *Handlers) handleNotifyAssignment(ctx context.Context, task *asynq.Task) error {
	var p NotifyAssignmentPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	a, err := h.store.GetAssignment(ctx, p.AssignmentID)
	if err != nil {
		return err
	}
	if a.NotifiedAt != nil {
		return nil // already delivered
	}
	student, err := h.store.GetUser(ctx, a.StudentID)
	if err != nil {
		return err
	}
	if student.TelegramID == nil {
		slog.Warn("student has no telegram id; skipping notify", "student", student.ID)
		_, _ = h.store.MarkAssignmentNotified(ctx, a.ID)
		return nil
	}
	test, err := h.store.GetTest(ctx, a.TestID)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("📌 Назначен тест «%s» на %s. Удачи!",
		test.Title, a.ScheduledAt.Format("02.01 15:04"))
	if err := h.notifier.Send(ctx, *student.TelegramID, msg); err != nil {
		return err // asynq will retry
	}
	_, err = h.store.MarkAssignmentNotified(ctx, a.ID)
	return err
}

// handleStreakNudge messages students who have gone quiet for idleThreshold.
func (h *Handlers) handleStreakNudge(ctx context.Context, _ *asynq.Task) error {
	idle, err := h.store.IdleStudents(ctx, time.Now().Add(-idleThreshold))
	if err != nil {
		return err
	}
	for _, s := range idle {
		if s.TelegramID == nil {
			continue
		}
		msg := fmt.Sprintf("%s, ты не решал уже несколько дней — не теряй стрик! /solve рус", firstName(s))
		if err := h.notifier.Send(ctx, *s.TelegramID, msg); err != nil {
			slog.Error("streak nudge send", "student", s.ID, "err", err)
		}
	}
	return nil
}

func firstName(s store.IdleStudent) string {
	if s.Name == "" {
		return "Привет"
	}
	return s.Name
}

// SweepDueAssignments is the safety-net: re-enqueue notifications for due,
// un-notified assignments (covers the case where the API couldn't reach Redis
// at creation time). Run periodically from the worker.
func (h *Handlers) SweepDueAssignments(ctx context.Context, enq *Enqueuer) error {
	due, err := h.store.ListDueAssignments(ctx, time.Now())
	if err != nil {
		return err
	}
	for _, a := range due {
		if a.Status != domain.AssignmentScheduled {
			continue
		}
		if err := enq.ScheduleAssignmentNotification(ctx, a.ID, time.Now()); err != nil {
			slog.Error("sweep enqueue", "assignment", a.ID, "err", err)
		}
	}
	return nil
}
