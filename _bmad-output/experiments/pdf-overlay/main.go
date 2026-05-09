// PDF overlay experiment for the TrustMe contract pipeline.
//
// Standalone sandbox — lives outside the ugcboost repo. Once the approach is
// validated, the production version will go through PR with deps registered
// in docs/standards/backend-libraries.md.
//
// Usage:
//
//	cd ~/pdf-overlay-experiment
//	go run . \
//	    -in '/home/alikhan/projects/ugcboost/legal-documents/Тест, эксперимент BLACK BURN_Соглашение_UGC.docx-1.pdf' \
//	    -out /tmp/contract-out.pdf
package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/signintech/gopdf"
)

type ContractData struct {
	CreatorFIO string
	CreatorIIN string
	IssuedDate string
}

func (d ContractData) Get(name string) string {
	switch name {
	case "CreatorFIO":
		return d.CreatorFIO
	case "CreatorIIN":
		return d.CreatorIIN
	case "IssuedDate":
		return d.IssuedDate
	default:
		return ""
	}
}

type Placeholder struct {
	Page      int
	Name      string
	XMin      float64
	XMax      float64
	YBaseline float64
	FontSize  float64
}

var placeholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

func extractPlaceholders(path string) ([]Placeholder, int, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	totalPages := r.NumPage()
	var found []Placeholder

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		chars := page.Content().Text
		if len(chars) == 0 {
			continue
		}
		for _, line := range groupByLine(chars) {
			for _, w := range splitWords(line) {
				if m := placeholderRE.FindStringSubmatch(w.text); m != nil {
					found = append(found, Placeholder{
						Page:      pageNum,
						Name:      m[1],
						XMin:      w.xMin,
						XMax:      w.xMax,
						YBaseline: w.yBaseline,
						FontSize:  w.fontSize,
					})
				}
			}
		}
	}
	return found, totalPages, nil
}

func groupByLine(chars []pdf.Text) [][]pdf.Text {
	if len(chars) == 0 {
		return nil
	}
	indexed := make([]pdf.Text, len(chars))
	copy(indexed, chars)
	sort.SliceStable(indexed, func(i, j int) bool {
		if abs(indexed[i].Y-indexed[j].Y) < 0.5 {
			return indexed[i].X < indexed[j].X
		}
		return indexed[i].Y > indexed[j].Y
	})

	var lines [][]pdf.Text
	var cur []pdf.Text
	for _, c := range indexed {
		if len(cur) == 0 || abs(c.Y-cur[0].Y) < 0.5 {
			cur = append(cur, c)
			continue
		}
		lines = append(lines, cur)
		cur = []pdf.Text{c}
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return lines
}

type wordBox struct {
	text      string
	xMin      float64
	xMax      float64
	yBaseline float64
	fontSize  float64
}

func splitWords(line []pdf.Text) []wordBox {
	var words []wordBox
	var cur wordBox
	started := false

	flush := func(xEnd float64) {
		if !started {
			return
		}
		cur.xMax = xEnd
		words = append(words, cur)
		started = false
	}

	for _, c := range line {
		if strings.TrimSpace(c.S) == "" {
			flush(c.X)
			continue
		}
		if !started {
			cur = wordBox{
				text:      c.S,
				xMin:      c.X,
				yBaseline: c.Y,
				fontSize:  c.FontSize,
			}
			started = true
		} else {
			cur.text += c.S
		}
	}
	if started {
		cur.xMax = cur.xMin + cur.fontSize*0.5*float64(len([]rune(cur.text)))
		words = append(words, cur)
	}
	return words
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

const (
	pageWidth  = 596.0
	pageHeight = 842.0
)

func renderOverlay(inputPath, outputPath, fontPath string, data ContractData, placeholders []Placeholder, totalPages int) error {
	out := gopdf.GoPdf{}
	out.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageWidth, H: pageHeight}})

	if err := out.AddTTFFont("body", fontPath); err != nil {
		return fmt.Errorf("add font %s: %w", fontPath, err)
	}

	byPage := map[int][]Placeholder{}
	for _, p := range placeholders {
		byPage[p.Page] = append(byPage[p.Page], p)
	}

	for pn := 1; pn <= totalPages; pn++ {
		tpl := out.ImportPage(inputPath, pn, "/MediaBox")
		out.AddPage()
		out.UseImportedTemplate(tpl, 0, 0, pageWidth, pageHeight)

		for _, ph := range byPage[pn] {
			value := data.Get(ph.Name)
			if value == "" {
				log.Printf("warn: no value for {{%s}} on page %d", ph.Name, pn)
				continue
			}
			// ledongthuc Y is the glyph baseline in bottom-left origin.
			// For serif fonts (Times New Roman, Liberation Serif):
			//   ascent  ≈ 0.80 × FontSize (above baseline)
			//   descent ≈ 0.22 × FontSize (below baseline)
			// Convert to top-left used by gopdf.
			const ascentRatio = 0.75
			const descentRatio = 0.27
			yGlyphTop := pageHeight - (ph.YBaseline + ph.FontSize*ascentRatio)
			yGlyphBot := pageHeight - (ph.YBaseline - ph.FontSize*descentRatio)
			glyphH := yGlyphBot - yGlyphTop
			w := ph.XMax - ph.XMin

			// White rect must cover ascender AND descender of the original
			// placeholder; small pad covers anti-alias halo.
			pad := 1.0
			out.SetFillColor(255, 255, 255)
			out.RectFromUpperLeftWithStyle(ph.XMin, yGlyphTop-pad, w, glyphH+2*pad, "F")

			out.SetFillColor(0, 0, 0)
			if err := out.SetFont("body", "", ph.FontSize); err != nil {
				return fmt.Errorf("set font: %w", err)
			}
			out.SetX(ph.XMin)
			out.SetY(yGlyphTop)
			if err := out.Cell(nil, value); err != nil {
				return fmt.Errorf("cell: %w", err)
			}
		}
	}

	return out.WritePdf(outputPath)
}

func main() {
	in := flag.String("in", "", "input PDF template path")
	out := flag.String("out", "/tmp/contract-out.pdf", "output PDF path")
	font := flag.String("font", "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf", "TTF font path for overlay text (must contain Cyrillic)")
	fio := flag.String("fio", "Иванов Иван Иванович", "value for {{CreatorFIO}}")
	iin := flag.String("iin", "880101300123", "value for {{CreatorIIN}}")
	date := flag.String("date", "«9» мая 2026 г.", "value for {{IssuedDate}}")
	flag.Parse()

	if *in == "" {
		log.Fatal("--in is required")
	}

	placeholders, totalPages, err := extractPlaceholders(*in)
	if err != nil {
		log.Fatalf("extract: %v", err)
	}

	log.Printf("total pages: %d", totalPages)
	log.Printf("found %d placeholders:", len(placeholders))
	for _, p := range placeholders {
		log.Printf("  p%d {{%s}} x=%.1f..%.1f baseline=%.1f size=%.1f",
			p.Page, p.Name, p.XMin, p.XMax, p.YBaseline, p.FontSize)
	}

	data := ContractData{
		CreatorFIO: *fio,
		CreatorIIN: *iin,
		IssuedDate: *date,
	}

	if err := renderOverlay(*in, *out, *font, data, placeholders, totalPages); err != nil {
		log.Fatalf("render: %v", err)
	}
	log.Printf("wrote %s", *out)
}
