// Command ingest runs a content-ingest source and writes draft tasks (§6 WS-E).
//
// Usage:
//
//	ingest -source file -provider dataset-2026 -path deploy/seed/tasks.sample.json
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"egeism/internal/config"
	"egeism/internal/domain"
	"egeism/internal/ingest"
	"egeism/internal/media"
	"egeism/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	var (
		sourceKind = flag.String("source", "dataset", "source kind: dataset | file")
		provider   = flag.String("provider", "dataset", "provider label stored in tasks.source")
		path       = flag.String("path", "", "path to a local source file (JSON array or JSONL)")
		url        = flag.String("url", "", "http(s) URL of a dataset (JSON array or JSONL)")
		status     = flag.String("status", "draft", "status for new tasks: draft | active | rejected")
	)
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	var src ingest.Source
	switch *sourceKind {
	case "dataset":
		loc := *url
		if loc == "" {
			loc = *path
		}
		if loc == "" {
			slog.Error("-url or -path is required for -source dataset")
			os.Exit(2)
		}
		src = ingest.NewDatasetSource(*provider, loc)
	case "file":
		if *path == "" {
			slog.Error("-path is required for -source file")
			os.Exit(2)
		}
		src = ingest.NewFileSource(*provider, *path)
	default:
		slog.Error("unknown source kind", "kind", *sourceKind)
		os.Exit(2)
	}

	runner := ingest.NewRunner(st)
	// Attach MinIO so task images/files are downloaded and stored. Best-effort:
	// if MinIO is down, ingest still runs (media URLs kept as-is).
	if ms, err := media.New(ctx, cfg); err != nil {
		slog.Warn("media store unavailable; media will not be downloaded", "err", err)
	} else {
		runner.WithMedia(ms)
	}
	switch domain.TaskStatus(*status) {
	case domain.TaskDraft, domain.TaskActive, domain.TaskRejected:
		runner.Status = domain.TaskStatus(*status)
	default:
		slog.Error("invalid -status", "status", *status)
		os.Exit(2)
	}

	res, err := runner.Run(ctx, src)
	if err != nil {
		slog.Error("ingest failed", "err", err)
		os.Exit(1)
	}
	slog.Info("done", "fetched", res.Fetched, "inserted", res.Inserted, "skipped", res.Skipped, "invalid", res.Invalid)
}
