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

type taskSummaryResp struct {
	Subject domain.SubjectCode          `json:"subject"`
	Numbers []domain.NumberAvailability `json:"numbers"`
}

// handleTaskSummary returns per-номер bank availability for a subject (active +
// total task counts), номер-ordered. The composed-variant builder uses it to
// show which задания can be filled and by how many before the teacher composes.
func (s *Server) handleTaskSummary(w http.ResponseWriter, r *http.Request) {
	teacher, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	// A subject-scoped teacher only ever sees their own bank.
	code := r.URL.Query().Get("subject")
	if teacher.Subject != nil {
		code = string(*teacher.Subject)
	}
	if code == "" {
		writeErr(w, http.StatusBadRequest, "subject query param is required")
		return
	}
	if !s.subjectInScope(w, teacher, domain.SubjectCode(code)) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), domain.SubjectCode(code))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	nums, err := s.store.TaskCountsByNumber(r.Context(), sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskSummaryResp{Subject: domain.SubjectCode(code), Numbers: nums})
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

type slotReq struct {
	Number int `json:"number"`
	Count  int `json:"count"`
}

type generateVariantReq struct {
	Subject domain.SubjectCode `json:"subject"`
	Kind    domain.TestKind    `json:"kind"`
	Title   string             `json:"title"`
	Number  int                `json:"number"` // drill only
	Count   int                `json:"count"`  // drill only
	Slots   []slotReq          `json:"slots"`  // composed only: per-number counts
}

type generateVariantResp struct {
	Test      domain.Test `json:"test"`
	TaskCount int         `json:"task_count"`
	Requested int         `json:"requested"` // tasks asked for (composed) — may exceed task_count if the bank ran short
	Source    string      `json:"source"`    // always "real" — there is no mock source
}

const (
	// A composed variant is capped so a stray range can't build an enormous test
	// (or hammer the fetcher). maxTaskNumber bounds a задание-номер defensively.
	maxComposedTasks = 100
	maxTaskNumber    = 99
)

// normalizeSlots validates and aggregates a composed variant's per-number slots:
// empty slots (count ≤ 0) are dropped, out-of-range numbers rejected, duplicate
// numbers summed (preserving first-seen order so "3 первых, 3 вторых" keeps its
// order), and the total task count capped. Returns the cleaned slots and the
// total requested. Pure — unit-tested independently of the handler.
func normalizeSlots(in []slotReq) (slots []store.VariantSlot, total int, err error) {
	agg := map[int]int{}
	order := make([]int, 0, len(in))
	for _, sl := range in {
		if sl.Count <= 0 {
			continue
		}
		if sl.Number < 1 || sl.Number > maxTaskNumber {
			return nil, 0, fmt.Errorf("номер задания вне диапазона: %d", sl.Number)
		}
		if _, ok := agg[sl.Number]; !ok {
			order = append(order, sl.Number)
		}
		agg[sl.Number] += sl.Count
	}
	if len(order) == 0 {
		return nil, 0, fmt.Errorf("укажите хотя бы одно задание")
	}
	slots = make([]store.VariantSlot, 0, len(order))
	for _, n := range order {
		total += agg[n]
		slots = append(slots, store.VariantSlot{Number: n, Count: agg[n]})
	}
	if total > maxComposedTasks {
		return nil, 0, fmt.Errorf("слишком много заданий: %d (максимум %d)", total, maxComposedTasks)
	}
	return slots, total, nil
}

// handleGenerateVariant auto-builds a test from random active bank tasks — the
// teacher's variant builder. classic = one random task per number; drill = N
// random tasks of one number; composed = a teacher-defined mix (`slots`: per
// number how many tasks, e.g. 3×№1 + 3×№2 + 3×№3), laid out grouped by number.
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
	if req.Kind != domain.TestClassic && req.Kind != domain.TestDrill && req.Kind != domain.TestComposed {
		writeErr(w, http.StatusBadRequest, "kind must be classic, drill or composed")
		return
	}
	if req.Kind == domain.TestDrill && req.Number <= 0 {
		writeErr(w, http.StatusBadRequest, "number is required for a drill variant")
		return
	}
	// Composed: validate + aggregate the per-number slots up front so a bad
	// request fails before any fetch.
	var slots []store.VariantSlot
	var requested int
	if req.Kind == domain.TestComposed {
		slots, requested, err = normalizeSlots(req.Slots)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Tests populate the base: fetch the tasks this variant needs from the
	// source into the bank (as active), then assemble from them. Fetch errors
	// are non-fatal — we assemble from whatever the bank already has.
	source := "real"
	switch req.Kind {
	case domain.TestDrill:
		n := req.Count * 3
		if n < 20 {
			n = 20
		}
		_, source, _ = s.fetchAndIngest(r.Context(), req.Subject, n, req.Number, domain.TaskActive)
	case domain.TestComposed:
		// Top up each requested номер concurrently, then assemble. Pull a little
		// extra per number so repeat generations can draw fresh tasks.
		numbers := make([]int, 0, len(slots))
		per := 0
		for _, sl := range slots {
			numbers = append(numbers, sl.Number)
			if sl.Count > per {
				per = sl.Count
			}
		}
		per *= 2
		if per < 10 {
			per = 10
		}
		if per > 40 {
			per = 40
		}
		_, _ = s.fetchNumbersAndIngest(r.Context(), req.Subject, numbers, per, domain.TaskActive)
	default: // classic
		_, source, _ = s.fetchAndIngest(r.Context(), req.Subject, 60, 0, domain.TaskActive)
	}

	// Distinct default names so variants are easy to tell apart (a teacher can
	// also rename later). Number by the smallest free ordinal among the subject's
	// existing tests, so repeat generations never collide and deletion gaps reuse.
	existing, _ := s.store.ListTests(r.Context(), &sub.ID)
	title := strings.TrimSpace(req.Title)

	var gv store.GeneratedVariant
	switch req.Kind {
	case domain.TestClassic:
		if title == "" {
			title = nextVariantTitle(existing, "Вариант")
		}
		gv, err = s.store.GenerateClassicVariant(r.Context(), sub.ID, teacher.ID, title)
	case domain.TestDrill:
		if title == "" {
			title = nextVariantTitle(existing, fmt.Sprintf("Дрилл №%d ·", req.Number))
		}
		gv, err = s.store.GenerateDrillVariant(r.Context(), sub.ID, req.Number, req.Count, teacher.ID, title)
	case domain.TestComposed:
		if title == "" {
			title = nextVariantTitle(existing, "Составной вариант")
		}
		gv, err = s.store.GenerateComposedVariant(r.Context(), sub.ID, slots, teacher.ID, title)
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if gv.TaskCount == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "источник не дал заданий — попробуй ещё раз или проверь логи fetcher")
		return
	}
	writeJSON(w, http.StatusCreated, generateVariantResp{Test: gv.Test, TaskCount: gv.TaskCount, Requested: requested, Source: source})
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
