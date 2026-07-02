package bot

import (
	"strings"
	"testing"
)

// The load-bearing statement shape: paragraphs around a pipe table (the ФИПИ
// distance matrix emitted by the fetcher) plus a block figure and a file.
const matrixStatement = `На рисунке схема дорог, в таблице — протяжённость (км).
| | | Номер пункта | | |
| --- | --- | --- | --- | --- |
| | | 1 | 2 | 3 |
| Номер пункта | 1 | | 8 & 9 | |
| | 2 | 8 | | 74 |
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

	// A REAL table, not a <pre>: bordered+striped grid, header rows (before ---)
	// as <th>, data as <td>, cells centered like the web's .stmt-table.
	if !strings.Contains(html, `<table bordered striped><tr><th align="center"></th><th align="center"></th><th align="center">Номер пункта</th>`) {
		t.Fatalf("table header not rendered as bordered/centered <th>: %s", html)
	}
	if !strings.Contains(html, `<td align="center">8 &amp; 9</td>`) {
		t.Fatalf("cell content not escaped/rendered: %s", html)
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

	// Public origin → MinIO keys resolve through it and inline.
	html, leftovers := statementToRichHTML("Условие.", media, "https://ege.example.com")
	if !strings.Contains(html, `<img src="https://ege.example.com/api/media/abc123">`) {
		t.Fatalf("figure must inline via public origin: %s", html)
	}
	if len(leftovers) != 0 {
		t.Fatalf("no leftovers expected, got %+v", leftovers)
	}

	// Local origins are unreachable for Telegram (verified live:
	// RICH_MESSAGE_PHOTO_URL_INVALID fails the whole message) → never inline.
	for _, base := range []string{"", "http://localhost:3000", "http://127.0.0.1:8080"} {
		_, leftovers := statementToRichHTML("Условие.", media, base)
		if len(leftovers) != 1 {
			t.Fatalf("base %q: figure must stay a leftover, got %+v", base, leftovers)
		}
	}
}
