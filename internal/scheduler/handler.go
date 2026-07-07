package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"strings"
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
	webURL   string // public web app URL for the "решать на сайте" button ("" = omit)
}

// NewHandlers builds task handlers. webURL may be empty (no site button then).
func NewHandlers(st *store.Store, n Notifier, webURL string) *Handlers {
	return &Handlers{store: st, notifier: n, webURL: strings.TrimRight(webURL, "/")}
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
	// Enrich the card: subject title + task count (best-effort; the message
	// works without them). ListAssignmentCards already aggregates the count.
	subjectTitle := ""
	if subs, err := h.store.ListSubjects(ctx); err == nil {
		for _, sub := range subs {
			if sub.ID == test.SubjectID {
				subjectTitle = sub.Title
				break
			}
		}
	}
	var taskCount int64
	if cards, err := h.store.ListAssignmentCards(ctx, a.StudentID); err == nil {
		for _, c := range cards {
			if c.ID == a.ID {
				taskCount = c.TaskCount
				break
			}
		}
	}

	var msg strings.Builder
	fmt.Fprintf(&msg, "📌 <b>Тебе назначен тест!</b>\n\n📝 <b>«%s»</b>", html.EscapeString(test.Title))
	if subjectTitle != "" {
		fmt.Fprintf(&msg, "\n📚 %s", html.EscapeString(subjectTitle))
	}
	if taskCount > 0 {
		fmt.Fprintf(&msg, " · %d %s", taskCount, pluralTasks(taskCount))
	}
	// time.Local honors the TZ env (compose sets Europe/Moscow); timestamps from
	// pg are UTC instants and must be shown in the family's local time.
	fmt.Fprintf(&msg, "\n🗓 на %s", a.ScheduledAt.In(time.Local).Format("02.01 в 15:04"))
	if a.DueAt != nil {
		fmt.Fprintf(&msg, "\n⏰ сдать до %s", a.DueAt.In(time.Local).Format("02.01 в 15:04"))
	}
	msg.WriteString("\n\nУдачи! 💪")

	// "Решать тут" is a callback the bot handles (same bot identity as the
	// worker); "На сайте" opens the web app when its URL is configured.
	rows := [][]Button{{{Text: "▶️ Решать тут", Data: "assign:" + a.ID.String()}}}
	if h.webURL != "" {
		rows = append(rows, []Button{{Text: "🌐 Решать на сайте", URL: h.webURL}})
	}
	if err := h.notifier.SendHTML(ctx, *student.TelegramID, msg.String(), rows); err != nil {
		return err // asynq will retry
	}
	_, err = h.store.MarkAssignmentNotified(ctx, a.ID)
	return err
}

// pluralTasks declines «задание» for a count (1 задание, 2 задания, 15 заданий).
func pluralTasks(n int64) string {
	n = n % 100
	if n >= 11 && n <= 14 {
		return "заданий"
	}
	switch n % 10 {
	case 1:
		return "задание"
	case 2, 3, 4:
		return "задания"
	default:
		return "заданий"
	}
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

// SweepOverdueAssignments flips still-scheduled assignments whose deadline
// passed to "missed", so the teacher's roster and the student's feed reflect
// what's overdue. Soft deadline: a missed assignment stays solvable; finishing
// it flips missed → done. Run periodically from the worker alongside the
// notify sweep.
func (h *Handlers) SweepOverdueAssignments(ctx context.Context) error {
	n, err := h.store.MarkOverdueAssignments(ctx, time.Now())
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("marked overdue assignments", "count", n)
	}
	return nil
}
