package api

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"egeism/internal/domain"
)

type practiceReq struct {
	Subject domain.SubjectCode `json:"subject"`
}

type practiceResp struct {
	TestID    uuid.UUID `json:"test_id"`
	AttemptID uuid.UUID `json:"attempt_id"`
}

// masteredThreshold: a task solved correctly this many times stops appearing in
// practice (§ user request: don't repeat what's already learned).
const masteredThreshold = 2

// maxUnsolvedSelfVariants caps how many UNSOLVED пробники a student can pile up
// per subject. Solving one always frees a slot, so the cap message («сначала
// реши собранные») is honest and the lockout self-heals — unlike a lifetime cap.
const maxUnsolvedSelfVariants = 10

// maxSelfVariantList bounds the «Мои пробники» page size.
const maxSelfVariantList = 100

// weakSpotCutoff: a номер with accuracy at or above this isn't "weak" — it can
// surface from WeakSpots merely by being the worst of a good bunch.
const weakSpotCutoff = 0.7

// practiceLimit clamps a ?limit= to (0, max] with a default — a stray huge
// value must not dump the whole bank in one response.
func practiceLimit(r *http.Request, def, max int) int {
	limit := queryInt(r, "limit", def)
	if limit < 1 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

// handlePracticeTasks returns active tasks for the acting student to solve,
// EXCLUDING ones they've already mastered (solved correctly >= threshold),
// student-safe (no answers), random order. An optional ?number= narrows the
// pool to one задание — the server-side drill (тренировка по номеру).
func (s *Server) handlePracticeTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	var number *int
	if n := queryInt(r, "number", 0); n > 0 {
		number = &n
	}
	limit := practiceLimit(r, 20, 100)
	tasks, err := s.store.PracticeTasks(r.Context(), user.ID, subjectID, number, masteredThreshold, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskViews(tasks))
}

// handleMistakeTasks returns the acting student's «работа над ошибками» queue:
// active tasks whose latest answer is wrong, oldest first. Solving one
// correctly drops it out of the queue on the next request.
func (s *Server) handleMistakeTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	limit := practiceLimit(r, 15, 100)
	tasks, err := s.store.MistakeTasks(r.Context(), user.ID, subjectID, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskViews(tasks))
}

type practiceOverviewResp struct {
	Subject  domain.SubjectCode      `json:"subject"`
	Mistakes int                     `json:"mistakes"`
	Numbers  []domain.PracticeNumber `json:"numbers"`
}

// handlePracticeOverview powers the student's «Тренировка» hub in one request:
// the mistake-queue size plus the per-номер training map (bank availability,
// mastered count, lifetime accuracy).
func (s *Server) handlePracticeOverview(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	code := domain.SubjectCode(r.URL.Query().Get("subject"))
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	mistakes, err := s.store.CountMistakeTasks(r.Context(), user.ID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	numbers, err := s.store.PracticeNumbers(r.Context(), user.ID, subjectID, masteredThreshold)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, practiceOverviewResp{Subject: code, Mistakes: mistakes, Numbers: numbers})
}

type recommendedResp struct {
	MaxNumber int        `json:"max_number"`
	Tasks     []taskView `json:"tasks"`
	// Breakdown of the plan, for the UI's context line: how many tasks came
	// from the mistake queue and which weak номера contributed drills.
	Mistakes    int   `json:"mistakes"`
	WeakNumbers []int `json:"weak_numbers"`
}

