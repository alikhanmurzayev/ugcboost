package contract

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
	"github.com/stretchr/testify/require"
)

func loadTemplate(t *testing.T) []byte {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	path := filepath.Join(wd, "testdata", "template.pdf")
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	return bytes
}

// pdfText reads all glyphs from PDF bytes and concatenates their .S values
// in default reader order. Good enough for substring assertions on rendered
// content. Reads only page 1 since fixture is single-page.
func pdfText(t *testing.T, b []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)
	var sb strings.Builder
	for n := 1; n <= r.NumPage(); n++ {
		page := r.Page(n)
		if page.V.IsNull() {
			continue
		}
		for _, c := range page.Content().Text {
			sb.WriteString(c.S)
		}
	}
	return sb.String()
}

func TestRealRenderer_Render_FillsPlaceholders(t *testing.T) {
	t.Parallel()

	tpl := loadTemplate(t)
	r := NewRealRenderer(nil)

	out, err := r.Render(tpl, ContractData{
		CreatorFIO: "Иванов Иван Иванович",
		CreatorIIN: "880101300123",
		IssuedDate: "«9» мая 2026 г.",
	})
	require.NoError(t, err)
	require.NotEmpty(t, out)

	text := pdfText(t, out)
	require.Contains(t, text, "Иванов Иван Иванович")
	require.Contains(t, text, "880101300123")
	require.Contains(t, text, "«9» мая 2026 г.")
	// placeholders themselves must be covered by overlay rectangles — but
	// the underlying template still carries them as text in the imported
	// page, so we just check that rendered values were drawn on top.
}

func TestRealRenderer_Render_EmptyTemplate(t *testing.T) {
	t.Parallel()
	r := NewRealRenderer(nil)
	_, err := r.Render(nil, ContractData{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty template")
}

func TestRealRenderer_Render_SkipMissingValues(t *testing.T) {
	t.Parallel()
	tpl := loadTemplate(t)
	r := NewRealRenderer(nil)

	// IssuedDate empty — overlay just leaves the placeholder text untouched.
	out, err := r.Render(tpl, ContractData{
		CreatorFIO: "Иванов Иван",
		CreatorIIN: "880101300123",
	})
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

func TestRealExtractor_FindsPlaceholders(t *testing.T) {
	t.Parallel()
	tpl := loadTemplate(t)
	ex := NewRealExtractor()
	got, err := ex.ExtractPlaceholders(tpl)
	require.NoError(t, err)

	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	require.True(t, names["CreatorFIO"], "CreatorFIO not found")
	require.True(t, names["CreatorIIN"], "CreatorIIN not found")
	require.True(t, names["IssuedDate"], "IssuedDate not found")
}
