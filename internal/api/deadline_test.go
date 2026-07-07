package api

import (
	"testing"
	"time"
)

// TestValidateDeadline covers the soft-deadline guard: a deadline is valid only
// when absent (no deadline) or strictly later than the scheduled time. The
// handler runs this before the fan-out, so a bad deadline means a 400 with no
// assignments created.
func TestValidateDeadline(t *testing.T) {
	scheduled := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		dueAt    *time.Time
		wantErr  bool
	}{
		{"no deadline", nil, false},
		{"deadline after scheduled", ptrTime(scheduled.Add(2 * time.Hour)), false},
		{"deadline equal scheduled", ptrTime(scheduled), true},
		{"deadline before scheduled", ptrTime(scheduled.Add(-1 * time.Hour)), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDeadline(scheduled, c.dueAt)
			if c.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
