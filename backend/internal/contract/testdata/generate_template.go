//go:build ignore

// Generates internal/contract/testdata/template.pdf — fixture со всеми тремя
// плейсхолдерами на одной странице A4. Запускается вручную:
//
//	cd backend
//	go run ./internal/contract/testdata/generate_template.go
//
// Результат коммитим в репо.
package main

import (
	"log"
	"os"

	"github.com/jung-kurt/gofpdf"
)

func main() {
	const fontPath = "/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf"
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		log.Fatalf("read font: %v", err)
	}

	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddUTF8FontFromBytes("liberation", "", fontData)
	pdf.SetFont("liberation", "", 11)
	pdf.AddPage()

	lines := []string{
		"ДОГОВОР № UGC-PILOT",
		"",
		"Креатор: {{CreatorFIO}}",
		"ИИН: {{CreatorIIN}}",
		"Дата выдачи: {{IssuedDate}}",
		"",
		"Подписи сторон ниже.",
	}
	for _, line := range lines {
		pdf.CellFormat(0, 16, line, "", 1, "L", false, 0, "")
	}

	if err := pdf.OutputFileAndClose("internal/contract/testdata/template.pdf"); err != nil {
		log.Fatalf("output: %v", err)
	}
}
