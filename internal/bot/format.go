package bot

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file turns a task statement into a Telegram-HTML message (parse_mode=HTML).
// It mirrors the web's StatementView (web/src/ui.tsx): Markdown-ish pipe tables
// (`| a | b |`) become aligned monospace <pre> blocks so columns don't collapse,
// and inline formula placeholders (⟦img:N⟧, emitted by the РЕШУ fetcher for
// <img class=tex> chunks) are swapped for their alt text so equations/special
// symbols read inline instead of vanishing. Everything else is escaped text.

var inlineImgRe = regexp.MustCompile(`⟦img:(\d+)⟧`)

// escapeHTML escapes the three characters Telegram's HTML parser is sensitive to.
// Ampersand first so we don't double-escape the entities we introduce.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// substituteInline replaces ⟦img:N⟧ placeholders with media[N].Alt (the formula's
// text form). A placeholder whose media has no alt is dropped from the text — the
// caller sends that formula image as a photo instead, so nothing is lost.
func substituteInline(s string, media []MediaRef) string {
	if !strings.Contains(s, "⟦img:") {
		return s
	}
	return inlineImgRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := inlineImgRe.FindStringSubmatch(m)
		idx, err := strconv.Atoi(sub[1])
		if err != nil || idx < 0 || idx >= len(media) {
			return ""
		}
		return media[idx].Alt
	})
}

// statementToHTML renders a task statement as Telegram HTML: pipe tables as
// aligned <pre> blocks, inline formulas as their alt text, the rest as escaped
// text with line breaks preserved.
func statementToHTML(text string, media []MediaRef) string {
	lines := strings.Split(text, "\n")
	var out []string
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		joined := strings.TrimRight(strings.Join(para, "\n"), "\n")
		if strings.TrimSpace(joined) != "" {
			out = append(out, escapeHTML(substituteInline(joined, media)))
		}
		para = para[:0]
	}
	for i := 0; i < len(lines); {
		if isTableRow(lines[i]) {
			flush()
			var rows []string
			for i < len(lines) && isTableRow(lines[i]) {
				rows = append(rows, lines[i])
				i++
			}
			out = append(out, tableHTML(rows, media))
		} else {
			para = append(para, lines[i])
			i++
		}
	}
	flush()
	return strings.Join(out, "\n")
}

// --- pipe-table detection (mirrors web/src/ui.tsx isRow/cellsOf/isSep) ---

var tableRowRe = regexp.MustCompile(`^\s*\|.*\|\s*$`)

func isTableRow(l string) bool { return tableRowRe.MatchString(l) }

func tableCells(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	l = strings.TrimSuffix(l, "|")
	parts := strings.Split(l, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

var dashesRe = regexp.MustCompile(`^-+$`)

// isSeparatorRow reports a Markdown header divider like `| --- | --- |`.
func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if c != "" && !dashesRe.MatchString(c) {
			return false
		}
	}
	return true
}

// tableHTML renders pipe-table rows as an aligned monospace <pre> block: cells
// are inline-substituted, columns padded to a common width, and a header divider
// drawn if the source had a `---` separator row.
func tableHTML(rows []string, media []MediaRef) string {
	parsed := make([][]string, 0, len(rows))
	sepAt := -1
	for _, r := range rows {
		cells := tableCells(r)
		for j, c := range cells {
			cells[j] = strings.TrimSpace(substituteInline(c, media))
		}
		if sepAt == -1 && isSeparatorRow(cells) {
			sepAt = len(parsed)
		}
		parsed = append(parsed, cells)
	}
	ncols := 0
	for _, cells := range parsed {
		if len(cells) > ncols {
			ncols = len(cells)
		}
	}
	widths := make([]int, ncols)
	for ri, cells := range parsed {
		if ri == sepAt {
			continue // the divider row doesn't set widths
		}
		for j := 0; j < ncols; j++ {
			if w := runeLen(cell(cells, j)); w > widths[j] {
				widths[j] = w
			}
		}
	}
	var b strings.Builder
	b.WriteString("<pre>")
	first := true
	for ri, cells := range parsed {
		if ri == sepAt {
			continue // re-drawn below as a padded divider
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(escapeHTML(joinPadded(cells, widths, ncols)))
		// After the header block, draw a divider so headers read as headers.
		if sepAt >= 0 && ri == sepAt-1 {
			b.WriteByte('\n')
			b.WriteString(dividerRow(widths))
		}
	}
	b.WriteString("</pre>")
	return b.String()
}

func cell(cells []string, j int) string {
	if j < len(cells) {
		return cells[j]
	}
	return ""
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// joinPadded pads each cell to its column width and joins with " | ".
func joinPadded(cells []string, widths []int, ncols int) string {
	parts := make([]string, ncols)
	for j := 0; j < ncols; j++ {
		c := cell(cells, j)
		parts[j] = c + strings.Repeat(" ", widths[j]-runeLen(c))
	}
	return strings.TrimRight(strings.Join(parts, " | "), " ")
}

// dividerRow builds a `---+---` rule matching the column widths.
func dividerRow(widths []int) string {
	segs := make([]string, len(widths))
	for j, w := range widths {
		segs[j] = strings.Repeat("-", w)
	}
	return strings.Join(segs, "-+-")
}
