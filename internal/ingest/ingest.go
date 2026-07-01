// Package ingest is the content pipeline (§6 WS-E, §9). It is a fully isolated
// adapter whose only contract with the rest of the system is: write rows into
// tasks with status=draft and a filled answer_schema. Everything else is
// indifferent to where tasks come from, so swapping sources touches only this
// package. Ingested tasks are never live until a human curates them in the
// admin (status -> active).
package ingest

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	"egeism/internal/domain"
	"egeism/internal/media"
	"egeism/internal/store"
)

// RawMedia is a media reference from a source: a URL to download plus metadata.
// The runner fetches it into MinIO and rewrites it to a stored object key.
type RawMedia struct {
	URL    string `json:"url"`
	Kind   string `json:"kind"` // image | table | file
	Alt    string `json:"alt,omitempty"`
	Inline bool   `json:"inline,omitempty"` // inline formula (⟦img:N⟧ placeholder in statement)
}

// RawTask is a normalized candidate from a source, pre-persistence.
type RawTask struct {
	Subject      domain.SubjectCode  `json:"subject"`
	Number       int                 `json:"number"`
	Statement    string              `json:"statement"`
	Media        []RawMedia          `json:"media,omitempty"`
	AnswerSchema domain.AnswerSchema `json:"answer_schema"`
	Source       domain.Source       `json:"source"`
}

// Source produces normalized candidate tasks. Implementations: a dataset file,
// an open-bank scraper, an LLM-assisted OCR pipeline, etc. (§9).
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]RawTask, error)
}

// Result summarizes an ingest run.
type Result struct {
	Fetched  int `json:"fetched"`
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"` // duplicates
	Invalid  int `json:"invalid"` // failed validation
}

// Runner writes candidates into the store with dedup and validation.
type Runner struct {
	store *store.Store
	media *media.Store // optional: downloads source media into MinIO
	// Status is the status new tasks get. Default (empty) = draft, so
	// auto-ingested content is curated before going live. Seed/demo runs may
	// set it to active.
	Status domain.TaskStatus
}

// NewRunner builds a Runner over the store (new tasks default to draft).
func NewRunner(st *store.Store) *Runner { return &Runner{store: st} }

// WithMedia attaches a MinIO store so source images/files are downloaded and
// stored; tasks then reference them by object key.
func (r *Runner) WithMedia(m *media.Store) *Runner { r.media = m; return r }

// Run fetches from src and inserts new, valid tasks.
func (r *Runner) Run(ctx context.Context, src Source) (Result, error) {
	raws, err := src.Fetch(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("fetch from %s: %w", src.Name(), err)
	}
	return r.Ingest(ctx, src.Name(), raws)
}

// Ingest inserts new, valid tasks from an in-memory list. Used by the CLI (via
// Run) and by the API's teacher upload endpoint. Dedup/validation as usual.
func (r *Runner) Ingest(ctx context.Context, sourceName string, raws []RawTask) (Result, error) {
	res := Result{Fetched: len(raws)}
	for _, raw := range raws {
		if err := r.ingestOne(ctx, sourceName, raw); err != nil {
			switch {
			case err == errDuplicate:
				res.Skipped++
			case err == errInvalid:
				res.Invalid++
			default:
				return res, err
			}
			continue
		}
		res.Inserted++
	}
	slog.Info("ingest complete", "source", sourceName,
		"fetched", res.Fetched, "inserted", res.Inserted, "skipped", res.Skipped, "invalid", res.Invalid)
	return res, nil
}

var (
	errDuplicate = fmt.Errorf("duplicate")
	errInvalid   = fmt.Errorf("invalid")
)

