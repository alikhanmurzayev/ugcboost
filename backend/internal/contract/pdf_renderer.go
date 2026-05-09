package contract

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"github.com/signintech/gopdf"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
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
	// overflowSlack — допуск для warn-on-overflow: 5% сверх bbox считаются
	// нормой (anti-alias halo + микро-дрейф kerning'а). Только при превышении
	// этого порога пишем warning лог.
	overflowSlack = 1.05
)

// Renderer накладывает overlay на шаблон договора per Decision #12.
type Renderer interface {
	Render(template []byte, data ContractData) ([]byte, error)
}

// RealRenderer — продакшн-реализация на signintech/gopdf + ledongthuc/pdf
// (через RealExtractor). Без сети, без CGo.
type RealRenderer struct {
	extractor Extractor
	logger    logger.Logger
}

// NewRealRenderer собирает рендерер. extractor=nil → новый RealExtractor.
// log=nil допустим — overflow-warning'и тогда не пишутся (но render продолжает
// работать, см. Open Forks в intent-trustme-contract-v2).
func NewRealRenderer(extractor Extractor, log logger.Logger) *RealRenderer {
	if extractor == nil {
		extractor = NewRealExtractor()
	}
	return &RealRenderer{extractor: extractor, logger: log}
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
			r.warnOnOverflow(&out, ph, pn, value, width)
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

// warnOnOverflow логирует Warn, если value визуально не влезает в bbox
// плейсхолдера (gopdf.MeasureTextWidth > width). PII (само value) не пишем —
// только rune count, имя плейсхолдера, page и геометрия. Текущая политика —
// warn-only (см. Open Forks в intent-v2): креатор всё равно подписывает
// договор, оператор разбирает кейсы overflow по логам вручную.
func (r *RealRenderer) warnOnOverflow(out *gopdf.GoPdf, ph Placeholder, page int, value string, bboxWidth float64) {
	if r.logger == nil {
		return
	}
	w, err := out.MeasureTextWidth(value)
	if err != nil || w <= bboxWidth*overflowSlack {
		return
	}
	r.logger.Warn(context.Background(), "contract: pdf overlay value overflows placeholder bbox",
		"placeholder", ph.Name,
		"page", page,
		"value_runes", utf8.RuneCountInString(value),
		"value_width_pt", w,
		"bbox_width_pt", bboxWidth,
		"font_size", ph.FontSize,
	)
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
