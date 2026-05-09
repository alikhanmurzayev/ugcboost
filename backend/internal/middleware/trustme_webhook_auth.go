package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// TrustMeWebhookPath gates the TrustMe webhook receiver. Mirrors the
// SendPulse middleware so package consts and openapi.yaml stay in lockstep.
const TrustMeWebhookPath = "/trustme/webhook"

// trustMeBearerPrefix — RFC 6750 scheme. TrustMe шлёт `Authorization: Bearer
// <token>` per actual prod-cabinet behavior. Scheme match — case-insensitive
// (RFC 6750 + защита от случайной смены капитализации со стороны TrustMe).
const trustMeBearerPrefix = "Bearer "

// TrustMeWebhookAuth gates POST /trustme/webhook with a constant-time
// compare of the static token. TrustMe wire-format — `Authorization: Bearer
// <token>` (scheme case-insensitive). Same token заводится в кабинете
// TrustMe при создании вебхука и в `TRUSTME_WEBHOOK_TOKEN` env var.
//
// Wrong or missing Authorization yields 401 with an empty JSON body —
// same shape as the success 200, so an attacker cannot distinguish the
// two and brute-force the secret. No PII, no error code.
//
// Empty configured secret always denies (defense-in-depth). config.Load()
// requires a non-empty token outside `local`; the middleware fails closed
// even if a misconfigured local boot reaches here — `subtle.ConstantTimeCompare`
// of two empty byte slices returns 1 (a match), which would silently
// open the endpoint to any anonymous request.
func TrustMeWebhookAuth(secret string, log logger.Logger) func(http.Handler) http.Handler {
	expected := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != TrustMeWebhookPath {
				next.ServeHTTP(w, r)
				return
			}
			if len(expected) == 0 {
				writeTrustMeWebhookUnauthorized(w, r, log)
				return
			}
			header := r.Header.Get(headerAuthorization)
			if len(header) < len(trustMeBearerPrefix) ||
				!strings.EqualFold(header[:len(trustMeBearerPrefix)], trustMeBearerPrefix) {
				writeTrustMeWebhookUnauthorized(w, r, log)
				return
			}
			token := []byte(header[len(trustMeBearerPrefix):])
			if subtle.ConstantTimeCompare(token, expected) != 1 {
				writeTrustMeWebhookUnauthorized(w, r, log)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeTrustMeWebhookUnauthorized(w http.ResponseWriter, r *http.Request, log logger.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]any{}); err != nil {
		log.Error(r.Context(), "trustme webhook 401 encode failed", "error", err)
	}
}
