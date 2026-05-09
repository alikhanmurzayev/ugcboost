package contract

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

type Placeholder struct {
	Page      int
	Name      string
	XMin      float64
	XMax      float64
	YBaseline float64
	FontSize  float64
}

type Extractor interface {
	ExtractPlaceholders(pdfBytes []byte) ([]Placeholder, error)
}

type RealExtractor struct{}

func NewRealExtractor() *RealExtractor { return &RealExtractor{} }

var placeholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

// lineTolerance — Y-окно в PDF-точках для склейки глифов в одну логическую
// линию. У шаблонов Аиданы глифы кластеризуются куда плотнее 0.5pt, при этом
// 0.5pt не съедает соседние baseline'ы.
const lineTolerance = 0.5

// ExtractPlaceholders walks every page, splits glyphs into lines / words и
// собирает все `{{Name}}` (включая `{{A}}{{B}}` без пробела — несколько
// токенов в одном слове). Bounding box нужен chunk 16 outbox-renderer'у.
func (e *RealExtractor) ExtractPlaceholders(pdfBytes []byte) ([]Placeholder, error) {
	reader := bytes.NewReader(pdfBytes)
	pdfDoc, err := pdf.NewReader(reader, int64(len(pdfBytes)))
	if err != nil {
		return nil, fmt.Errorf("pdf.NewReader: %w", err)
	}

	totalPages := pdfDoc.NumPage()
	var found []Placeholder
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := pdfDoc.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		chars := page.Content().Text
		if len(chars) == 0 {
			continue
		}
		for _, line := range groupByLine(chars) {
			for _, w := range splitWords(line) {
				for _, m := range placeholderRE.FindAllStringSubmatch(w.text, -1) {
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
	return found, nil
}

func groupByLine(chars []pdf.Text) [][]pdf.Text {
	if len(chars) == 0 {
		return nil
	}
	indexed := make([]pdf.Text, len(chars))
	copy(indexed, chars)
	sort.SliceStable(indexed, func(i, j int) bool {
		if math.Abs(indexed[i].Y-indexed[j].Y) < lineTolerance {
			return indexed[i].X < indexed[j].X
		}
		return indexed[i].Y > indexed[j].Y
	})

	var lines [][]pdf.Text
	var cur []pdf.Text
	for _, c := range indexed {
		if len(cur) == 0 || math.Abs(c.Y-cur[0].Y) < lineTolerance {
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

// splitWords splits a line on whitespace. ledongthuc/pdf не отдаёт word advance —
// xMax для слов внутри строки берём из X следующего whitespace-glyph'а; для
// trailing word оцениваем по fontSize·0.5·runes (в пределах 1–2pt — хватает
// под bbox-overlay).
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
