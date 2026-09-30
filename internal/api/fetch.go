package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"egeism/internal/domain"
	"egeism/internal/ingest"
)

type fetchTasksReq struct {
	Subject domain.SubjectCode `json:"subject"`
	Limit   int                `json:"limit"`
	Active  bool               `json:"active"` // skip curation, ingest as active
}

// handleFetchTasks queues a source pull and returns its durable progress record.
// Downloading conditions/media and ingestion belong to the background worker.
func (s *Server) handleFetchTasks(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	if s.fetcherURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "источник заданий не настроен")
		return
	}
	var req fetchTasksReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Subject != domain.SubjectRus && req.Subject != domain.SubjectMath &&
		req.Subject != domain.SubjectInf && req.Subject != domain.SubjectSoc {
		writeErr(w, http.StatusBadRequest, "unknown subject")
		return
	}
	if !s.subjectInScope(w, teacher, req.Subject) {
		return
	}
	if req.Limit <= 0 {
		req.Limit = 30
	}
	if req.Limit > 200 {
		req.Limit = 200
	}

	job, err := s.enqueueBankSync(r.Context(), req.Subject, 0, req.Limit, req.Active, false)
	s.writeBankSync(w, r, job, err)
}

// fetchAndIngest pulls tasks from the source (optionally for one number) and
// runs them through ingest (media → MinIO, dedup), within a background job.
func (s *Server) fetchAndIngest(ctx context.Context, subject domain.SubjectCode, limit, number int, status domain.TaskStatus) (ingest.Result, string, error) {
	if s.fetcherURL == "" {
		return ingest.Result{}, "", fmt.Errorf("источник заданий не настроен")
	}
	raws, mode, err := s.callFetcher(ctx, subject, limit, number)
	if err != nil {
		return ingest.Result{}, "", err
	}
	res, err := s.ingestRaws(ctx, subject, raws, status)
	return res, mode, err
}

// ingestRaws runs already-fetched RawTasks through the shared ingest pipeline
// (media → MinIO, dedup, source eligibility, status).
func (s *Server) ingestRaws(ctx context.Context, subject domain.SubjectCode, raws []ingest.RawTask, status domain.TaskStatus) (ingest.Result, error) {
	runner := ingest.NewRunner(s.store)
	if s.media != nil {
		runner.WithMedia(s.media)
	}
	runner.Status = status
	return runner.Ingest(ctx, "fetch:"+string(subject), raws)
}

// fetchResp is the ingest result plus the fetch mode reported by the fetcher
// (always "real": openfipi/РЕШУ — there is no mock source).
type fetchResp struct {
	Fetched  int    `json:"fetched"`
	Inserted int    `json:"inserted"`
	Skipped  int    `json:"skipped"`
	Invalid  int    `json:"invalid"`
	Promoted int    `json:"promoted"` // dedup hits promoted draft → active
	Held     int    `json:"held"`
	Source   string `json:"source"`
}

// callFetcherByIDs asks the fetcher to re-fetch specific РЕШУ problem ids (the
// upgrade path), returning refreshed RawTasks (statement + inline media).
func (s *Server) callFetcherByIDs(ctx context.Context, subject domain.SubjectCode, ids []string, provider ...string) ([]ingest.RawTask, error) {
	payload := map[string]any{"subject": subject, "ids": ids}
	if len(provider) > 0 {
		payload["provider"] = provider[0]
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.fetcherURL+"/fetch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetcher %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}
	var raws []ingest.RawTask
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode fetcher response: %w", err)
	}
	return raws, nil
}

// callFetcher POSTs to the fetcher service and decodes the normalized tasks.
// It also returns the fetch mode from the X-Fetch-Mode header (always "real").
func (s *Server) callFetcher(ctx context.Context, subject domain.SubjectCode, limit, number int) ([]ingest.RawTask, string, error) {
	body, _ := json.Marshal(map[string]any{
		"subject": subject, "limit": limit, "number": number, "min_confidence": 0.5,
	})
	// Bounded so a hung fetcher doesn't spin the button forever; the fetcher
	// itself returns [] under its own deadline when the source is unreachable.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.fetcherURL+"/fetch", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetcher %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}
	var raws []ingest.RawTask
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, "", fmt.Errorf("decode fetcher response: %w", err)
	}
	return raws, resp.Header.Get("X-Fetch-Mode"), nil
}
