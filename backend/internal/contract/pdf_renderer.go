package contract

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"

	"github.com/ledongthuc/pdf"
	"github.com/signintech/gopdf"
)

// LiberationSerif-Regular.ttf — embedded TTF, метрики совместимы с Times New
// Roman (которые Google Docs использует для русского). Через embed.FS файл
// попадает в бинарь — на системные шрифты не полагаемся (Docker-image может
// быть minimal).
//
//go:embed fonts/LiberationSerif-Regular.ttf
var liberationSerifTTF []byte

// ContractData — три типизированных поля per Decision #13.
type ContractData struct {
	CreatorFIO string
	CreatorIIN string
	IssuedDate string
}

func (d ContractData) get(name string) string {
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

// Параметры overlay-рендера (зафиксированы экспериментом, см. intent v2 §
// «Параметры overlay-рендера»).
const (
	pageWidth    = 596.0
	pageHeight   = 842.0
	ascentRatio  = 0.75
	descentRatio = 0.27
	pad          = 1.0
)

// Renderer накладывает overlay на шаблон договора per Decision #12.
type Renderer interface {
	Render(template []byte, data ContractData) ([]byte, error)
}

// RealRenderer — продакшн-реализация на signintech/gopdf + ledongthuc/pdf
// (через RealExtractor). Без сети, без CGo.
type RealRenderer struct {
	extractor Extractor
}

// NewRealRenderer собирает рендерер. extractor=nil → новый RealExtractor.
func NewRealRenderer(extractor Extractor) *RealRenderer {
	if extractor == nil {
		extractor = NewRealExtractor()
	}
	return &RealRenderer{extractor: extractor}
}

// Render накладывает overlay поверх template и возвращает результат.
// signintech/gopdf требует ImportPage по file path — проксируем template
// через temp-файл, который удаляем сразу после WritePdf'а.
func (r *RealRenderer) Render(template []byte, data ContractData) ([]byte, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("contract: empty template")
	}
	placeholders, err := r.extractor.ExtractPlaceholders(template)
	if err != nil {
		return nil, fmt.Errorf("contract: extract: %w", err)
	}

	tplFile, err := os.CreateTemp("", "ugc-template-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("contract: tempfile: %w", err)
	}
	tplPath := tplFile.Name()
	defer func() { _ = os.Remove(tplPath) }()
	if _, err := tplFile.Write(template); err != nil {
		_ = tplFile.Close()
		return nil, fmt.Errorf("contract: write tempfile: %w", err)
	}
	if err := tplFile.Close(); err != nil {
		return nil, fmt.Errorf("contract: close tempfile: %w", err)
	}

	totalPages, err := readNumPages(template)
	if err != nil {
		return nil, fmt.Errorf("contract: page count: %w", err)
	}
	if max := maxPage(placeholders); max > totalPages {
		totalPages = max
	}

	out := gopdf.GoPdf{}
	out.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageWidth, H: pageHeight}})
	if err := out.AddTTFFontByReader("body", bytes.NewReader(liberationSerifTTF)); err != nil {
		return nil, fmt.Errorf("contract: add font: %w", err)
	}

	byPage := map[int][]Placeholder{}
	for _, p := range placeholders {
		byPage[p.Page] = append(byPage[p.Page], p)
	}

	for pn := 1; pn <= totalPages; pn++ {
		tpl := out.ImportPage(tplPath, pn, "/MediaBox")
		out.AddPage()
		out.UseImportedTemplate(tpl, 0, 0, pageWidth, pageHeight)

		for _, ph := range byPage[pn] {
			value := data.get(ph.Name)
			if value == "" {
				continue
			}
			yGlyphTop := pageHeight - (ph.YBaseline + ph.FontSize*ascentRatio)
			width := ph.XMax - ph.XMin

			out.SetFillColor(255, 255, 255)
			out.RectFromUpperLeftWithStyle(
				ph.XMin, yGlyphTop-pad, width,
				(ascentRatio+descentRatio)*ph.FontSize+2*pad,
				"F",
			)

			out.SetFillColor(0, 0, 0)
			if err := out.SetFont("body", "", ph.FontSize); err != nil {
				return nil, fmt.Errorf("contract: set font: %w", err)
			}
			out.SetX(ph.XMin)
			out.SetY(yGlyphTop)
			if err := out.Cell(nil, value); err != nil {
				return nil, fmt.Errorf("contract: draw cell: %w", err)
			}
		}
	}

	var buf bytes.Buffer
	if _, err := out.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("contract: write pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func maxPage(placeholders []Placeholder) int {
	max := 0
	for _, p := range placeholders {
		if p.Page > max {
			max = p.Page
		}
	}
	return max
}

func readNumPages(template []byte) (int, error) {
	r, err := pdf.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return 0, err
	}
	return r.NumPage(), nil
}
