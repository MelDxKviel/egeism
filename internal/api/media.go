package api

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleGetMedia streams a task media object (image/file) from MinIO. Public so
// <img src> and file links work without auth; keys are unguessable content
// hashes. Returns 404 when no media store is configured.
func (s *Server) handleGetMedia(w http.ResponseWriter, r *http.Request) {
	if s.media == nil {
		writeErr(w, http.StatusNotFound, "media not available")
		return
	}
	key := chi.URLParam(r, "*")
	if key == "" {
		writeErr(w, http.StatusBadRequest, "missing media key")
		return
	}
	obj, err := s.media.Get(r.Context(), key)
	if err != nil {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	defer obj.Body.Close()
	if obj.ContentType != "" {
		w.Header().Set("Content-Type", obj.ContentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = io.Copy(w, obj.Body)
}
