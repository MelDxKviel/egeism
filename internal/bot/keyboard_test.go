package bot

import "testing"

func TestMarkupJSON(t *testing.T) {
	got := markupJSON([][]Button{
		{{Text: "▶️ Решать тут", Data: "assign:abc"}, {Text: "🌐 Сайт", URL: "https://x"}},
		{{Text: "", Data: "dropped"}, {Text: "no action"}}, // both invalid → row dropped
	})
	want := `{"inline_keyboard":[[{"text":"▶️ Решать тут","callback_data":"assign:abc"},{"text":"🌐 Сайт","url":"https://x"}]]}`
	if got != want {
		t.Fatalf("markupJSON:\n got %s\nwant %s", got, want)
	}
	if markupJSON(nil) != "" {
		t.Fatalf("empty rows must yield no markup")
	}
}

func TestPluralTasks(t *testing.T) {
	cases := map[int64]string{
		1: "задание", 2: "задания", 4: "задания", 5: "заданий",
		11: "заданий", 12: "заданий", 21: "задание", 22: "задания", 100: "заданий",
	}
	for n, want := range cases {
		if got := pluralTasks(n); got != want {
			t.Errorf("pluralTasks(%d) = %q, want %q", n, got, want)
		}
	}
}
