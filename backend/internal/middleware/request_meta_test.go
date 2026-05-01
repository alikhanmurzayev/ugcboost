package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestMeta(t *testing.T) {
	t.Parallel()

	t.Run("captures user agent", func(t *testing.T) {
		t.Parallel()
		var seenUA string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenUA = UserAgentFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")

		RequestMeta(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, "Mozilla/5.0", seenUA)
	})

	t.Run("user agent truncated to MaxUserAgentLength", func(t *testing.T) {
		t.Parallel()
		var seenUA string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenUA = UserAgentFromContext(r.Context())
		})

		oversized := make([]byte, MaxUserAgentLength+128)
		for i := range oversized {
			oversized[i] = 'a'
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", string(oversized))

		RequestMeta(next).ServeHTTP(httptest.NewRecorder(), req)

		require.Len(t, seenUA, MaxUserAgentLength)
	})
}

func TestUserAgentFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns stored value", func(t *testing.T) {
		t.Parallel()
		ctx := WithUserAgent(context.Background(), "explicit-ua")
		require.Equal(t, "explicit-ua", UserAgentFromContext(ctx))
	})

	t.Run("returns empty when absent", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, UserAgentFromContext(context.Background()))
	})
}
