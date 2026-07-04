// Package pdf renders a composed test (variant) into a printable PDF: a title
// block, every task's statement with pipe tables drawn as real grids and images
// embedded from media storage, an answer blank per task, and (optionally) a key
// of correct answers on a separate page for the teacher.
//
// This is presentation over the frozen domain types, like the bot's rich HTML:
// statement parsing mirrors internal/bot/format.go and web/src/ui.tsx — pipe
// tables (`| a | b |`) become grids, ⟦img:N⟧ inline-formula placeholders are
// drawn as real images flowing mid-sentence (like the web; РЕШУ formulas are
// SVG, rasterized here) with their alt text as the fallback, block figures are
// fetched and embedded.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"regexp"
	"strconv"
	"strings"

	_ "embed"
	_ "image/gif"  // decode support for figure embedding
	_ "image/jpeg" // decode support for figure embedding

	_ "golang.org/x/image/bmp"  // decode support for figure embedding
	_ "golang.org/x/image/tiff" // decode support for figure embedding
	_ "golang.org/x/image/webp" // decode support for figure embedding

	"github.com/go-pdf/fpdf"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

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
	pageBottom = 279.0 // page bottom minus break margin
	lineH      = 5.4   // body line height for 11pt
	cellPad    = 1.6   // inner padding of table cells
	maxImgW    = 150.0
	maxImgH    = 190.0
	imgDPI     = 110.0 // assumed raster source DPI: FIPI scans read well at this scale
	svgDPI     = 96.0  // SVG user units are CSS px (РЕШУ formulas size in them)
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

	r := &renderer{doc: doc, ctx: ctx, opts: opts, imgCache: map[string]*embImg{}}
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
	// One fetch+decode+registration per media key across the document (formula
	// images repeat); nil = known-bad key (failed fetch/decode), skip retries.
	imgCache map[string]*embImg
}

// embImg is a media image registered with the document, with its natural
// display size in mm (raster px at imgDPI, SVG user units at svgDPI).
type embImg struct {
	name     string
	wMM, hMM float64
}

var pngOpts = fpdf.ImageOptions{ImageType: "PNG"}

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

	handled := r.statement(t.Statement, t.Media)
	r.figures(t.Media, handled)

	doc.Ln(2.5)
	doc.SetFont("dejavu", "", 11)
	doc.MultiCell(contentW, 6, "Ответ: "+answerRule, "", "L", false)
	doc.Ln(1.5)
	r.rule()
}

// statement renders the task text: paragraph runs as flowing text with ⟦img:N⟧
// placeholders drawn as real inline images (alt text as the fallback),
// pipe-table runs as bordered grids. Returns the media indices it rendered (or
// substituted) so figures() doesn't repeat them as blocks.
func (r *renderer) statement(text string, media []domain.Media) map[int]bool {
	handled := map[int]bool{}
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
		r.flowText(joined, media, handled)
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
	return handled
}

// flowText writes one paragraph as flowing text, drawing each ⟦img:N⟧ as an
// image mid-sentence (the web's renderInline, in print). A formula that can't
// be fetched/decoded degrades to its alt text, exactly the old behavior.
func (r *renderer) flowText(text string, media []domain.Media, handled map[int]bool) {
	doc := r.doc
	doc.SetFont("dejavu", "", 11)
	locs := inlineImgRe.FindAllStringSubmatchIndex(text, -1)
	pos := 0
	for _, loc := range locs {
		if loc[0] > pos {
			doc.Write(lineH, text[pos:loc[0]])
		}
		idx, err := strconv.Atoi(text[loc[2]:loc[3]])
		pos = loc[1]
		if err != nil || idx < 0 || idx >= len(media) {
			continue
		}
		r.inlineImage(idx, media[idx], handled)
	}
	if pos < len(text) {
		doc.Write(lineH, text[pos:])
	}
	if doc.GetX() > marginX { // flush the unfinished line Write leaves behind
		doc.Ln(lineH)
	}
	doc.Ln(1)
}

