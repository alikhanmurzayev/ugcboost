package contract

import (
	"bytes"
	"sort"
	"testing"

	"github.com/jung-kurt/gofpdf"
	"github.com/stretchr/testify/require"
)

func TestRealExtractor_ExtractPlaceholders(t *testing.T) {
	t.Parallel()

	t.Run("invalid PDF — text payload", func(t *testing.T) {
		t.Parallel()
		extractor := NewRealExtractor()
		_, err := extractor.ExtractPlaceholders([]byte("not a pdf"))
		require.Error(t, err)
		require.ErrorContains(t, err, "pdf.NewReader")
	})

	t.Run("empty bytes", func(t *testing.T) {
		t.Parallel()
		extractor := NewRealExtractor()
		_, err := extractor.ExtractPlaceholders(nil)
		require.Error(t, err)
	})

	t.Run("PDF without placeholders", func(t *testing.T) {
		t.Parallel()
		pdfBytes := buildPDF(t, [][]string{
			{"Договор на оказание услуг"},
			{"Сторона 1: ABC LLP"},
			{"Сторона 2: креатор"},
		})
		extractor := NewRealExtractor()
		got, err := extractor.ExtractPlaceholders(pdfBytes)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("PDF with all three known placeholders, single page", func(t *testing.T) {
		t.Parallel()
		pdfBytes := buildPDF(t, [][]string{
			{"Contract template"},
			{"Creator: {{CreatorFIO}}"},
			{"IIN: {{CreatorIIN}}"},
			{"Issued: {{IssuedDate}}"},
		})
		extractor := NewRealExtractor()
		got, err := extractor.ExtractPlaceholders(pdfBytes)
		require.NoError(t, err)
		names := uniquePlaceholderNames(got)
		require.ElementsMatch(t, []string{"CreatorFIO", "CreatorIIN", "IssuedDate"}, names)
		for _, p := range got {
			require.Equal(t, 1, p.Page)
			require.Greater(t, p.FontSize, 0.0)
		}
	})

	t.Run("placeholder repeated across pages produces multiple entries", func(t *testing.T) {
		t.Parallel()
		pdfBytes := buildMultiPagePDF(t, [][][]string{
			{{"Page 1"}, {"{{CreatorFIO}}"}},
			{{"Page 2"}, {"{{CreatorFIO}}"}, {"{{CreatorIIN}}"}},
		})
		extractor := NewRealExtractor()
		got, err := extractor.ExtractPlaceholders(pdfBytes)
		require.NoError(t, err)
		require.Len(t, got, 3)
		var page1, page2 int
		for _, p := range got {
			if p.Page == 1 {
				page1++
			}
			if p.Page == 2 {
				page2++
			}
		}
		require.Equal(t, 1, page1)
		require.Equal(t, 2, page2)
	})

	t.Run("PDF with unknown placeholder is reported as-is — Validate decides", func(t *testing.T) {
		t.Parallel()
		pdfBytes := buildPDF(t, [][]string{
			{"{{CreatorFIO}}"},
			{"{{CreatorEmail}}"},
		})
		extractor := NewRealExtractor()
		got, err := extractor.ExtractPlaceholders(pdfBytes)
		require.NoError(t, err)
		names := uniquePlaceholderNames(got)
		require.ElementsMatch(t, []string{"CreatorFIO", "CreatorEmail"}, names)
	})

	t.Run("multiple placeholders on one line — both space-separated and glued", func(t *testing.T) {
		t.Parallel()
		pdfBytes := buildPDF(t, [][]string{
			{"{{CreatorFIO}} {{CreatorIIN}}"},
			{"{{IssuedDate}}{{CreatorFIO}}"},
		})
		extractor := NewRealExtractor()
		got, err := extractor.ExtractPlaceholders(pdfBytes)
		require.NoError(t, err)
		var names []string
		for _, p := range got {
			names = append(names, p.Name)
		}
		require.ElementsMatch(t, []string{"CreatorFIO", "CreatorIIN", "IssuedDate", "CreatorFIO"}, names)
	})
}

func buildPDF(t *testing.T, lines [][]string) []byte {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	for _, line := range lines {
		pdf.Cell(0, 8, joinLine(line))
		pdf.Ln(8)
	}
	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))
	return buf.Bytes()
}

func buildMultiPagePDF(t *testing.T, pages [][][]string) []byte {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	for _, lines := range pages {
		pdf.AddPage()
		for _, line := range lines {
			pdf.Cell(0, 8, joinLine(line))
			pdf.Ln(8)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))
	return buf.Bytes()
}

func joinLine(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func uniquePlaceholderNames(ps []Placeholder) []string {
	seen := make(map[string]struct{}, len(ps))
	for _, p := range ps {
		seen[p.Name] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
