// Command migrate applies the embedded goose migrations. Run as the init step
// before api/bot/worker (§6 WS-F). Usage: migrate [up|down|status|init-storage]
// (default up). init-storage provisions MinIO without a database connection.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"egeism/internal/config"
	"egeism/internal/media"
	"egeism/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "init-storage" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := media.InitBuckets(ctx, cfg); err != nil {
			slog.Error("initialize storage", "err", err)
			os.Exit(1)
		}
		slog.Info("storage buckets initialized")
		return
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("set dialect", "err", err)
		os.Exit(1)
	}
	if err := goose.RunContext(context.Background(), command, db, "."); err != nil {
		slog.Error("migrate", "cmd", command, "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied", "cmd", command)
}