// handleRecommendedTasks assembles the «умная тренировка» session: mistakes
// first, then a couple of tasks from each weak номер, then fresh unmastered
// tasks — deduplicated, capped at ?limit= (default 12).
func (s *Server) handleRecommendedTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	limit := practiceLimit(r, 12, 30)

	stats, err := s.store.RecentNumberPerformance(r.Context(), user.ID, subjectID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	ceiling := domain.TrainingCeiling(stats, 99)
	// The 12/13 boundary is a stage boundary: math part 2 is reached only
	// after all twelve short-answer numbers meet the prerequisites.
	mistakes, err := s.store.MistakeTasks(r.Context(), user.ID, subjectID, (limit+2)/3, ceiling)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	performance := map[int]domain.RecentNumber{}
	for _, row := range stats {
		performance[row.Number] = row
	}
	weakNumbers := []int{}
	var weak, fresh []domain.Task
	for n := 1; n <= ceiling; n++ {
		drills, err := s.store.PracticeTasks(r.Context(), user.ID, subjectID, &n, masteredThreshold, limit)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		sp := performance[n]
		if sp.Total >= 3 && float64(sp.Correct)/float64(sp.Total) < weakSpotCutoff && len(drills) > 0 {
			weakNumbers = append(weakNumbers, n)
			count := min(2, len(drills))
			weak = append(weak, drills[:count]...)
		}
		fresh = append(fresh, drills...)
	}
	// Interleave numbers so a full first-number pool cannot monopolize a session.
	fresh = interleaveNumbers(fresh, ceiling)

	plan := recommendPlan(mistakes, weak, fresh, limit)
	fromMistakes := map[uuid.UUID]bool{}
	for _, t := range mistakes {
		fromMistakes[t.ID] = true
	}
	mistakeCount := 0
	for _, t := range plan {
		if fromMistakes[t.ID] {
			mistakeCount++
		}
	}
	writeJSON(w, http.StatusOK, recommendedResp{
		Tasks: toTaskViews(plan), Mistakes: mistakeCount, WeakNumbers: weakNumbers, MaxNumber: ceiling,
	})
}

