package bot

import (
	"strings"
	"testing"
)

// The load-bearing statement shape: paragraphs around a pipe table (the compact
// corner matrix the fetcher emits after collapsing the «Номер пункта»
// decorations) plus a block figure and a file.
const matrixStatement = `На рисунке схема дорог, в таблице — протяжённость (км).
| | 1 | 2 | 3 |
| --- | --- | --- | --- |
| 1 | | 8 & 9 | |
| 2 | 8 | | 74 |
Определите, какова протяжённость дороги из пункта A в пункт B.`

func richMedia() []MediaRef {
	return []MediaRef{
		{Key: "abc123", Kind: "image", Alt: "граф"},
		{Key: "https://ege.fipi.ru/pic.png", Kind: "image"},
		{Key: "zip456", Kind: "file", Alt: "файлы.zip"},
	}
}

func TestStatementToRichHTMLTables(t *testing.T) {
	html, leftovers := statementToRichHTML(matrixStatement, richMedia(), "")

	// A REAL table, not a <pre>: bordered+striped grid, the header row (before
	// ---) as <th>, cells centered like the web's .stmt-table.
	if !strings.Contains(html, `<table bordered striped><tr><th align="center"></th><th align="center">1</th>`) {
		t.Fatalf("table header not rendered as bordered/centered <th>: %s", html)
	}
	if !strings.Contains(html, `<td align="center">8 &amp; 9</td>`) {
		t.Fatalf("cell content not escaped/rendered: %s", html)
	}
	// Corner matrix (empty top-left header): row indices in the first column
	// render as <th> so both axes read as headers.
	if !strings.Contains(html, `<tr><th align="center">1</th><td align="center"></td><td align="center">8 &amp; 9</td>`) {
		t.Fatalf("row-label column not rendered as <th>: %s", html)
	}
	if strings.Contains(html, "<pre>") {
		t.Fatalf("rich rendering must not fall back to <pre>: %s", html)
	}
	// Paragraphs around the table survive.
	if !strings.Contains(html, "<p>На рисунке схема дорог, в таблице — протяжённость (км).</p>") {
		t.Fatalf("leading paragraph missing: %s", html)
	}
	if !strings.Contains(html, "из пункта A в пункт B") {
		t.Fatalf("trailing paragraph missing: %s", html)
	}

	// No public origin: the MinIO-key figure and the file stay leftovers; the
	// absolute FIPI URL is public and inlines.
	if !strings.Contains(html, `<img src="https://ege.fipi.ru/pic.png">`) {
		t.Fatalf("public absolute figure must inline: %s", html)
	}
	if len(leftovers) != 2 || leftovers[0].Kind != "file" && leftovers[1].Kind != "file" {
		t.Fatalf("expected MinIO figure + file as leftovers, got %+v", leftovers)
	}
}

func TestStatementToRichHTMLMediaBaseGate(t *testing.T) {
	media := []MediaRef{{Key: "abc123", Kind: "image"}}

	// A public media base (exposed MinIO bucket, or <web>/api/media) → MinIO
	// keys resolve through it and inline.
	for _, base := range []string{"https://ege.example.com/api/media", "http://203.0.113.5:9000/egeism-media/"} {
		html, leftovers := statementToRichHTML("Условие.", media, base)
		if !strings.Contains(html, `<img src="`+strings.TrimRight(base, "/")+`/abc123">`) {
			t.Fatalf("base %q: figure must inline, got %s", base, html)
		}
		if len(leftovers) != 0 {
			t.Fatalf("base %q: no leftovers expected, got %+v", base, leftovers)
		}
	}

	// Local origins are unreachable for Telegram (verified live:
	// RICH_MESSAGE_PHOTO_URL_INVALID fails the whole message) → never inline.
	for _, base := range []string{"", "http://localhost:9000/egeism-media", "http://127.0.0.1:8080/api/media"} {
		_, leftovers := statementToRichHTML("Условие.", media, base)
		if len(leftovers) != 1 {
			t.Fatalf("base %q: figure must stay a leftover, got %+v", base, leftovers)
		}
	}
}
