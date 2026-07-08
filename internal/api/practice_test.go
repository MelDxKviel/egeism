package api

import (
	"testing"

	"github.com/google/uuid"

	"egeism/internal/domain"
)

func mkTasks(n int) []domain.Task {
	out := make([]domain.Task, n)
	for i := range out {
		out[i] = domain.Task{ID: uuid.New(), Number: i + 1}
	}
	return out
}

func ids(tasks []domain.Task) []uuid.UUID {
	out := make([]uuid.UUID, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return out
}

func TestRecommendPlan(t *testing.T) {
	mistakes := mkTasks(3)
	weak := mkTasks(4)
	fresh := mkTasks(10)

	t.Run("order is mistakes, weak, fresh", func(t *testing.T) {
		plan := recommendPlan(mistakes, weak, fresh, 12)
		if len(plan) != 12 {
			t.Fatalf("len = %d, want 12", len(plan))
		}
		want := append(append(append([]uuid.UUID{}, ids(mistakes)...), ids(weak)...), ids(fresh[:5])...)
		for i, id := range want {
			if plan[i].ID != id {
				t.Fatalf("plan[%d] = %s, want %s", i, plan[i].ID, id)
			}
		}
	})

	t.Run("dedup across groups", func(t *testing.T) {
		// The first weak task and the first two fresh tasks repeat a mistake.
		weakDup := append([]domain.Task{mistakes[0]}, weak...)
		freshDup := append([]domain.Task{mistakes[1], weak[1]}, fresh...)
		plan := recommendPlan(mistakes, weakDup, freshDup, 30)
		seen := map[uuid.UUID]bool{}
		for _, task := range plan {
			if seen[task.ID] {
				t.Fatalf("duplicate task %s in plan", task.ID)
			}
			seen[task.ID] = true
		}
		if len(plan) != 17 { // 3 + 4 + 10 unique
			t.Fatalf("len = %d, want 17", len(plan))
		}
	})

	t.Run("cap wins over sources", func(t *testing.T) {
		plan := recommendPlan(mistakes, weak, fresh, 5)
		if len(plan) != 5 {
			t.Fatalf("len = %d, want 5", len(plan))
		}
		// Mistakes always make the cut first.
		for i := range mistakes {
			if plan[i].ID != mistakes[i].ID {
				t.Fatalf("plan[%d] should be a mistake task", i)
			}
		}
	})

	t.Run("empty sources", func(t *testing.T) {
		if got := recommendPlan(nil, nil, nil, 10); len(got) != 0 {
			t.Fatalf("len = %d, want 0", len(got))
		}
		if got := recommendPlan(nil, nil, fresh, 10); len(got) != 10 {
			t.Fatalf("len = %d, want 10", len(got))
		}
	})

	t.Run("non-positive limit falls back to default", func(t *testing.T) {
		if got := recommendPlan(nil, nil, fresh, 0); len(got) != 10 {
			t.Fatalf("len = %d, want all 10 under default limit 12", len(got))
		}
	})
}
