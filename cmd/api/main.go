// Command api runs the HTTP API server: the single home of business logic that
// both the web app and the Telegram bot call (§3).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"egeism/internal/api"
	"egeism/internal/config"
	"egeism/internal/media"
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

	// The enqueuer lets the API schedule assignment notifications. It connects
	// lazily; if Redis is down the worker's sweep is the safety net.
	enq := scheduler.NewEnqueuer(cfg.RedisAddr)
	defer enq.Close()

	// Media store is best-effort: if MinIO is unreachable the API still runs,
	// just without serving task media.
	mediaStore, err := media.New(ctx, cfg)
	if err != nil {
		slog.Warn("media store unavailable; task media disabled", "err", err)
		mediaStore = nil
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewServer(st, enq, cfg.JWTSecret, mediaStore, cfg.FetcherURL, cfg.TelegramBotUsername, cfg.AllowRegistration).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown", "err", err)
	}
}
