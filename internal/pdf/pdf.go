// Package pdf renders a composed test (variant) into a printable PDF: a title
// block, every task's statement with pipe tables drawn as real grids and images
// embedded from media storage, an answer blank per task, and (optionally) a key
// of correct answers on a separate page for the teacher.
//
// This is presentation over the frozen domain types, like the bot's rich HTML:
// statement parsing mirrors internal/bot/format.go and web/src/ui.tsx — pipe
// tables (`| a | b |`) become grids, ⟦img:N⟧ inline-formula placeholders are
// substituted with their alt text, block figures are fetched and embedded.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strconv"
	"strings"

	_ "embed"
	_ "image/gif"  // decode support for figure embedding
	_ "image/jpeg" // decode support for figure embedding

	"github.com/go-pdf/fpdf"

	"egeism/internal/domain"
)

// DejaVu Sans ships with the binary so Cyrillic renders without host fonts.
// Bitstream Vera / public-domain licensed; see https://dejavu-fonts.github.io.
var (
	//go:embed fonts/DejaVuSans.ttf
	fontRegular []byte
	//go:embed fonts/DejaVuSans-Bold.ttf
	fontBold []byte
)

// ImageFetcher resolves a media key (MinIO object key or absolute URL) to raw
// image bytes. nil disables image embedding — figures degrade to their alt text.
type ImageFetcher func(ctx context.Context, key string) ([]byte, error)

// Options tunes one render.
type Options struct {
	SubjectTitle string       // human subject name for the header
	WithAnswers  bool         // append the answer key page (teacher copy)
	FetchImage   ImageFetcher // nil → skip figures, keep alt text
}

const (
	pageW      = 210.0 // A4 portrait, mm
	marginX    = 16.0
	marginTop  = 16.0
	contentW   = pageW - 2*marginX
	lineH      = 5.4 // body line height for 11pt
	cellPad    = 1.6 // inner padding of table cells
	maxImgW    = 150.0
	maxImgH    = 190.0
	imgDPI     = 110.0 // assumed source DPI: FIPI scans read well at this scale
	answerRule = "___________________________"
)

