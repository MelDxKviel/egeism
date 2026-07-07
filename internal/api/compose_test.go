package api

import "testing"

// TestNormalizeSlots covers the composed-variant slot validation/aggregation the
// generate handler runs before any fetch: empty slots dropped, bad numbers
// rejected, duplicate numbers summed in first-seen order, total capped.
func TestNormalizeSlots(t *testing.T) {
	t.Run("range keeps order and counts", func(t *testing.T) {
		slots, total, err := normalizeSlots([]slotReq{{1, 3}, {2, 3}, {3, 3}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 9 {
			t.Fatalf("total = %d, want 9", total)
		}
		if len(slots) != 3 || slots[0].Number != 1 || slots[0].Count != 3 ||
			slots[2].Number != 3 || slots[2].Count != 3 {
			t.Fatalf("unexpected slots: %+v", slots)
		}
	})

	t.Run("empty slots dropped", func(t *testing.T) {
		slots, total, err := normalizeSlots([]slotReq{{1, 0}, {2, 5}, {3, -2}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(slots) != 1 || slots[0].Number != 2 || slots[0].Count != 5 || total != 5 {
			t.Fatalf("unexpected: slots=%+v total=%d", slots, total)
		}
	})

	t.Run("duplicate numbers summed, order preserved", func(t *testing.T) {
		slots, total, err := normalizeSlots([]slotReq{{5, 2}, {1, 1}, {5, 3}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 6 {
			t.Fatalf("total = %d, want 6", total)
		}
		if len(slots) != 2 || slots[0].Number != 5 || slots[0].Count != 5 || slots[1].Number != 1 {
			t.Fatalf("unexpected slots: %+v", slots)
		}
	})

	t.Run("no valid slots errors", func(t *testing.T) {
		if _, _, err := normalizeSlots(nil); err == nil {
			t.Fatal("expected an error for empty slots")
		}
		if _, _, err := normalizeSlots([]slotReq{{1, 0}}); err == nil {
			t.Fatal("expected an error when every slot is empty")
		}
	})

	t.Run("out-of-range number errors", func(t *testing.T) {
		if _, _, err := normalizeSlots([]slotReq{{0, 3}}); err == nil {
			t.Fatal("expected an error for номер 0")
		}
		if _, _, err := normalizeSlots([]slotReq{{maxTaskNumber + 1, 1}}); err == nil {
			t.Fatal("expected an error for номер above the max")
		}
	})

	t.Run("total cap enforced", func(t *testing.T) {
		if _, _, err := normalizeSlots([]slotReq{{1, maxComposedTasks + 1}}); err == nil {
			t.Fatal("expected an error when the total exceeds the cap")
		}
		// Exactly at the cap is allowed.
		if _, total, err := normalizeSlots([]slotReq{{1, maxComposedTasks}}); err != nil || total != maxComposedTasks {
			t.Fatalf("cap boundary rejected: total=%d err=%v", total, err)
		}
	})
}
