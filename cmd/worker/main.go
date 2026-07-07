// Command worker runs the asynq worker: it processes delayed assignment
// notifications, runs the periodic streak-nudge, and sweeps due assignments as
// a safety net (§6 WS-D).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed the zone DB so TZ=Europe/Moscow works in scratch containers

	"github.com/hibiken/asynq"

	"egeism/internal/config"
	"egeism/internal/scheduler"
	"egeism/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	var notifier scheduler.Notifier
	if cfg.TelegramToken != "" {
		notifier = scheduler.NewTelegramNotifier(cfg.TelegramToken)
	} else {
		slog.Warn("no TELEGRAM_TOKEN; notifications will be logged only")
		notifier = scheduler.LogNotifier{Log: func(chatID int64, text string) {
			slog.Info("notify (log)", "chat", chatID, "text", text)
		}}
	}

	handlers := scheduler.NewHandlers(st, notifier, cfg.WebURL)
	enq := scheduler.NewEnqueuer(cfg.RedisAddr)
	defer enq.Close()

	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}

	// Periodic tasks: streak nudge once a day.
	asynqScheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := asynqScheduler.Register("0 10 * * *", scheduler.NewStreakNudgeTask()); err != nil {
		slog.Error("register streak nudge", "err", err)
	}
	go func() {
		if err := asynqScheduler.Run(); err != nil {
			slog.Error("asynq scheduler", "err", err)
		}
	}()

	// Safety-net sweep of due assignments every minute.
	go sweepLoop(ctx, handlers, enq)

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 5})
	mux := asynq.NewServeMux()
	handlers.Register(mux)

	slog.Info("worker started")
	go func() {
		if err := srv.Run(mux); err != nil {
			slog.Error("asynq server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("worker shutting down")
	srv.Shutdown()
	asynqScheduler.Shutdown()
}

func sweepLoop(ctx context.Context, h *scheduler.Handlers, enq *scheduler.Enqueuer) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.SweepDueAssignments(ctx, enq); err != nil {
				slog.Error("sweep due assignments", "err", err)
			}
			if err := h.SweepOverdueAssignments(ctx); err != nil {
				slog.Error("sweep overdue assignments", "err", err)
			}
		}
	}
}
