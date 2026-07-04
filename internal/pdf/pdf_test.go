package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// РЕШУ serves every formula (and many figures) as SVG — exactly what used to
// come out of the PDF blank. Both media of the sample task must now embed as
// real images: the inline ⟦img:0⟧ formula mid-sentence and the block figure.
func TestRenderEmbedsSVG(t *testing.T) {
	svgSized := func(w int) []byte {
		return []byte(fmt.Sprintf(`<?xml version="1.0"?>`+
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d 16">`+
			`<rect x="1" y="1" width="%d" height="14" fill="black"/></svg>`, w, w-2))
	}
	fetched := map[string]int{}
	out, err := Render(context.Background(), sampleTest(), sampleTasks(), Options{
		FetchImage: func(ctx context.Context, key string) ([]byte, error) {
			fetched[key]++
			if key == "formula.png" {
				return svgSized(40), nil // line-sized formula
			}
			return svgSized(400), nil // wide block figure
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// fpdf dedups identical bytes, so distinct content must yield two objects.
	if got := bytes.Count(out, []byte("/Subtype /Image")); got != 2 {
		t.Fatalf("embedded images: got %d, want 2 (inline formula + block figure)", got)
	}
	// The attached file is never fetched; each image key exactly once (cache).
	if fetched["data.zip"] != 0 {
		t.Fatalf("file media must not be fetched, got %d", fetched["data.zip"])
	}
	for _, key := range []string{"formula.png", "figure.png"} {
		if fetched[key] != 1 {
			t.Fatalf("fetch count for %s: got %d, want 1", key, fetched[key])
		}
	}
}

// A failed image is remembered: repeated placeholders of one key cost one fetch.
func TestImageFailureCached(t *testing.T) {
	task := domain.Task{
		ID:        uuid.New(),
		Number:    2,
		Statement: "⟦img:0⟧ и ещё раз ⟦img:1⟧",
		Media: []domain.Media{
			{Key: "same.png", Kind: "image", Alt: "x", Inline: true},
			{Key: "same.png", Kind: "image", Alt: "x", Inline: true},
		},
		AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"1"}},
	}
	calls := 0
	_, err := Render(context.Background(), sampleTest(), []domain.Task{task}, Options{
		FetchImage: func(ctx context.Context, key string) ([]byte, error) {
			calls++
			return nil, errors.New("boom")
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls: got %d, want 1 (failure cached)", calls)
	}
}

func TestLooksSVG(t *testing.T) {
	cases := map[string]bool{
		`<svg viewBox="0 0 1 1"/>`:                         true,
		"\xef\xbb\xbf  <?xml version=\"1.0\"?><svg></svg>": true,
		"\x89PNG\r\n\x1a\n":                                false,
		"plain text":                                       false,
		"":                                                 false,
	}
	for in, want := range cases {
		if got := looksSVG([]byte(in)); got != want {
			t.Errorf("looksSVG(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRasterizeSVG(t *testing.T) {
	img, w, h, err := rasterizeSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 16"><rect width="40" height="16"/></svg>`))
	if err != nil {
		t.Fatalf("rasterizeSVG: %v", err)
	}
	if w != 40 || h != 16 {
		t.Fatalf("natural size: got %gx%g, want 40x16", w, h)
	}
	b := img.Bounds()
	if b.Dx() != 160 || b.Dy() != 64 { // 4× scale
		t.Fatalf("raster size: got %dx%d, want 160x64", b.Dx(), b.Dy())
	}
	// Degenerate SVGs must error (degrade to alt), not produce a broken image.
	if _, _, _, err := rasterizeSVG([]byte(`<svg></svg>`)); err == nil {
		t.Fatal("empty-viewBox svg: want error")
	}
	// oksvg skips <text> with a warning; an SVG that renders all-white must
	// error too — alt text beats an invisible formula.
	textOnly := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 90 18"><text x="2" y="14">formula</text></svg>`
	if _, _, _, err := rasterizeSVG([]byte(textOnly)); err == nil {
		t.Fatal("blank-rendered svg: want error")
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
