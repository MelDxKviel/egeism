package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
	"egeism/internal/store"
)

// handleListAdminTasks lists full tasks (including answer_schema) for the bank
// curation screen. Unlike the student /api/tasks, this exposes the answer so the
// teacher can review/edit it.
func (s *Server) handleListAdminTasks(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.TaskFilter{Limit: 200}
	// A subject-scoped teacher only ever sees their own bank: their subject
	// overrides whatever the query asks for.
	code := q.Get("subject")
	if teacher.Subject != nil {
		code = string(*teacher.Subject)
	}
	if code != "" {
		sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		f.SubjectID = &sub.ID
	}
	if n := q.Get("number"); n != "" {
		if num, err := strconv.Atoi(n); err == nil {
			f.Number = &num
		}
	}
	if st := q.Get("status"); st != "" {
		ts := domain.TaskStatus(st)
		f.Status = &ts
	}
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			f.Limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			f.Offset = v
		}
	}
	tasks, err := s.store.ListTasks(r.Context(), f)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks) // full domain.Task incl. answer_schema
}

// handleListTests lists tests, optionally filtered by subject, for the builder
// and assignment screens (contract gap #4).
func (s *Server) handleListTests(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var subjectID *uuid.UUID
	code := r.URL.Query().Get("subject")
	if teacher.Subject != nil {
		code = string(*teacher.Subject) // scoped teachers see only their subject
	}
	if code != "" {
		sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		subjectID = &sub.ID
	}
	tests, err := s.store.ListTests(r.Context(), subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tests)
}

type testDetail struct {
	Test  domain.Test   `json:"test"`
	Tasks []domain.Task `json:"tasks"`
}

// handleGetTestDetail returns a test with its ordered tasks (full, incl. answer)
// for builder preview and PDF export.
func (s *Server) handleGetTestDetail(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "testID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid test id")
		return
	}
	test, ok := s.testInScope(w, r, teacher, id)
	if !ok {
		return
	}
	tasks, err := s.store.ListTestTasks(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, testDetail{Test: test, Tasks: tasks})
}

type generateVariantReq struct {
	Subject domain.SubjectCode `json:"subject"`
	Kind    domain.TestKind    `json:"kind"`
	Title   string             `json:"title"`
	Number  int                `json:"number"` // drill only
	Count   int                `json:"count"`  // drill only
}

type generateVariantResp struct {
	Test      domain.Test `json:"test"`
	TaskCount int         `json:"task_count"`
	Source    string      `json:"source"` // "real"/"mock" — where tasks came from
}

// handleGenerateVariant auto-builds a test from random active bank tasks — the
// "собрать вариант в один клик" shortcut for teachers. classic = one random task
// per number; drill = N random tasks of one number.
func (s *Server) handleGenerateVariant(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var req generateVariantReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.subjectInScope(w, teacher, req.Subject) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if req.Kind != domain.TestClassic && req.Kind != domain.TestDrill {
		writeErr(w, http.StatusBadRequest, "kind must be classic or drill")
		return
	}
	if req.Kind == domain.TestDrill && req.Number <= 0 {
		writeErr(w, http.StatusBadRequest, "number is required for a drill variant")
		return
	}

	// Tests populate the base: fetch the tasks this variant needs from the
	// source into the bank (as active), then assemble from them. Fetch errors
	// are non-fatal — we assemble from whatever the bank already has.
	var source string
	if req.Kind == domain.TestDrill {
		n := req.Count * 3
		if n < 20 {
			n = 20
		}
		_, source, _ = s.fetchAndIngest(r.Context(), req.Subject, n, req.Number, domain.TaskActive)
	} else {
		_, source, _ = s.fetchAndIngest(r.Context(), req.Subject, 60, 0, domain.TaskActive)
	}

	// Distinct default names so variants are easy to tell apart (a teacher can
	// also rename later). Number by the smallest free ordinal among the subject's
	// existing tests, so repeat generations never collide and deletion gaps reuse.
	existing, _ := s.store.ListTests(r.Context(), &sub.ID)

	var gv store.GeneratedVariant
	if req.Kind == domain.TestClassic {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = nextVariantTitle(existing, "Вариант")
		}
		gv, err = s.store.GenerateClassicVariant(r.Context(), sub.ID, teacher.ID, title)
	} else {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = nextVariantTitle(existing, fmt.Sprintf("Дрилл №%d ·", req.Number))
		}
		gv, err = s.store.GenerateDrillVariant(r.Context(), sub.ID, req.Number, req.Count, teacher.ID, title)
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if gv.TaskCount == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "источник не дал заданий — попробуй ещё раз или проверь логи fetcher")
		return
	}
	writeJSON(w, http.StatusCreated, generateVariantResp{Test: gv.Test, TaskCount: gv.TaskCount, Source: source})
}

// nextVariantTitle returns a distinct default variant name of the form
// "<prefix> N", choosing the smallest N≥1 whose title isn't already taken by one
// of the subject's existing tests — so generated variants never share a name.
func nextVariantTitle(existing []domain.Test, prefix string) string {
	taken := make(map[string]bool, len(existing))
	for _, t := range existing {
		taken[t.Title] = true
	}
	for n := 1; ; n++ {
		if cand := fmt.Sprintf("%s %d", prefix, n); !taken[cand] {
			return cand
		}
	}
}
