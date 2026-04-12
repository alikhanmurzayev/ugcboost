package testutil

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
)

// BaseURL is the API base URL, configurable via E2E_BASE_URL env var.
var BaseURL = getBaseURL()

func getBaseURL() string {
	if url := os.Getenv("E2E_BASE_URL"); url != "" {
		return url
	}
	return "http://localhost:8082"
}

// cfAccessTransport injects Cloudflare Access headers when CF_ACCESS_CLIENT_ID is set.
type cfAccessTransport struct {
	base http.RoundTripper
}

func (t *cfAccessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if id := os.Getenv("CF_ACCESS_CLIENT_ID"); id != "" {
		req.Header.Set("CF-Access-Client-Id", id)
		req.Header.Set("CF-Access-Client-Secret", os.Getenv("CF_ACCESS_CLIENT_SECRET"))
	}
	return t.base.RoundTrip(req)
}

// HTTPClient returns an http.Client with CF Access transport and optional cookie jar.
func HTTPClient(jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:       jar,
		Transport: &cfAccessTransport{base: http.DefaultTransport},
	}
}

// NewAPIClient creates an API client with cookie jar for refresh token support.
func NewAPIClient(t *testing.T) *apiclient.ClientWithResponses {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c, err := apiclient.NewClientWithResponses(BaseURL,
		apiclient.WithHTTPClient(HTTPClient(jar)))
	require.NoError(t, err)
	return c
}

// NewTestClient creates a client for test-only endpoints.
func NewTestClient(t *testing.T) *testclient.ClientWithResponses {
	t.Helper()
	c, err := testclient.NewClientWithResponses(BaseURL,
		testclient.WithHTTPClient(HTTPClient(nil)))
	require.NoError(t, err)
	return c
}

// WithAuth adds an Authorization: Bearer header to requests.
func WithAuth(token string) apiclient.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}
