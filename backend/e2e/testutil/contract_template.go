package testutil

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/jung-kurt/gofpdf"
	"github.com/stretchr/testify/require"
)

// PutContractTemplate sends a PUT with raw application/pdf body. Returns the
// response — the caller owns Body and MUST close it.
func PutContractTemplate(t *testing.T, path string, pdf []byte, opts ...RawOption) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, BaseURL+path, bytes.NewReader(pdf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/pdf")
	req.ContentLength = int64(len(pdf))
	for _, opt := range opts {
		opt(req)
	}
	resp, err := HTTPClient(nil).Do(req)
	require.NoError(t, err)
	return resp
}

// GetContractTemplate sends a GET expecting an application/pdf or json body.
// Returns the response — caller owns Body.
func GetContractTemplate(t *testing.T, path string, opts ...RawOption) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, BaseURL+path, nil)
	require.NoError(t, err)
	for _, opt := range opts {
		opt(req)
	}
	resp, err := HTTPClient(nil).Do(req)
	require.NoError(t, err)
	return resp
}

// ReadBody pulls the response body into a byte slice and closes it. Useful
// after PutContractTemplate/GetContractTemplate when the test asserts on
// either bytes (PDF) or json shape.
func ReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}

// BuildContractPDF generates an in-memory PDF whose lines carry the supplied
// placeholder names verbatim. Each entry is rendered on its own line as
// `{{Name}}` so the production extractor (gofpdf-rendered Helvetica + the
// ledongthuc/pdf reader) recognises the token.
func BuildContractPDF(t *testing.T, placeholders []string) []byte {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(0, 8, "Contract template (e2e fixture)")
	pdf.Ln(8)
	for _, name := range placeholders {
		pdf.Cell(0, 8, "{{"+name+"}}")
		pdf.Ln(8)
	}
	var buf bytes.Buffer
	require.NoError(t, pdf.Output(&buf))
	return buf.Bytes()
}

// BuildValidContractPDF returns a PDF carrying all three known placeholders
// — passes the upload validator successfully.
func BuildValidContractPDF(t *testing.T) []byte {
	t.Helper()
	return BuildContractPDF(t, []string{"CreatorFIO", "CreatorIIN", "IssuedDate"})
}

