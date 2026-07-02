package api

import (
	"context"
	"net/http"
	"strings"

	"egeism/internal/domain"
	"egeism/internal/ingest"
	"egeism/internal/store"
)

// formulaPlaceholder marks where an inline formula image sits in a statement
// (see the fetcher). A sdamgia task whose statement has none was ingested before
// inline-formula support and shows its formulas as detached blocks — the upgrade
// re-fetches it and rewrites statement + media in place.
const formulaPlaceholder = "⟦img:"

// theoryBloatMarkers are section headings from РЕШУ's collapsible «Что
// проверяется…» theory/справка card. They never appear in a real task condition,
// so a statement containing one was scraped by the OLD fetcher (which grabbed the
// hidden theory card instead of the condition — see tools/fetch/fetch.py) and
// must be re-fetched. Distinctive on purpose to avoid false positives.
var theoryBloatMarkers = []string{
	"Что проверяется", "Файлы для скачивания", "Необходимые материалы к заданию",
}

// isTheoryBloat reports whether a statement is a РЕШУ theory card dumped in as
// the condition (the русский-задания bug), so the refetch upgrade re-pulls it.
func isTheoryBloat(statement string) bool {
	for _, m := range theoryBloatMarkers {
		if strings.Contains(statement, m) {
			return true
		}
	}
	return false
}

// refetchResp reports how many tasks were refreshed per subject.
type refetchResp struct {
	Updated   int            `json:"updated"`
	Scanned   int            `json:"scanned"`
	BySubject map[string]int `json:"by_subject"`
}

