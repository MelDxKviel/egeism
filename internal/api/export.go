package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/pdf"
)

// httpImageClient fetches media whose key is an absolute URL (the ingest
// fallback when the MinIO upload failed). Short timeout: a slow source must not
// stall the whole export.
var httpImageClient = &http.Client{Timeout: 10 * time.Second}

// handleExportTestPDF renders a composed test as a printable PDF —
// GET /api/admin/tests/{testID}/export.pdf. `?answers=1` appends the answer-key
// page (the teacher's copy); without it the file is safe to hand to a student.
func (s *Server) handleExportTestPDF(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	test, err := s.store.GetTest(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	tasks, err := s.store.ListTestTasks(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	var subjectTitle string
	if subs, err := s.store.ListSubjects(r.Context()); err == nil {
		for _, sub := range subs {
			if sub.ID == test.SubjectID {
				subjectTitle = sub.Title
				break
			}
		}
	}

	withAnswers := r.URL.Query().Get("answers") == "1"
	out, err := pdf.Render(r.Context(), test, tasks, pdf.Options{
		SubjectTitle: subjectTitle,
		WithAnswers:  withAnswers,
		FetchImage:   s.fetchTaskImage,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось собрать PDF")
		return
	}

	name := test.Title
	if withAnswers {
		name += " (с ответами)"
	}
	w.Header().Set("Content-Type", "application/pdf")
	// ASCII fallback + RFC 5987 filename* so Cyrillic titles survive the download.
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="variant.pdf"; filename*=UTF-8''%s`, url.PathEscape(name+".pdf")))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// fetchTaskImage resolves a task media key to raw bytes for PDF embedding:
// MinIO keys through the media store, absolute http(s) keys (ingest fallback)
// over HTTP. Errors just mean the figure degrades to its alt text.
func (s *Server) fetchTaskImage(ctx context.Context, key string) ([]byte, error) {
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpImageClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("fetch %s: status %d", key, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	}
	if s.media == nil {
		return nil, fmt.Errorf("no media store")
	}
	obj, err := s.media.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer obj.Body.Close()
	return io.ReadAll(io.LimitReader(obj.Body, 16<<20))
}
