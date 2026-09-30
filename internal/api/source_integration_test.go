package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"egeism/internal/domain"
	"egeism/internal/ingest"
	"egeism/internal/store"
	"github.com/google/uuid"
)

func TestSourceFreshnessPersistence(t *testing.T) {
	s := reviewTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	date := now.AddDate(0, -2, 0)
	src := domain.Source{Provider: "sdamgia", ExternID: "source-fixture", URL: "https://math-ege.sdamgia.ru/problem?id=1",
		PublishedAt: &date, VerifiedAt: &now, DateEvidenceURL: "https://math-ege.sdamgia.ru/methodist", DateEvidence: "test source date"}
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"current", src, true},
		{"legacy", map[string]string{"provider": "sdamgia", "extern_id": "old", "url": src.URL}, false},
		{"broken date", map[string]string{"provider": "sdamgia", "extern_id": "broken", "url": src.URL,
			"date_evidence": "test", "date_evidence_url": src.DateEvidenceURL, "published_at": "not-a-date", "verified_at": now.Format(time.RFC3339)}, false},
		{"untrusted", map[string]string{"provider": "mock", "extern_id": "made-up", "url": src.URL}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blob, _ := json.Marshal(tc.value)
			var current bool
			if err := s.store.Pool().QueryRow(ctx, "SELECT task_source_current($1::jsonb, $2)", blob, now).Scan(&current); err != nil {
				t.Fatal(err)
			}
			if current != tc.want {
				t.Fatalf("current=%v want=%v", current, tc.want)
			}
		})
	}

	sub, err := s.store.GetSubjectByCode(ctx, domain.SubjectMath)
	if err != nil {
		t.Fatal(err)
	}
	create := func(id string, published *time.Time) domain.Task {
		copy := src
		copy.ExternID, copy.PublishedAt = id, published
		task, err := s.store.CreateTask(ctx, domain.Task{SubjectID: sub.ID, Number: 1, Statement: "Integration fixture",
			AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"1"}}, Source: &copy, Status: domain.TaskDraft})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	fresh := create("current", &date)
	oldDate := now.AddDate(-2, 0, 0)
	old := create("old", &oldDate)
	unknown := create("unknown", nil)
	if _, err := s.store.SetTaskStatus(ctx, fresh.ID, domain.TaskActive); err != nil {
		t.Fatal(err)
	}
	for _, task := range []domain.Task{old, unknown} {
		if _, err := s.store.SetTaskStatus(ctx, task.ID, domain.TaskActive); err == nil {
			t.Fatal("stale source activated")
		}
		// Simulate already-active rows from before migration 00012.
		if _, err := s.store.Pool().Exec(ctx, "UPDATE tasks SET status='active' WHERE id=$1", task.ID); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := s.store.TaskCountsByNumber(ctx, sub.ID)
	if err != nil || len(counts) != 1 || counts[0].Active != 1 || counts[0].Total != 3 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	tasks, err := s.store.ListTasks(ctx, store.TaskFilter{SubjectID: &sub.ID, CurrentOnly: true})
	if err != nil || len(tasks) != 1 || tasks[0].ID != fresh.ID {
		t.Fatalf("current tasks=%+v err=%v", tasks, err)
	}
	tasks, err = s.store.PracticeTasks(ctx, uuid.New(), sub.ID, nil, 2, 20)
	if err != nil || len(tasks) != 1 || tasks[0].ID != fresh.ID {
		t.Fatalf("practice tasks=%+v err=%v", tasks, err)
	}

	// Reimport cannot rejuvenate an old task or silently change a historical
	// number merely because the current official variant uses another position.
	copy := src
	copy.ExternID = "old"
	if err := s.store.RefreshTaskSource(ctx, sub.ID, copy, 1); err != nil {
		t.Fatal(err)
	}
	got, err := s.store.GetTask(ctx, old.ID)
	if err != nil || got.Source.PublishedAt == nil || !got.Source.PublishedAt.Equal(oldDate) {
		t.Fatalf("old source refreshed incorrectly: %+v %v", got.Source, err)
	}
	copy.ExternID = "unknown"
	if err := s.store.RefreshTaskSource(ctx, sub.ID, copy, 2); err == nil {
		t.Fatal("mismatched exam number admitted")
	}
	got, _ = s.store.GetTask(ctx, unknown.ID)
	if got.Source.PublishedAt != nil || got.Number != 1 {
		t.Fatal("historical task changed on number mismatch")
	}

	runner := ingest.NewRunner(s.store)
	runner.Status = domain.TaskActive
	result, err := runner.Ingest(ctx, "sdamgia", []ingest.RawTask{
		{Subject: domain.SubjectMath, Number: 1, Statement: "Undated fixture", AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"1"}}, Source: domain.Source{Provider: "sdamgia", ExternID: "held", URL: src.URL}},
		{Subject: domain.SubjectMath, Number: 1, Statement: "Invalid schema", Source: domain.Source{Provider: "sdamgia", ExternID: "bad"}},
	})
	if err != nil || result.Held != 1 || result.Invalid != 1 || result.Inserted != 1 {
		t.Fatalf("ingest result=%+v err=%v", result, err)
	}
	// Numeric provider IDs are scoped to the subject's subsite. They may
	// coincide across math/inf without overwriting or promoting each other.
	other := src
	other.ExternID = "current"
	other.URL = "https://inf-ege.sdamgia.ru/problem?id=1"
	other.DateEvidenceURL = "https://inf-ege.sdamgia.ru/methodist"
	result, err = runner.Ingest(ctx, "sdamgia", []ingest.RawTask{{Subject: domain.SubjectInf, Number: 1,
		Statement: "Other subject fixture", AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"2"}}, Source: other}})
	if err != nil || result.Inserted != 1 || result.Skipped != 0 {
		t.Fatalf("cross-subject dedup: %+v %v", result, err)
	}
}
