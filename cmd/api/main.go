// Command api runs the HTTP API server: the single home of business logic that
// both the web app and the Telegram bot call (§3).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"egeism/internal/api"
	"egeism/internal/config"
	"egeism/internal/domain"
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

	// Self-registration is gone, so the first admin must exist before anyone
	// can log in at all — bootstrap it once on startup.
	if err := ensureAdmin(ctx, st, cfg); err != nil {
		slog.Error("bootstrap admin", "err", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewServer(st, enq, cfg.JWTSecret, mediaStore, cfg.FetcherURL, cfg.TelegramBotUsername).Router(),
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

// ensureAdmin creates the bootstrap admin account when no active admin exists
// yet (first run / upgrade from the pre-admin schema). Credentials come from
// ADMIN_USERNAME / ADMIN_PASSWORD; with no password configured a random one is
// generated and printed ONCE to the log — change it in the admin panel.
func ensureAdmin(ctx context.Context, st *store.Store, cfg config.Config) error {
	n, err := st.CountActiveAdmins(ctx)
	if err != nil || n > 0 {
		return err
	}
	username := strings.TrimSpace(strings.ToLower(cfg.AdminUsername))
	if username == "" {
		username = "admin"
	}
	password := cfg.AdminPassword
	generated := false
	if password == "" {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		password = hex.EncodeToString(b)
		generated = true
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin, err := st.CreateUserWithCredentials(ctx, domain.RoleAdmin, "Администратор", username, string(hash), nil)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			slog.Error("bootstrap admin: username is taken by another account — set ADMIN_USERNAME to a free login", "username", username)
			return nil
		}
		return err
	}
	if generated {
		slog.Warn("bootstrap admin created with a GENERATED password — log in and change it",
			"username", username, "password", password, "id", admin.ID)
	} else {
		slog.Info("bootstrap admin created", "username", username, "id", admin.ID)
	}
	return nil
}
