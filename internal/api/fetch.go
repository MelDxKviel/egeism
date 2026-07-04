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

// handleFetchTasks is the button-driven pull: the teacher picks a subject and
// count in the bank UI, and the server pulls tasks from the source (via the
// fetcher service, which wraps РЕШУ/sdamgia) and runs them through the same
// ingest as everything else — media into MinIO, dedup, draft for curation.
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

	status := domain.TaskDraft
	if req.Active {
		status = domain.TaskActive
	}
	res, mode, err := s.fetchAndIngest(r.Context(), req.Subject, req.Limit, 0, status)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "источник недоступен: "+err.Error())
		return
	}
	if res.Fetched == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "источник не вернул заданий")
		return
	}
	writeJSON(w, http.StatusOK, fetchResp{
		Fetched: res.Fetched, Inserted: res.Inserted, Skipped: res.Skipped, Invalid: res.Invalid,
		Promoted: res.Promoted, Source: mode,
	})
}

// fetchAndIngest pulls tasks from the source (optionally for one number) and
// runs them through ingest (media → MinIO, dedup). Shared by the bank's fetch
// button and the test builder (which fetches the tasks it needs into the bank).
func (s *Server) fetchAndIngest(ctx context.Context, subject domain.SubjectCode, limit, number int, status domain.TaskStatus) (ingest.Result, string, error) {
	if s.fetcherURL == "" {
		return ingest.Result{}, "", fmt.Errorf("источник заданий не настроен")
	}
	raws, mode, err := s.callFetcher(ctx, subject, limit, number)
	if err != nil {
		return ingest.Result{}, "", err
	}
	runner := ingest.NewRunner(s.store)
	if s.media != nil {
		runner.WithMedia(s.media)
	}
	runner.Status = status
	res, err := runner.Ingest(ctx, "fetch:"+string(subject), raws)
	return res, mode, err
}

// fetchResp is the ingest result plus the fetch mode reported by the fetcher
// (always "real": openfipi/РЕШУ — there is no mock source).
type fetchResp struct {
	Fetched  int    `json:"fetched"`
	Inserted int    `json:"inserted"`
	Skipped  int    `json:"skipped"`
	Invalid  int    `json:"invalid"`
	Promoted int    `json:"promoted"` // dedup hits promoted draft → active
	Source   string `json:"source"`
}

// callFetcherByIDs asks the fetcher to re-fetch specific РЕШУ problem ids (the
// upgrade path), returning refreshed RawTasks (statement + inline media).
func (s *Server) callFetcherByIDs(ctx context.Context, subject domain.SubjectCode, ids []string) ([]ingest.RawTask, error) {
	body, _ := json.Marshal(map[string]any{"subject": subject, "ids": ids})
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
