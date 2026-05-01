package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// RawOption mutates the request before it is sent. Used to attach headers
// or other transport-level details that would not survive the generated
// client's request shaping.
type RawOption func(*http.Request)

// WithHeader sets a header on the raw request. Suitable for attaching
// proxy-chain markers like CF-Connecting-IP that the backend's RealIP
// middleware should pick up.
func WithHeader(key, value string) RawOption {
	return func(r *http.Request) {
		r.Header.Set(key, value)
	}
}

// PostRaw sends a POST to BaseURL+path with body marshalled as JSON, using
// the shared retry + CF-Access transport. Use this ONLY when a test must
// deliberately submit input that the generated client rejects client-side
// (empty email, non-enum values) — the generated openapi types validate
// formats before the request ever hits the wire, so testing the backend's
// own validation requires sidestepping that check. Every call site MUST be
// annotated with a short comment explaining why raw is needed.
//
// The caller owns the response — always defer resp.Body.Close().
func PostRaw(t *testing.T, path string, body any, opts ...RawOption) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, BaseURL+path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for _, opt := range opts {
		opt(req)
	}
	resp, err := HTTPClient(nil).Do(req)
	require.NoError(t, err)
	return resp
}