func (r *Runner) ingestOne(ctx context.Context, sourceName string, raw RawTask) error {
	if err := raw.AnswerSchema.Validate(); err != nil {
		slog.Warn("ingest: invalid answer schema", "source", sourceName, "err", err)
		return errInvalid
	}
	src := raw.Source
	if src.Provider == "" {
		src.Provider = sourceName
	}
	if src.ExternID == "" {
		src.ExternID = externID(raw)
	}
	exists, err := r.store.TaskExistsBySource(ctx, src.Provider, src.ExternID)
	if err != nil {
		return err
	}
	if exists {
		return errDuplicate
	}
	sub, err := r.store.GetSubjectByCode(ctx, raw.Subject)
	if err != nil {
		return fmt.Errorf("unknown subject %q: %w", raw.Subject, err)
	}
	status := r.Status
	if status == "" {
		status = domain.TaskDraft // default: curated to active in the admin
	}
	_, err = r.store.CreateTask(ctx, domain.Task{
		SubjectID:    sub.ID,
		Number:       raw.Number,
		Statement:    raw.Statement,
		Media:        r.resolveMedia(ctx, raw.Media),
		AnswerSchema: raw.AnswerSchema,
		Source:       &src,
		Status:       status,
	})
	return err
}

// resolveMedia downloads each source media URL into MinIO and returns stored
// references. Without a media store (or for a non-http key), the URL is kept
// as-is so nothing is lost — but images won't be served until re-ingested.
func (r *Runner) resolveMedia(ctx context.Context, raws []RawMedia) []domain.Media {
	if len(raws) == 0 {
		return nil
	}
	out := make([]domain.Media, 0, len(raws))
	for _, m := range raws {
		key := m.URL
		if r.media != nil {
			if strings.HasPrefix(m.URL, "data:") {
				// Inline base64 media (e.g. some FIPI images openfipi inlines).
				// Store it in MinIO; if it can't be decoded/stored, drop it
				// rather than persist a huge data: URI as the media key.
				data, ct, ext, derr := decodeDataURL(m.URL)
				if derr != nil {
					slog.Warn("ingest: bad data URL, dropping media", "err", derr)
					continue
				}
				if k, err := r.media.Put(ctx, data, ct, "media"+ext); err != nil {
					slog.Warn("ingest: inline media upload failed, dropping", "err", err)
					continue
				} else {
					key = k
				}
			} else if strings.HasPrefix(m.URL, "http") {
				if k, err := r.media.PutFromURL(ctx, m.URL); err != nil {
					slog.Warn("ingest: media download failed", "url", m.URL, "err", err)
				} else {
					key = k
				}
			} else if data, err := os.ReadFile(m.URL); err == nil {
				// Local file (e.g. a bundled seed image): upload it to MinIO.
				if k, err := r.media.Put(ctx, data, contentTypeByExt(m.URL), m.URL); err != nil {
					slog.Warn("ingest: media upload failed", "path", m.URL, "err", err)
				} else {
					key = k
				}
			}
		}
		out = append(out, domain.Media{Key: key, Kind: m.Kind, Alt: m.Alt, Inline: m.Inline})
	}
	return out
}

// decodeDataURL parses a base64 "data:<mime>;base64,<payload>" URL into its
// bytes, content-type, and a file extension for the object key. Only base64
// data URLs are supported (that's what our sources emit).
func decodeDataURL(u string) (data []byte, contentType, ext string, err error) {
	const p = "data:"
	if !strings.HasPrefix(u, p) {
		return nil, "", "", fmt.Errorf("not a data URL")
	}
	meta, payload, ok := strings.Cut(u[len(p):], ",")
	if !ok || !strings.Contains(meta, "base64") {
		return nil, "", "", fmt.Errorf("unsupported data URL (want base64)")
	}
	data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return nil, "", "", fmt.Errorf("data URL base64: %w", err)
	}
	contentType, _, _ = strings.Cut(meta, ";")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/svg+xml":
		ext = ".svg"
	}
	return data, contentType, ext, nil
}

// contentTypeByExt is a tiny mime guess for locally-read media.
func contentTypeByExt(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// externID derives a stable id when the source didn't supply one, so re-runs
// dedup on content.
func externID(raw RawTask) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%s", raw.Subject, raw.Number, raw.Statement)))
	return hex.EncodeToString(h[:])
}
