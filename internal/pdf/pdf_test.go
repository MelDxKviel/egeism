package pdf

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/google/uuid"

	"egeism/internal/domain"
)

func sampleTasks() []domain.Task {
	return []domain.Task{
		{
			ID:     uuid.New(),
			Number: 1,
			Statement: "Между населёнными пунктами построены дороги.\n" +
				"|  | A | B | C |\n| --- | --- | --- | --- |\n| A |  | 4 | 7 |\n| B | 4 |  | 2 |\n" +
				"Определите длину кратчайшего пути из A в C.",
			AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"6"}},
		},
		{
			ID:        uuid.New(),
			Number:    9,
			Statement: "Вычислите значение выражения ⟦img:0⟧ при x = 2.",
			Media: []domain.Media{
				{Key: "formula.png", Kind: "image", Alt: "x^2 + 3x", Inline: true},
				{Key: "figure.png", Kind: "image", Alt: "график функции"},
				{Key: "data.zip", Kind: "file", Alt: "files.zip"},
			},
			AnswerSchema: domain.AnswerSchema{Type: domain.AnswerString, Correct: []string{"десять", "10"}},
		},
	}
}

func sampleTest() domain.Test {
	return domain.Test{ID: uuid.New(), Title: "Вариант 1", Kind: domain.TestClassic}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for x := 0; x < 8; x++ {
		for y := 0; y < 6; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderProducesPDF(t *testing.T) {
	pngBytes := tinyPNG(t)
	out, err := Render(context.Background(), sampleTest(), sampleTasks(), Options{
		SubjectTitle: "Математика",
		WithAnswers:  true,
		FetchImage: func(ctx context.Context, key string) ([]byte, error) {
			return pngBytes, nil
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF (starts with %q)", out[:min(8, len(out))])
	}
	if len(out) < 10_000 {
		t.Fatalf("suspiciously small PDF: %d bytes", len(out))
	}
}

// A failing/absent image fetcher must degrade to alt text, not fail the export.
func TestRenderSurvivesImageFailures(t *testing.T) {
	cases := map[string]ImageFetcher{
		"nil fetcher":    nil,
		"fetch error":    func(ctx context.Context, key string) ([]byte, error) { return nil, errors.New("boom") },
		"not an image":   func(ctx context.Context, key string) ([]byte, error) { return []byte("<svg></svg>"), nil },
		"corrupt png":    func(ctx context.Context, key string) ([]byte, error) { return []byte("\x89PNG\r\n\x1a\ngarbage"), nil },
		"empty response": func(ctx context.Context, key string) ([]byte, error) { return nil, nil },
	}
	for name, fetch := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Render(context.Background(), sampleTest(), sampleTasks(), Options{FetchImage: fetch})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !bytes.HasPrefix(out, []byte("%PDF-")) {
				t.Fatal("output is not a PDF")
			}
		})
	}
}

func TestRenderEmptyTest(t *testing.T) {
	out, err := Render(context.Background(), sampleTest(), nil, Options{WithAnswers: true})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
}

func TestSubstituteInline(t *testing.T) {
	media := []domain.Media{{Alt: "x^2"}, {Alt: ""}}
	got := substituteInline("f = ⟦img:0⟧ и ⟦img:1⟧ и ⟦img:9⟧", media)
	want := "f = x^2 и  и "
	if got != want {
		t.Fatalf("substituteInline: got %q want %q", got, want)
	}
}
