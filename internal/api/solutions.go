package api

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	"egeism/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxSolutionPhotoBytes = 10 << 20

func validSolutionPhoto(data []byte) (string, bool) {
	ct := http.DetectContentType(data)
	if ct != "image/jpeg" && ct != "image/png" {
		return "", false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return "", false
	}
	return ct, true
}

func (s *Server) ownAttempt(w http.ResponseWriter, r *http.Request, id uuid.UUID) bool {
	u, _ := userFrom(r.Context())
	a, err := s.store.GetAttempt(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return false
	}
	if u.Role != domain.RoleStudent || a.StudentID != u.ID {
		writeErr(w, http.StatusForbidden, "Это не твоя попытка")
		return false
	}
	return true
}

func (s *Server) handleUploadSolutionPhoto(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, 400, "invalid attempt id")
		return
	}
	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeErr(w, 400, "invalid task id")
		return
	}
	if !s.ownAttempt(w, r, attemptID) {
		return
	}
	if s.media == nil {
		writeErr(w, 503, "Хранилище фотографий не настроено")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSolutionPhotoBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeErr(w, 400, "Не удалось загрузить фото: максимум 10 МБ")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "Выбери фотографию")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSolutionPhotoBytes+1))
	if err != nil || len(data) > maxSolutionPhotoBytes {
		writeErr(w, 400, "Фотография должна быть не больше 10 МБ")
		return
	}
	ct, ok := validSolutionPhoto(data)
	if !ok {
		writeErr(w, 400, "Нужна фотография JPEG или PNG, до 40 мегапикселей")
		return
	}
	key := "solutions/" + uuid.NewString()
	if err := s.media.PutPrivate(r.Context(), key, data, ct); err != nil {
		writeErr(w, 503, "Не удалось сохранить фотографию")
		return
	}
	photo, err := s.store.MutateSolutionPhoto(r.Context(), domain.SolutionPhoto{AttemptID: attemptID, TaskID: taskID, ObjectKey: key, ContentType: ct, SizeBytes: int64(len(data))}, false)
	if err != nil {
		_ = s.media.Delete(r.Context(), key)
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, photo)
}

func (s *Server) handleGetSolutionPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "photoID"))
	if err != nil {
		writeErr(w, 400, "invalid photo id")
		return
	}
	p, err := s.store.SolutionPhoto(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !s.attemptReadable(w, r, p.AttemptID) {
		return
	}
	if s.media == nil {
		writeErr(w, 503, "Хранилище фотографий не настроено")
		return
	}
	obj, err := s.media.Get(r.Context(), p.ObjectKey)
	if err != nil {
		writeErr(w, 404, "Фотография не найдена")
		return
	}
	defer obj.Body.Close()
	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, obj.Body)
}

func (s *Server) handleDeleteSolutionPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "photoID"))
	if err != nil {
		writeErr(w, 400, "invalid photo id")
		return
	}
	p, err := s.store.SolutionPhoto(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !s.ownAttempt(w, r, p.AttemptID) {
		return
	}
	if _, err := s.store.MutateSolutionPhoto(r.Context(), p, true); err != nil {
		writeStoreErr(w, err)
		return
	}
	if s.media != nil {
		_ = s.media.Delete(r.Context(), p.ObjectKey)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSaveReview(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "attemptID"))
	if err != nil {
		writeErr(w, 400, "invalid attempt id")
		return
	}
	if !s.attemptReadable(w, r, id) {
		return
	}
	var req struct {
		Grades []domain.AnswerGrade `json:"grades"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.store.ReviewAttempt(r.Context(), id, teacher.ID, req.Grades); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.handleAttemptReview(w, r)
}
