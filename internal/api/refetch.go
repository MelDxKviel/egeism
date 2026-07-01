package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

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

// handleRefetchFormulas refreshes РЕШУ/sdamgia tasks with a stale/broken
// statement — either theory-card bloat (the русский pbodies[0] bug) or formulas
// dumped as detached blocks (ingested before inline-formula support). It
// re-fetches each by its stored extern_id and rewrites the statement (now the
// real condition, with ⟦img:N⟧ placeholders) and media in place, keeping the
// curated answer and status. Non-destructive — no task is deleted, so tasks
// already placed in tests keep working. информатика (openfipi) is skipped: its
// images are genuine diagrams, correctly rendered as blocks, and it has no
// by-id re-fetch.
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
	// sdamgia-backed subjects only (информатика comes from openfipi, no by-id).
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
				taskID, ok := byExtern[raw.Source.ExternID]
				if !ok || raw.Statement == "" {
					continue
				}
				media := runner.MediaFor(r.Context(), raw.Media)
				if _, err := s.store.UpdateTaskContent(r.Context(), taskID, raw.Statement, media); err != nil {
					writeStoreErr(w, err)
					return
				}
				resp.Updated++
				resp.BySubject[string(subj)]++
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// sdamgiaTasksNeedingUpgrade returns extern_id → task id for sdamgia tasks of a
// subject whose statement is stale: theory-card bloat, or (media without an
// inline-formula placeholder) formulas dumped as blocks. Clean tasks are skipped.
func (s *Server) sdamgiaTasksNeedingUpgrade(ctx context.Context, subj domain.SubjectCode) (map[string]uuid.UUID, error) {
	sub, err := s.store.GetSubjectByCode(ctx, subj)
	if err != nil {
		return nil, err
	}
	tasks, err := s.store.ListTasks(ctx, store.TaskFilter{SubjectID: &sub.ID, Limit: 5000})
	if err != nil {
		return nil, err
	}
	out := make(map[string]uuid.UUID)
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
		out[t.Source.ExternID] = t.ID
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