// Render builds the PDF for a test and its ordered tasks.
func Render(ctx context.Context, test domain.Test, tasks []domain.Task, opts Options) ([]byte, error) {
	doc := fpdf.New("P", "mm", "A4", "")
	doc.SetMargins(marginX, marginTop, marginX)
	doc.SetAutoPageBreak(true, 18)
	doc.AddUTF8FontFromBytes("dejavu", "", fontRegular)
	doc.AddUTF8FontFromBytes("dejavu", "B", fontBold)
	doc.SetTitle(test.Title, true)

	r := &renderer{doc: doc, ctx: ctx, opts: opts}
	r.header(test, len(tasks))
	for i, t := range tasks {
		r.task(i+1, t)
	}
	if opts.WithAnswers {
		r.answerKey(tasks)
	}

	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

type renderer struct {
	doc  *fpdf.Fpdf
	ctx  context.Context
	opts Options
	imgN int // unique image registration names
}

func (r *renderer) header(test domain.Test, taskCount int) {
	doc := r.doc
	doc.AddPage()
	doc.SetFont("dejavu", "B", 16)
	doc.MultiCell(contentW, 7.5, test.Title, "", "L", false)
	doc.Ln(1)

	kind := "Вариант ЕГЭ (часть 1)"
	if test.Kind == domain.TestDrill {
		kind = "Дрилл"
	}
	sub := strings.TrimSpace(r.opts.SubjectTitle)
	meta := kind
	if sub != "" {
		meta = sub + " · " + kind
	}
	meta += fmt.Sprintf(" · %s", plural(taskCount, "задание", "задания", "заданий"))
	doc.SetFont("dejavu", "", 10)
	doc.SetTextColor(110, 110, 110)
	doc.MultiCell(contentW, 5, meta, "", "L", false)
	doc.Ln(2)
	doc.SetTextColor(0, 0, 0)
	doc.SetFont("dejavu", "", 11)
	doc.MultiCell(contentW, 6, "ФИО: "+answerRule+"     Дата: _______________", "", "L", false)
	doc.Ln(2)
	r.rule()
}

// rule draws a light horizontal separator at the current position.
func (r *renderer) rule() {
	doc := r.doc
	y := doc.GetY()
	doc.SetDrawColor(200, 200, 200)
	doc.Line(marginX, y, pageW-marginX, y)
	doc.SetDrawColor(0, 0, 0)
	doc.Ln(3)
}

func (r *renderer) task(pos int, t domain.Task) {
	doc := r.doc
	// Keep the task header attached to at least a few statement lines.
	if doc.GetY() > 250 {
		doc.AddPage()
	}
	doc.Ln(2)
	doc.SetFont("dejavu", "B", 12)
	head := fmt.Sprintf("Задание %d", pos)
	doc.CellFormat(doc.GetStringWidth(head)+1, 7.5, head, "", 0, "L", false, 0, "")
	doc.SetFont("dejavu", "", 9)
	doc.SetTextColor(130, 130, 130)
	doc.CellFormat(0, 7.5, fmt.Sprintf("(№%d в ЕГЭ)", t.Number), "", 1, "L", false, 0, "")
	doc.SetTextColor(0, 0, 0)
	doc.Ln(0.5)

	r.statement(t.Statement, t.Media)
	r.figures(t.Media)

	doc.Ln(2.5)
	doc.SetFont("dejavu", "", 11)
	doc.MultiCell(contentW, 6, "Ответ: "+answerRule, "", "L", false)
	doc.Ln(1.5)
	r.rule()
}

// statement renders the task text: paragraph runs as wrapped text, pipe-table
// runs as bordered grids, ⟦img:N⟧ placeholders as their alt text.
func (r *renderer) statement(text string, media []domain.Media) {
	doc := r.doc
	doc.SetFont("dejavu", "", 11)
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
		doc.MultiCell(contentW, lineH, substituteInline(joined, media), "", "L", false)
		doc.Ln(1)
	}
	for i := 0; i < len(lines); {
		if isTableRow(lines[i]) {
			flush()
			var rows []string
			for i < len(lines) && isTableRow(lines[i]) {
				rows = append(rows, lines[i])
				i++
			}
			r.table(rows, media)
		} else {
			para = append(para, lines[i])
			i++
		}
	}
	flush()
}

