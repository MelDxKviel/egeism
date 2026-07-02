package scheduler

import "testing"

func TestMarkupJSONAndCallbackOnly(t *testing.T) {
	rows := [][]Button{
		{{Text: "▶️ Решать тут", Data: "assign:abc"}},
		{{Text: "🌐 На сайте", URL: "http://localhost:3000"}},
	}
	full := markupJSON(rows)
	want := `{"inline_keyboard":[[{"text":"▶️ Решать тут","callback_data":"assign:abc"}],[{"text":"🌐 На сайте","url":"http://localhost:3000"}]]}`
	if full != want {
		t.Fatalf("markupJSON:\n got %s\nwant %s", full, want)
	}
	// The degradation step keeps callbacks and drops URL buttons (a rejected
	// button URL must not take «Решать тут» down with it).
	cb := markupJSON(callbackOnly(rows))
	wantCB := `{"inline_keyboard":[[{"text":"▶️ Решать тут","callback_data":"assign:abc"}]]}`
	if cb != wantCB {
		t.Fatalf("callbackOnly markup:\n got %s\nwant %s", cb, wantCB)
	}
}

func TestPluralTasks(t *testing.T) {
	cases := map[int64]string{1: "задание", 3: "задания", 15: "заданий", 22: "задания"}
	for n, want := range cases {
		if got := pluralTasks(n); got != want {
			t.Errorf("pluralTasks(%d) = %q, want %q", n, got, want)
		}
	}
}
