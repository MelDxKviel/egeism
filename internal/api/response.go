package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"egeism/internal/store"
)

// errorBody is the uniform error envelope returned to clients.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

// writeErr maps common errors to status codes and returns the error envelope.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// writeStoreErr translates a store error into an HTTP response.
func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		slog.Error("store error", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

// decodeJSON reads a JSON request body into dst, capping the body size.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
