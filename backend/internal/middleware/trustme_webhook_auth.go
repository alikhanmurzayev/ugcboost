package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// TrustMeWebhookPath gates the TrustMe webhook receiver. Mirrors the
// SendPulse middleware so package consts and openapi.yaml stay in lockstep.
const TrustMeWebhookPath = "/trustme/webhook"

// TrustMeWebhookAuth gates POST /trustme/webhook with a constant-time
// compare of the static token. Per TrustMe blueprint § «Установка хуков»
// the header is `Authorization: <token>` (raw, без `Bearer`-префикса) —
// формат не настраивается на их стороне.
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
			token := []byte(r.Header.Get(headerAuthorization))
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
