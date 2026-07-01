package bot

import (
	"strings"
	"testing"
)

func TestStatementToHTML_EscapesAndInlineFormulas(t *testing.T) {
	media := []MediaRef{{Kind: "image", Alt: "p = 500", Inline: true}}
	got := statementToHTML("Цена равна ⟦img:0⟧ руб. при a<b & c>d", media)
	if !strings.Contains(got, "p = 500") {
		t.Fatalf("inline formula alt not substituted: %q", got)
	}
	if !strings.Contains(got, "a&lt;b &amp; c&gt;d") {
		t.Fatalf("special chars not HTML-escaped: %q", got)
	}
}

func TestStatementToHTML_MissingAltDropsPlaceholder(t *testing.T) {
	media := []MediaRef{{Kind: "image", Alt: "", Inline: true}}
	got := statementToHTML("x = ⟦img:0⟧ end", media)
	if strings.Contains(got, "⟦img:") {
		t.Fatalf("empty-alt placeholder should be dropped: %q", got)
	}
	if !strings.Contains(got, "x =  end") {
		t.Fatalf("unexpected text around dropped placeholder: %q", got)
	}
}

func TestStatementToHTML_AlignsPipeTable(t *testing.T) {
	stmt := "Смотри таблицу:\n| A | Bb |\n| --- | --- |\n| 1 | 22 |"
	got := statementToHTML(stmt, nil)
	if !strings.Contains(got, "<pre>") || !strings.Contains(got, "</pre>") {
		t.Fatalf("table not wrapped in <pre>: %q", got)
	}
	// The divider row is redrawn, so a '-+-' rule must appear between header and body.
	if !strings.Contains(got, "-+-") {
		t.Fatalf("header divider not drawn: %q", got)
	}
	if !strings.Contains(got, "A | Bb") {
		t.Fatalf("header row not aligned: %q", got)
	}
	if !strings.Contains(got, "1 | 22") {
		t.Fatalf("body row not present: %q", got)
	}
}

func TestStatementToHTML_TableColumnPadding(t *testing.T) {
	// "abc" (width 3) over "a" forces the short cell to pad to 3.
	stmt := "| abc | y |\n| a | zz |"
	got := statementToHTML(stmt, nil)
	if !strings.Contains(got, "abc | y") {
		t.Fatalf("first row wrong: %q", got)
	}
	if !strings.Contains(got, "a   | zz") {
		t.Fatalf("short cell not padded to column width: %q", got)
	}
}
