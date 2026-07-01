package api

import (
	"io"
	"net/http"
	"strings"

	"egeism/internal/domain"
	"egeism/internal/ingest"
)

// handleImportTasks lets a teacher upload a batch of tasks from the UI (or any
// client). Accepts either multipart/form-data with a `file` field, or a raw
// JSON/JSONL body. Tasks run through the same ingest as the CLI: media is
// downloaded into MinIO, duplicates are skipped, and new tasks land as `draft`
// (default) for curation in the bank. `provider` labels their source; `status`
// may be set to `active` to skip curation.
func (s *Server) handleImportTasks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}

	provider := "upload"
	status := domain.TaskDraft
	var data []byte

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid upload: "+err.Error())
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, 16<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "could not read file")
			return
		}
		if v := strings.TrimSpace(r.FormValue("provider")); v != "" {
			provider = v
		}
		if v := r.FormValue("status"); v == string(domain.TaskActive) {
			status = domain.TaskActive
		}
	} else {
		var err error
		data, err = io.ReadAll(io.LimitReader(http.MaxBytesReader(w, r.Body, 16<<20), 16<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "could not read body")
			return
		}
		if v := strings.TrimSpace(r.URL.Query().Get("provider")); v != "" {
			provider = v
		}
		if r.URL.Query().Get("status") == string(domain.TaskActive) {
			status = domain.TaskActive
		}
	}

	raws, err := ingest.DecodeRawTasks(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad format (expect JSON array or JSONL): "+err.Error())
		return
	}
	if len(raws) == 0 {
		writeErr(w, http.StatusBadRequest, "no tasks found in upload")
		return
	}

	runner := ingest.NewRunner(s.store)
	if s.media != nil {
		runner.WithMedia(s.media)
	}
	runner.Status = status

	res, err := runner.Ingest(r.Context(), provider, raws)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