// handleRefetchFormulas refreshes tasks whose stored statement predates a
// fetcher fix, rewriting statement + media in place while keeping the curated
// answer and status. Non-destructive — no task is deleted, so tasks already
// placed in tests keep working. Two passes:
//   - РЕШУ/sdamgia (rus/math/soc): stale tasks only — theory-card bloat (the
//     русский pbodies[0] bug) or formulas dumped as detached blocks (ingested
//     before inline-formula support).
//   - openfipi (информатика): EVERY task is re-parsed by the current parser and
//     rewritten only when the result differs — this heals e.g. the mangled
//     colspan/rowspan distance-matrix tables (задание 1) ingested before the
//     table fix, and any future parser improvement, with no per-bug heuristic.
func (s *Server) handleRefetchFormulas(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTeacher(w, r); !ok {
		return
	}
	if s.fetcherURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "источник заданий не настроен")
		return
	}
	runner := ingest.NewRunner(s.store)
	if s.media != nil {
		runner.WithMedia(s.media)
	}
	resp := refetchResp{BySubject: map[string]int{}}
	// РЕШУ/sdamgia subjects: re-fetch only tasks detected as stale.
	for _, subj := range []domain.SubjectCode{domain.SubjectMath, domain.SubjectRus, domain.SubjectSoc} {
		byExtern, err := s.sdamgiaTasksNeedingUpgrade(r.Context(), subj)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		resp.Scanned += len(byExtern)
		if len(byExtern) == 0 {
			continue
		}
		ids := make([]string, 0, len(byExtern))
		for id := range byExtern {
			ids = append(ids, id)
		}
		for _, chunk := range chunkStrings(ids, 40) {
			raws, err := s.callFetcherByIDs(r.Context(), subj, chunk)
			if err != nil {
				writeErr(w, http.StatusBadGateway, "источник недоступен: "+err.Error())
				return
			}
			for _, raw := range raws {
				t, ok := byExtern[raw.Source.ExternID]
				// Skip when the re-fetched statement is identical — the staleness
				// heuristic has permanent matches (e.g. a task whose media are
				// genuine block diagrams, not formulas), and without this the same
				// tasks would be rewritten and re-counted on every click.
				if !ok || raw.Statement == "" || raw.Statement == t.Statement {
					continue
				}
				media := runner.MediaFor(r.Context(), raw.Media)
				if _, err := s.store.UpdateTaskContent(r.Context(), t.ID, raw.Statement, media); err != nil {
					writeStoreErr(w, err)
					return
				}
				resp.Updated++
				resp.BySubject[string(subj)]++
			}
		}
	}
	// openfipi (информатика): re-parse everything, rewrite only real changes.
	if err := s.refetchOpenfipi(r.Context(), runner, &resp); err != nil {
		writeErr(w, http.StatusBadGateway, "источник недоступен: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// refetchOpenfipi re-fetches every stored openfipi task by id and rewrites the
// ones whose re-parsed statement differs from what's stored. Statement equality
// is the change detector — no per-bug staleness heuristic, so any past ingest
// bug is healed by whatever the current parser produces. Media is refreshed
// together with the statement (content-addressed keys → re-upload dedups).
func (s *Server) refetchOpenfipi(ctx context.Context, runner *ingest.Runner, resp *refetchResp) error {
	sub, err := s.store.GetSubjectByCode(ctx, domain.SubjectInf)
	if err != nil {
		return err
	}
	tasks, err := s.store.ListTasks(ctx, store.TaskFilter{SubjectID: &sub.ID, Limit: 5000})
	if err != nil {
		return err
	}
	byExtern := make(map[string]domain.Task)
	for _, t := range tasks {
		if t.Source != nil && t.Source.Provider == "openfipi" && t.Source.ExternID != "" {
			byExtern[t.Source.ExternID] = t
		}
	}
	resp.Scanned += len(byExtern)
	if len(byExtern) == 0 {
		return nil
	}
	ids := make([]string, 0, len(byExtern))
	for id := range byExtern {
		ids = append(ids, id)
	}
	// Smaller chunks than РЕШУ: the openfipi by-id path fetches sequentially with
	// a politeness delay, and each chunk must fit the fetcher's wall-time budget.
	for _, chunk := range chunkStrings(ids, 20) {
		raws, err := s.callFetcherByIDs(ctx, domain.SubjectInf, chunk)
		if err != nil {
			return err
		}
		for _, raw := range raws {
			t, ok := byExtern[raw.Source.ExternID]
			if !ok || raw.Statement == "" || raw.Statement == t.Statement {
				continue
			}
			media := runner.MediaFor(ctx, raw.Media)
			if _, err := s.store.UpdateTaskContent(ctx, t.ID, raw.Statement, media); err != nil {
				return err
			}
			resp.Updated++
			resp.BySubject[string(domain.SubjectInf)]++
		}
	}
	return nil
}

// sdamgiaTasksNeedingUpgrade returns extern_id → task for sdamgia tasks of a
// subject whose statement is stale: theory-card bloat, or (media without an
// inline-formula placeholder) formulas dumped as blocks. Clean tasks are skipped.
// The caller compares each re-fetched statement against the returned task's and
// writes only real changes.
func (s *Server) sdamgiaTasksNeedingUpgrade(ctx context.Context, subj domain.SubjectCode) (map[string]domain.Task, error) {
	sub, err := s.store.GetSubjectByCode(ctx, subj)
	if err != nil {
		return nil, err
	}
	tasks, err := s.store.ListTasks(ctx, store.TaskFilter{SubjectID: &sub.ID, Limit: 5000})
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Task)
	for _, t := range tasks {
		if t.Source == nil || t.Source.Provider != "sdamgia" || t.Source.ExternID == "" {
			continue
		}
		// Re-fetch a task if EITHER its statement is bloated with a РЕШУ theory
		// card (the русский pbodies[0] bug — pure text, so no media), OR it
		// predates inline-formula support (has media but no ⟦img:N⟧ placeholder,
		// i.e. formulas dumped as detached blocks). Clean tasks with neither are
		// left untouched.
		staleFormulas := len(t.Media) > 0 && !strings.Contains(t.Statement, formulaPlaceholder)
		if !isTheoryBloat(t.Statement) && !staleFormulas {
			continue
		}
		out[t.Source.ExternID] = t
	}
	return out, nil
}

func chunkStrings(s []string, size int) [][]string {
	var out [][]string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
