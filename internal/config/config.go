// Package config loads process configuration from the environment. Every
// binary (api, bot, worker, ingest) reads the same shape so docker-compose can
// hand them one env file.
package config

import (
	"os"
	"time"
)

// Config is the union of settings the binaries need; each uses a subset.
type Config struct {
	DatabaseURL         string
	HTTPAddr            string
	RedisAddr           string
	TelegramToken       string
	TelegramBotUsername string
	JWTSecret           string
	FetcherURL          string
	WebURL              string // public URL of the web app (for bot "открыть сайт" buttons); empty = omit
	// MediaPublicURL is a PUBLIC base under which task media keys are directly
	// fetchable (e.g. an exposed MinIO bucket: http://host:9000/egeism-media, or
	// <WebURL>/api/media). Used to inline figures into bot rich messages; empty
	// falls back to WebURL+"/api/media" when WebURL is set.
	MediaPublicURL string
	// AdminUsername/AdminPassword seed the bootstrap admin account created on
	// API startup when no active admin exists (self-registration is gone, so
	// somebody has to be able to log in). Empty password = generate one and
	// print it to the log once.
	AdminUsername string
	AdminPassword string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool

	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment, applying local-dev defaults.
func Load() Config {
	return Config{
		DatabaseURL:         env("DATABASE_URL", "postgres://egeism:egeism@localhost:5432/egeism?sslmode=disable"),
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		RedisAddr:           env("REDIS_ADDR", "localhost:6379"),
		TelegramToken:       env("TELEGRAM_TOKEN", ""),
		TelegramBotUsername: env("TELEGRAM_BOT_USERNAME", ""),
		JWTSecret:           env("JWT_SECRET", "dev-insecure-change-me"),
		FetcherURL:          env("FETCHER_URL", "http://localhost:8090"),
		WebURL:              env("WEB_URL", ""),
		MediaPublicURL:      env("MEDIA_PUBLIC_URL", ""),
		AdminUsername:       env("ADMIN_USERNAME", "admin"),
		AdminPassword:       env("ADMIN_PASSWORD", ""),
		MinIOEndpoint:       env("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:      env("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:      env("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:         env("MINIO_BUCKET", "egeism-media"),
		MinIOUseSSL:         env("MINIO_USE_SSL", "false") == "true",
		ShutdownTimeout:     15 * time.Second,
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
