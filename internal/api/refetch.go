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

// refetchResp reports how many tasks were refreshed per subject.
type refetchResp struct {
	Updated   int            `json:"updated"`
	Scanned   int            `json:"scanned"`
	BySubject map[string]int `json:"by_subject"`
}

// handleRefetchFormulas upgrades РЕШУ/sdamgia tasks that predate inline-formula
// support: it re-fetches each by its stored extern_id and rewrites the statement
// (now with ⟦img:N⟧ placeholders) and media (formulas flagged inline) in place,
// keeping the curated answer and status. Non-destructive — no task is deleted,
// so tasks already placed in tests keep working. информатика (openfipi) is
// skipped: its images are genuine diagrams, correctly rendered as blocks.
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
// subject that still lack inline-formula placeholders but carry media (i.e. the
// formulas that were dumped as blocks). Pure-text tasks have nothing to fix.
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
		if len(t.Media) == 0 || strings.Contains(t.Statement, formulaPlaceholder) {
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
