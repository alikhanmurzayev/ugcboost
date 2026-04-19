package testutil

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
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

// checkRetry governs which failures the retry layer re-attempts. The rule is
// narrow on purpose: retry only when the request never reached the app or
// the app was briefly unavailable. Any real application response — including
// 4xx validation errors and 500 from business logic — is a signal to the
// test and must NOT be retried (retrying would mask regressions).
func checkRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		// Transport-layer failure: connection refused, DNS, TLS, timeout.
		return true, nil
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true, nil
	default:
		return false, nil
	}
}

// HTTPClient returns an http.Client with retry + CF Access transport and an
// optional cookie jar. CF Access headers are injected by the inner transport
// so every retry attempt (including after transient failures) carries the
// correct auth envelope.
func HTTPClient(jar http.CookieJar) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient = &http.Client{
		Transport: &cfAccessTransport{base: http.DefaultTransport},
	}
	retryClient.Logger = nil
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 500 * time.Millisecond
	retryClient.RetryWaitMax = 5 * time.Second
	retryClient.CheckRetry = checkRetry

	client := retryClient.StandardClient()
	client.Jar = jar
	return client
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