// inlineImage draws media[idx] at the current write position. Text-height
// formulas sit in the line (vertically centered on the line box); tall ones
// (multi-storey fractions, systems) get their own line at natural size so they
// stay readable. Marks the index handled on success or alt-text fallback.
func (r *renderer) inlineImage(idx int, m domain.Media, handled map[int]bool) {
	doc := r.doc
	emb := r.embedImage(m)
	if emb == nil {
		if m.Alt != "" {
			doc.Write(lineH, m.Alt)
			handled[idx] = true
		}
		return
	}
	w, h := emb.wMM, emb.hMM
	const maxInlineH = lineH * 1.15
	if h <= maxInlineH*1.35 { // line-sized (or близко): squeeze into the line
		if h > maxInlineH {
			w, h = w*maxInlineH/h, maxInlineH
		}
		if w > contentW {
			h, w = h*contentW/w, contentW
		}
		if doc.GetX()+w > pageW-marginX {
			doc.Ln(lineH)
		}
		if doc.GetY()+h > pageBottom {
			doc.AddPage()
		}
		x, y := doc.GetX(), doc.GetY()
		doc.ImageOptions(emb.name, x, y+(lineH-h)/2, w, h, false, pngOpts, 0, "")
		doc.SetXY(x+w+0.5, y)
	} else { // display-sized: its own line inside the paragraph flow
		if w > contentW {
			h, w = h*contentW/w, contentW
		}
		if h > maxImgH {
			w, h = w*maxImgH/h, maxImgH
		}
		if doc.GetX() > marginX {
			doc.Ln(lineH)
		}
		if doc.GetY()+h > pageBottom {
			doc.AddPage()
		}
		doc.ImageOptions(emb.name, marginX, doc.GetY()+0.7, w, h, false, pngOpts, 0, "")
		doc.SetY(doc.GetY() + h + 1.4)
		doc.SetX(marginX)
	}
	handled[idx] = true
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

		if doc.GetY()+rowH > pageBottom {
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
// formulas the statement already drew (or substituted as alt text) are skipped;
// anything unembeddable degrades to its alt text so no information is silently
// lost.
func (r *renderer) figures(media []domain.Media, handled map[int]bool) {
	doc := r.doc
	var files []domain.Media
	for i, m := range media {
		if m.Kind == "file" {
			files = append(files, m)
			continue
		}
		if handled[i] {
			continue // drawn (or alt-substituted) inside the statement
		}
		if m.Inline && m.Alt != "" {
			continue // substituted as text (e.g. inside a table cell)
		}
		if !r.blockImage(m) && m.Alt != "" {
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

// blockImage draws one figure at the left margin at its natural size (clamped
// to the content box), returning false when it can't be embedded.
func (r *renderer) blockImage(m domain.Media) bool {
	emb := r.embedImage(m)
	if emb == nil {
		return false
	}
	doc := r.doc
	w, h := emb.wMM, emb.hMM
	if w > maxImgW {
		h *= maxImgW / w
		w = maxImgW
	}
	if h > maxImgH {
		w *= maxImgH / h
		h = maxImgH
	}
	if doc.GetY()+h > pageBottom {
		doc.AddPage()
	}
	doc.Ln(1)
	doc.ImageOptions(emb.name, marginX, doc.GetY(), w, h, false, pngOpts, 0, "")
	doc.SetY(doc.GetY() + h + 1)
	return true
}

// embedImage fetches, decodes and registers a media image with the document,
// once per key (cached, incl. failures). Raster formats beyond the stdlib
// (WebP/BMP/TIFF) decode via x/image; SVG — how РЕШУ serves every formula and
// many figures, and the reason PDFs used to come out with no images at all —
// is rasterized. Everything is flattened onto white and re-encoded to a clean
// PNG first: fpdf has no way to recover from a corrupt image (SetError never
// clears), so only bytes fully decoded here are allowed anywhere near the
// document. Returns nil when the image can't be embedded.
func (r *renderer) embedImage(m domain.Media) *embImg {
	if emb, seen := r.imgCache[m.Key]; seen {
		return emb
	}
	emb := r.loadImage(m.Key)
	r.imgCache[m.Key] = emb
	return emb
}

func (r *renderer) loadImage(key string) *embImg {
	if r.opts.FetchImage == nil {
		return nil
	}
	data, err := r.opts.FetchImage(r.ctx, key)
	if err != nil || len(data) == 0 {
		return nil
	}
	var (
		src      image.Image
		wMM, hMM float64
	)
	if looksSVG(data) {
		svg, wPx, hPx, err := rasterizeSVG(data)
		if err != nil {
			return nil
		}
		src, wMM, hMM = svg, wPx*25.4/svgDPI, hPx*25.4/svgDPI
	} else {
		raster, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil
		}
		src = raster
		b := raster.Bounds()
		wMM, hMM = float64(b.Dx())*25.4/imgDPI, float64(b.Dy())*25.4/imgDPI
	}
	if wMM <= 0 || hMM <= 0 {
		return nil
	}
	var clean bytes.Buffer
	if err := png.Encode(&clean, flattenWhite(src)); err != nil {
		return nil
	}
	doc := r.doc
	r.imgN++
	name := fmt.Sprintf("img-%d", r.imgN)
	doc.RegisterImageOptionsReader(name, pngOpts, &clean)
	if doc.Err() {
		return nil
	}
	return &embImg{name: name, wMM: wMM, hMM: hMM}
}

// looksSVG sniffs SVG markup (XML can't be told apart by magic bytes).
func looksSVG(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	trimmed := bytes.TrimLeft(head, " \t\r\n\xef\xbb\xbf")
	return len(trimmed) > 0 && trimmed[0] == '<' &&
		bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// rasterizeSVG renders SVG markup to a raster image at 4× its natural size
// (print-crisp at A4 scale) and reports the natural size in SVG user units.
// oksvg is known to panic on some malformed inputs, so this recovers — a bad
// formula must degrade to alt text, never kill the whole export.
func rasterizeSVG(data []byte) (img image.Image, wPx, hPx float64, err error) {
	defer func() {
		if p := recover(); p != nil {
			img, err = nil, fmt.Errorf("svg rasterize panic: %v", p)
		}
	}()
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.WarnErrorMode)
	if err != nil {
		return nil, 0, 0, err
	}
	wPx, hPx = icon.ViewBox.W, icon.ViewBox.H
	if wPx <= 0 || hPx <= 0 {
		return nil, 0, 0, fmt.Errorf("svg: empty viewBox")
	}
	const scale = 4.0
	pw, ph := int(wPx*scale+0.5), int(hPx*scale+0.5)
	// Cap the raster: formulas are tiny, but a full-page SVG figure at 4× would
	// be enormous. The aspect ratio is preserved.
	const maxPx = 2600
	if pw > maxPx {
		ph = ph * maxPx / pw
		pw = maxPx
	}
	if ph > maxPx {
		pw = pw * maxPx / ph
		ph = maxPx
	}
	if pw < 1 || ph < 1 {
		return nil, 0, 0, fmt.Errorf("svg: degenerate size")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, pw, ph))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)
	icon.SetTarget(0, 0, float64(pw), float64(ph))
	icon.Draw(rasterx.NewDasher(pw, ph, rasterx.NewScannerGV(pw, ph, rgba, rgba.Bounds())), 1)
	if isBlank(rgba) {
		// oksvg skips elements it can't draw (e.g. <text>) with only a warning;
		// an all-white raster means nothing was drawn — alt text beats an
		// invisible formula.
		return nil, 0, 0, fmt.Errorf("svg: rendered blank")
	}
	return rgba, wPx, hPx, nil
}

// isBlank reports whether every pixel is (opaque) white.
func isBlank(img *image.RGBA) bool {
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff || img.Pix[i+1] != 0xff || img.Pix[i+2] != 0xff {
			return false
		}
	}
	return true
}

// flattenWhite composites an image onto a white background — the PDF page is
// white and print must never depend on alpha (mirrors the bot's flattenToWhite
// and the web's always-white figure container).
func flattenWhite(src image.Image) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Over)
	return out
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
		if doc.GetY()+rowH > pageBottom {
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
// formula's text form) — used inside table cells, where images don't flow; a
// placeholder without alt is dropped, its image still renders as a block
// figure below. Statement paragraphs draw the real image instead (flowText).
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