// recommendPlan merges the smart-session sources — mistakes first, then weak-
// номер drills, then fresh tasks — deduplicating by task id and capping at
// limit. The order is pedagogical: fix what went wrong, push the weak spots,
// then meet something new.
func recommendPlan(mistakes, weak, fresh []domain.Task, limit int) []domain.Task {
	if limit <= 0 {
		limit = 12
	}
	seen := make(map[uuid.UUID]bool, limit)
	out := make([]domain.Task, 0, limit)
	for _, group := range [][]domain.Task{mistakes, weak, fresh} {
		for _, t := range group {
			if len(out) >= limit {
				return out
			}
			if seen[t.ID] {
				continue
			}
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	return out
}

func interleaveNumbers(tasks []domain.Task, ceiling int) []domain.Task {
	pools := map[int][]domain.Task{}
	for _, t := range tasks {
		pools[t.Number] = append(pools[t.Number], t)
	}
	order := []int{}
	for n := max(1, ceiling-2); n <= ceiling; n++ {
		order = append(order, n)
	}
	for n := 1; n < max(1, ceiling-2); n++ {
		order = append(order, n)
	}
	out := make([]domain.Task, 0, len(tasks))
	for depth := 0; ; depth++ {
		added := false
		for _, n := range order {
			if depth < len(pools[n]) {
				out = append(out, pools[n][depth])
				added = true
			}
		}
		if !added {
			return out
		}
	}
}

// Student bank access shares the existing trusted fetcher/ingest, with a
// database-backed per-subject cooldown across users and API instances.
func (s *Server) handleStudentFetchBank(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	if u.Role != domain.RoleStudent {
		writeErr(w, 403, "Только для учеников")
		return
	}
	var req struct {
		Subject domain.SubjectCode `json:"subject"`
		Number  int                `json:"number"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Number < 0 || req.Number > 99 {
		writeErr(w, 400, "Некорректный номер задания")
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if s.fetcherURL == "" {
		writeErr(w, 503, "Источник заданий не настроен")
		return
	}
	claimed, err := s.store.ClaimPracticeFetch(r.Context(), sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if !claimed {
		w.Header().Set("Retry-After", "120")
		writeErr(w, 429, "Банк недавно обновлялся. Попробуй через две минуты")
		return
	}
	limit := 60
	if req.Number > 0 {
		limit = 15
	}
	res, mode, err := s.fetchAndIngest(r.Context(), req.Subject, limit, req.Number, domain.TaskActive)
	if err != nil {
		writeErr(w, 502, "Не удалось загрузить задания из источника. Попробуй позже")
		return
	}
	if res.Inserted+res.Promoted == 0 && res.Skipped == 0 {
		writeErr(w, 422, "Источник пока не вернул подходящих заданий")
		return
	}
	writeJSON(w, 200, fetchResp{Fetched: res.Fetched, Inserted: res.Inserted, Skipped: res.Skipped, Promoted: res.Promoted, Invalid: res.Invalid, Source: mode})
}

type selfVariantResp struct {
	Test      domain.Test `json:"test"`
	TaskCount int         `json:"task_count"`
}

// handleCreateSelfVariant builds a пробник for the acting student themselves:
// a classic exam-shaped variant (one random active task per номер) drawn from
// the EXISTING bank — unlike the teacher's generator it never calls the
// fetcher, so a student can't hammer external sources. The student then solves
// it through the ordinary test flow (GET /tests/{id}/tasks + POST /attempts).
func (s *Server) handleCreateSelfVariant(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	// Students only: a teacher's «Пробник N» would carry their creator role and
	// leak into every teacher's builder/assign library (ListTests filters by
	// creator role = student). Teachers have their own generator.
	if user.Role != domain.RoleStudent {
		writeErr(w, http.StatusForbidden, "пробники для себя собирает только ученик")
		return
	}
	var req practiceReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	unsolved, err := s.store.CountUnsolvedSelfVariants(r.Context(), user.ID, sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if unsolved >= maxUnsolvedSelfVariants {
		writeErr(w, http.StatusConflict, "у тебя уже много несданных пробников — сначала реши их")
		return
	}
	// Check the bank before creating the test row, so an empty bank doesn't
	// leave behind an empty test.
	avail, err := s.store.TaskCountsByNumber(r.Context(), sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	active := 0
	for _, a := range avail {
		active += a.Active
	}
	if active == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "в банке пока нет активных заданий — попроси учителя подтянуть их")
		return
	}
	// Title numbering counts ALL own пробники (monotonic), not just unsolved.
	total, err := s.store.CountSelfClassicTests(r.Context(), user.ID, sub.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	gv, err := s.store.GenerateClassicVariant(r.Context(), sub.ID, user.ID, fmt.Sprintf("Пробник %d", total+1))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if gv.TaskCount == 0 {
		// The bank emptied between the check and the draw; drop the husk.
		_ = s.store.DeleteTest(r.Context(), gv.Test.ID)
		writeErr(w, http.StatusUnprocessableEntity, "в банке пока нет активных заданий — попроси учителя подтянуть их")
		return
	}
	writeJSON(w, http.StatusCreated, selfVariantResp{Test: gv.Test, TaskCount: gv.TaskCount})
}

// handleListSelfVariants lists the acting student's generated пробники for a
// subject, with the latest finished attempt's score once solved.
func (s *Server) handleListSelfVariants(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	subjectID, ok := s.subjectID(w, r)
	if !ok {
		return
	}
	limit := practiceLimit(r, 50, maxSelfVariantList)
	rows, err := s.store.SelfVariants(r.Context(), user.ID, subjectID, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// handleStartPractice opens an ad-hoc free-solve session for the acting student:
// it get-or-creates their practice test for the subject and starts an attempt.
// The client then submits answers to attempt_id for any task. Used by the bot
// and web quick-practice (free solve, drills, mistake review, smart sessions —
// they all record against this one attempt stream).
func (s *Server) handleStartPractice(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	var req practiceReq
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.store.GetSubjectByCode(r.Context(), req.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	test, err := s.store.GetOrCreatePracticeTest(r.Context(), sub.ID, user.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	att, err := s.store.StartAttempt(r.Context(), user.ID, test.ID, nil)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, practiceResp{TestID: test.ID, AttemptID: att.ID})
}

// toTaskViews maps tasks to their student-safe views (no answers).
func toTaskViews(tasks []domain.Task) []taskView {
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, toTaskView(t))
	}
	return views
}
