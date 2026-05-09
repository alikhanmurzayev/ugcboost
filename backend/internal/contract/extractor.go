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

// Placeholder describes a single `{{Name}}` token found in a PDF, with its
// page number and bounding box in PDF coordinates (origin bottom-left). The
// chunk-9a upload validator only consumes Name and Page; chunk 16's overlay
// renderer reuses the geometric fields to lay rendered text over the
// original placeholder.
type Placeholder struct {
	Page      int
	Name      string
	XMin      float64
	XMax      float64
	YBaseline float64
	FontSize  float64
}

// Extractor pulls placeholder occurrences out of a PDF byte stream. The
// interface lets service-layer tests swap in a mock without touching real
// PDF parsing, while production wiring uses RealExtractor.
type Extractor interface {
	ExtractPlaceholders(pdfBytes []byte) ([]Placeholder, error)
}

// RealExtractor implements Extractor on top of github.com/ledongthuc/pdf.
// Stateless — safe to share across goroutines and reuse between requests.
type RealExtractor struct{}

// NewRealExtractor returns the default Extractor wired with ledongthuc/pdf.
func NewRealExtractor() *RealExtractor { return &RealExtractor{} }

// placeholderRE matches `{{Name}}` tokens where Name is a word character run.
var placeholderRE = regexp.MustCompile(`\{\{(\w+)\}\}`)

// lineTolerance — Y-coordinate window in PDF points used to merge characters
// into one logical line. Aидана's reference template clusters glyphs within
// well under 0.5pt of each other, so 0.5pt is generous yet tight enough to
// keep adjacent baselines apart.
const lineTolerance = 0.5

// ExtractPlaceholders parses pdfBytes and returns every placeholder
// occurrence (one entry per page-position; a placeholder repeated across
// pages produces multiple results). Returns a wrapped error when
// ledongthuc/pdf cannot parse the bytes — the service translates that into
// a domain CONTRACT_INVALID_PDF response.
//
// The algorithm mirrors `_bmad-output/experiments/pdf-overlay/main.go`:
// characters are grouped into lines by Y proximity, words are split on
// whitespace within a line, and each word is matched against the
// `{{Name}}` regex. ledongthuc/pdf does not report word widths so the
// closing X is taken from the next whitespace character (or estimated from
// font size as a fallback for the trailing word).
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
	return found, nil
}

// groupByLine clusters glyphs into lines by Y-coordinate proximity.
// Within a line the slice is sorted left-to-right by X. Lines are emitted
// top-to-bottom (descending Y) — page coordinates have origin at bottom-left.
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

// wordBox carries one whitespace-delimited token within a line plus its
// bounding box (X bounds + baseline + font size).
type wordBox struct {
	text      string
	xMin      float64
	xMax      float64
	yBaseline float64
	fontSize  float64
}

// splitWords walks one line of glyphs and emits whitespace-delimited words.
// ledongthuc/pdf does not report word advance, so the closing X for a word
// is the X of the next whitespace glyph; for the trailing word we estimate
// width from font size and rune count (within 1-2pt for monospace and
// proportional fonts alike — sufficient for placeholder-bbox usage).
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
