// Command bot runs the Telegram bot. It is a thin client over the API: it holds
// no database connection and no checking logic (§3).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // embed the zone DB so TZ=Europe/Moscow works in scratch containers

	"egeism/internal/bot"
	"egeism/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	if cfg.TelegramToken == "" {
		slog.Error("TELEGRAM_TOKEN is required")
		os.Exit(1)
	}
	apiBase := os.Getenv("API_BASE_URL")
	if apiBase == "" {
		apiBase = "http://localhost:8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := bot.New(bot.NewAPIClient(apiBase))
	tg := bot.NewTelegram(cfg.TelegramToken, b)

	if err := tg.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("bot stopped", "err", err)
		os.Exit(1)
	}
}
