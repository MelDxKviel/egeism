package bot

import (
	"fmt"
	"net/url"
	"strings"
)

// This file builds Rich Message HTML (Bot API 10.1 sendRichMessage): unlike the
// classic parse_mode=HTML message (format.go), rich HTML supports REAL block
// elements — <table> (rendered as an actual grid, not a monospace <pre>),
// headings, paragraphs and inline <img>. The bot sends rich-first and falls back
// to the classic layout if Telegram rejects the rich call (see telegram.go).
//
// Media constraint (verified live): rich <img src> accepts PUBLIC http(s) URLs
// only — a localhost/LAN URL fails the WHOLE message with
// RICH_MESSAGE_PHOTO_URL_INVALID, and file_id/attach:// uploads are not
// supported. So figures are inlined only when their URL is publicly reachable
// (an absolute source URL, or mediaBase = WEB_URL pointing at a real host);
// everything else is returned as leftovers to send as photos/documents after.

// publicMediaURL resolves a media key to a URL Telegram's servers can fetch, or
// "" when there is none. Keys that are already absolute http(s) URLs (ingest
// fallback) are used as-is; MinIO keys resolve through the public web origin.
func publicMediaURL(key, mediaBase string) string {
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		if hostIsLocal(key) {
			return ""
		}
		return key
	}
	if mediaBase == "" || hostIsLocal(mediaBase) {
		return ""
	}
	return strings.TrimRight(mediaBase, "/") + "/api/media/" + key
}

// hostIsLocal reports whether a URL points at a host Telegram cannot reach
// (loopback and .local names). Not exhaustive — private-range IPs still slip
// through — but the rich send fails gracefully to the classic layout anyway.
func hostIsLocal(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	h := strings.ToLower(u.Hostname())
	return h == "" || h == "localhost" || h == "127.0.0.1" || h == "0.0.0.0" ||
		h == "::1" || strings.HasSuffix(h, ".local")
}

// statementToRichHTML renders a task statement as rich HTML: pipe tables become
// real <table> grids (header rows from the `---` separator as <th>), paragraph
// runs become <p> with <br> line breaks, ⟦img:N⟧ inline formulas substitute
// their alt text. Block figures with a publicly fetchable URL are inlined as
// <img>; the rest are returned as leftovers for the classic media send.
func statementToRichHTML(text string, media []MediaRef, mediaBase string) (string, []MediaRef) {
	var out []string
	lines := strings.Split(text, "\n")
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		joined := strings.TrimSpace(strings.Join(para, "\n"))
		para = para[:0]
		if joined == "" {
			return
		}
		escaped := escapeHTML(substituteInline(joined, media))
		out = append(out, "<p>"+strings.ReplaceAll(escaped, "\n", "<br>")+"</p>")
	}
	for i := 0; i < len(lines); {
		if isTableRow(lines[i]) {
			flush()
			var rows []string
			for i < len(lines) && isTableRow(lines[i]) {
				rows = append(rows, lines[i])
				i++
			}
			out = append(out, tableRichHTML(rows, media))
		} else {
			para = append(para, lines[i])
			i++
		}
	}
	flush()

	// Figures: inline the publicly fetchable ones, keep the rest for later.
	var leftovers []MediaRef
	for _, m := range media {
		if m.Kind == "file" || (m.Inline && m.Alt != "") {
			if m.Kind == "file" {
				leftovers = append(leftovers, m)
			}
			continue // inline formulas with alt already substituted into the text
		}
		if u := publicMediaURL(m.Key, mediaBase); u != "" {
			out = append(out, fmt.Sprintf(`<img src="%s">`, escapeHTML(u)))
		} else {
			leftovers = append(leftovers, m)
		}
	}
	return strings.Join(out, ""), leftovers
}

// tableRichHTML renders pipe-table rows as a real rich <table>: rows above the
// `---` separator become header cells (<th>), the rest are <td>.
func tableRichHTML(rows []string, media []MediaRef) string {
	parsed := make([][]string, 0, len(rows))
	sepAt := -1
	for _, r := range rows {
		cells := tableCells(r)
		for j, c := range cells {
			cells[j] = strings.TrimSpace(substituteInline(c, media))
		}
		if sepAt == -1 && isSeparatorRow(cells) {
			sepAt = len(parsed)
			continue // the divider is markup, not data
		}
		parsed = append(parsed, cells)
	}
	headerRows := 0
	if sepAt > 0 {
		headerRows = sepAt
	}
	var b strings.Builder
	b.WriteString("<table>")
	for ri, cells := range parsed {
		b.WriteString("<tr>")
		tag := "td"
		if ri < headerRows {
			tag = "th"
		}
		for _, c := range cells {
			b.WriteString("<" + tag + ">")
			b.WriteString(escapeHTML(c))
			b.WriteString("</" + tag + ">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}
