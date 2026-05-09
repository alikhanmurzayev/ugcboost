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

func ReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}

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

func BuildValidContractPDF(t *testing.T) []byte {
	t.Helper()
	return BuildContractPDF(t, []string{"CreatorFIO", "CreatorIIN", "IssuedDate"})
}