// table draws pipe-table rows as a bordered grid: column widths follow content
// (scaled to fit the page), rows above the `---` separator render bold on a
// light fill, cell text wraps.
func (r *renderer) table(rows []string, media []domain.Media) {
	doc := r.doc
	parsed := make([][]string, 0, len(rows))
	sepAt := -1
	for _, row := range rows {
		cells := tableCells(row)
		for j, c := range cells {
			cells[j] = strings.TrimSpace(substituteInline(c, media))
		}
		if sepAt == -1 && isSeparatorRow(cells) {
			sepAt = len(parsed)
			continue
		}
		parsed = append(parsed, cells)
	}
	if len(parsed) == 0 {
		return
	}
	headerRows := 0
	if sepAt > 0 {
		headerRows = sepAt
	}
	ncols := 0
	for _, cells := range parsed {
		if len(cells) > ncols {
			ncols = len(cells)
		}
	}
	if ncols == 0 {
		return
	}

	// Column widths proportional to the widest cell, scaled to fit contentW.
	doc.SetFont("dejavu", "", 10)
	widths := make([]float64, ncols)
	for _, cells := range parsed {
		for j := 0; j < ncols; j++ {
			if w := doc.GetStringWidth(cellAt(cells, j)) + 2*cellPad + 1; w > widths[j] {
				widths[j] = w
			}
		}
	}
	total := 0.0
	for _, w := range widths {
		total += w
	}
	if total > contentW {
		scale := contentW / total
		for j := range widths {
			widths[j] *= scale
			if widths[j] < 8 { // keep narrow columns readable; slight overflow beats a 2mm column
				widths[j] = 8
			}
		}
	}

	tblLineH := 4.8
	for ri, cells := range parsed {
		header := ri < headerRows
		style := ""
		if header {
			style = "B"
		}
		doc.SetFont("dejavu", style, 10)
		// Wrap every cell, row height = tallest cell.
		wrapped := make([][]string, ncols)
		maxLines := 1
		for j := 0; j < ncols; j++ {
			wrapped[j] = doc.SplitText(cellAt(cells, j), widths[j]-2*cellPad)
			if len(wrapped[j]) > maxLines {
				maxLines = len(wrapped[j])
			}
		}
		rowH := float64(maxLines)*tblLineH + 2*cellPad

		if doc.GetY()+rowH > 279 { // page bottom minus break margin
			doc.AddPage()
		}
		x, y := marginX, doc.GetY()
		doc.SetDrawColor(120, 120, 120)
		for j := 0; j < ncols; j++ {
			if header {
				doc.SetFillColor(238, 238, 238)
				doc.Rect(x, y, widths[j], rowH, "FD")
			} else {
				doc.Rect(x, y, widths[j], rowH, "D")
			}
			for li, ln := range wrapped[j] {
				doc.SetXY(x+cellPad, y+cellPad+float64(li)*tblLineH)
				doc.CellFormat(widths[j]-2*cellPad, tblLineH, ln, "", 0, "C", false, 0, "")
			}
			x += widths[j]
		}
		doc.SetDrawColor(0, 0, 0)
		doc.SetXY(marginX, y+rowH)
	}
	doc.SetFont("dejavu", "", 11)
	doc.Ln(2)
}

// figures embeds the task's block images and notes attached files. Inline
// formulas with alt text were already substituted into the statement; anything
// unembeddable degrades to its alt text so no information is silently lost.
func (r *renderer) figures(media []domain.Media) {
	doc := r.doc
	var files []domain.Media
	for _, m := range media {
		if m.Kind == "file" {
			files = append(files, m)
			continue
		}
		if m.Inline && m.Alt != "" {
			continue // already in the text
		}
		if !r.image(m) && m.Alt != "" {
			doc.SetFont("dejavu", "", 10)
			doc.SetTextColor(110, 110, 110)
			doc.MultiCell(contentW, 5, "[Рисунок: "+m.Alt+"]", "", "L", false)
			doc.SetTextColor(0, 0, 0)
		}
	}
	if len(files) > 0 {
		names := make([]string, len(files))
		for i, f := range files {
			if f.Alt != "" {
				names[i] = f.Alt
			} else {
				names[i] = f.Key
			}
		}
		doc.Ln(1)
		doc.SetFont("dejavu", "", 10)
		doc.SetTextColor(110, 110, 110)
		doc.MultiCell(contentW, 5, "К заданию прилагаются файлы (доступны на сайте): "+strings.Join(names, ", "), "", "L", false)
		doc.SetTextColor(0, 0, 0)
	}
}

