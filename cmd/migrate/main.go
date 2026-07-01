// Command migrate applies the embedded goose migrations. Run as the init step
// before api/bot/worker (§6 WS-F). Usage: migrate [up|down|status] (default up).
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"egeism/internal/config"
	"egeism/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
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
