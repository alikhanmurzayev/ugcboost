package middleware

import (
	"bytes"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// TrustMeWebhookDebugDump — TEMPORARY debug middleware. Логирует raw входящий
// запрос (method, path, query, ВСЕ заголовки включая Authorization, body)
// для `/trustme/webhook` ДО проверки auth, чтобы вытащить, как именно
// TrustMe-сервер шлёт payload (заголовок, формат). Удалить после дебага.
//
// Не использовать на проде с PII в проде надолго: Authorization содержит
// секрет, body содержит client/contract_url.
func TrustMeWebhookDebugDump(log logger.Logger) func(http.Handler) http.Handler {
	const maxDump = 64 * 1024
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != TrustMeWebhookPath {
				next.ServeHTTP(w, r)
				return
			}
			body, _ := io.ReadAll(io.LimitReader(r.Body, maxDump+1))
			truncated := len(body) > maxDump
			if truncated {
				body = body[:maxDump]
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))

			keys := make([]string, 0, len(r.Header))
			for k := range r.Header {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var sb strings.Builder
			for _, k := range keys {
				sb.WriteString(k)
				sb.WriteString(": ")
				sb.WriteString(strings.Join(r.Header.Values(k), "; "))
				sb.WriteString("\n")
			}

			log.Info(r.Context(), "trustme webhook DEBUG dump",
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"remote_addr", r.RemoteAddr,
				"content_length", r.ContentLength,
				"headers", sb.String(),
				"body", string(body),
				"body_truncated", truncated,
			)
			next.ServeHTTP(w, r)
		})
	}
}