// image fetches and draws one figure, returning false when it can't be embedded
// (no fetcher, fetch error, or bytes that don't decode as an image). The source
// is decoded and re-encoded to a clean PNG first: fpdf has no way to recover
// from a corrupt image (SetError never clears), so only bytes the stdlib fully
// decodes are allowed anywhere near the document.
func (r *renderer) image(m domain.Media) bool {
	if r.opts.FetchImage == nil {
		return false
	}
	data, err := r.opts.FetchImage(r.ctx, m.Key)
	if err != nil || len(data) == 0 {
		return false
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	var clean bytes.Buffer
	if err := png.Encode(&clean, src); err != nil {
		return false
	}
	doc := r.doc
	r.imgN++
	name := fmt.Sprintf("img-%d", r.imgN)
	doc.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, &clean)
	if doc.Err() {
		return false
	}
	// Scale pixels to mm at the assumed source DPI, clamp to the content box.
	b := src.Bounds()
	w := float64(b.Dx()) * 25.4 / imgDPI
	h := float64(b.Dy()) * 25.4 / imgDPI
	if w > maxImgW {
		h *= maxImgW / w
		w = maxImgW
	}
	if h > maxImgH {
		w *= maxImgH / h
		h = maxImgH
	}
	if doc.GetY()+h > 279 {
		doc.AddPage()
	}
	doc.Ln(1)
	doc.ImageOptions(name, marginX, doc.GetY(), w, h, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	doc.SetY(doc.GetY() + h + 1)
	return true
}

// answerKey renders the teacher's key on its own page: one row per task with
// every accepted spelling of the answer.
func (r *renderer) answerKey(tasks []domain.Task) {
	doc := r.doc
	doc.AddPage()
	doc.SetFont("dejavu", "B", 14)
	doc.CellFormat(0, 8, "Ключ ответов", "", 1, "L", false, 0, "")
	doc.Ln(2)

	const (
		wPos = 18.0
		wNum = 22.0
	)
	wAns := contentW - wPos - wNum
	doc.SetFont("dejavu", "B", 10)
	doc.SetFillColor(238, 238, 238)
	doc.SetDrawColor(120, 120, 120)
	doc.CellFormat(wPos, 7, "#", "1", 0, "C", true, 0, "")
	doc.CellFormat(wNum, 7, "№ ЕГЭ", "1", 0, "C", true, 0, "")
	doc.CellFormat(wAns, 7, "Ответ", "1", 1, "C", true, 0, "")
	doc.SetFont("dejavu", "", 10)
	for i, t := range tasks {
		ans := strings.Join(t.AnswerSchema.Correct, " / ")
		lines := doc.SplitText(ans, wAns-2*cellPad)
		rowH := float64(len(lines))*4.8 + 2*cellPad
		if doc.GetY()+rowH > 279 {
			doc.AddPage()
		}
		y := doc.GetY()
		doc.Rect(marginX, y, wPos, rowH, "D")
		doc.Rect(marginX+wPos, y, wNum, rowH, "D")
		doc.Rect(marginX+wPos+wNum, y, wAns, rowH, "D")
		doc.SetXY(marginX, y+cellPad)
		doc.CellFormat(wPos, 4.8, strconv.Itoa(i+1), "", 0, "C", false, 0, "")
		doc.CellFormat(wNum, 4.8, strconv.Itoa(t.Number), "", 0, "C", false, 0, "")
		for li, ln := range lines {
			doc.SetXY(marginX+wPos+wNum+cellPad, y+cellPad+float64(li)*4.8)
			doc.CellFormat(wAns-2*cellPad, 4.8, ln, "", 0, "L", false, 0, "")
		}
		doc.SetXY(marginX, y+rowH)
	}
	doc.SetDrawColor(0, 0, 0)
}

// --- statement parsing (mirrors internal/bot/format.go and web/src/ui.tsx) ---

var inlineImgRe = regexp.MustCompile(`⟦img:(\d+)⟧`)

// substituteInline replaces ⟦img:N⟧ placeholders with media[N].Alt (the
// formula's text form); a placeholder without alt is dropped — its image still
// renders as a block figure below.
func substituteInline(s string, media []domain.Media) string {
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

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		if c != "" && !dashesRe.MatchString(c) {
			return false
		}
	}
	return true
}

func cellAt(cells []string, j int) string {
	if j < len(cells) {
		return cells[j]
	}
	return ""
}

// plural picks the Russian plural form for n (задание/задания/заданий).
func plural(n int, one, few, many string) string {
	form := many
	switch {
	case n%10 == 1 && n%100 != 11:
		form = one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}
