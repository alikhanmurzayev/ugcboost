package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// SendPulseWebhookPath is the single path SendPulseAuth gates. Defined as a
// package constant so the middleware definition, the OpenAPI document and
// any future router wiring stay in lockstep.
const SendPulseWebhookPath = "/webhooks/sendpulse/instagram"

// sendPulseBearerPrefix is the case-sensitive scheme prefix accepted on the
// SendPulse webhook. Lower-case "bearer " or any other variant fails the
// constant-time compare and gets a 401.
const sendPulseBearerPrefix = "Bearer "

// SendPulseAuth gates the SendPulse Instagram webhook with a constant-time
// bearer-secret check. Other request paths flow through unchanged so the
// middleware can be installed globally on the router without touching the
// rest of the API surface.
//
// Wrong or missing Authorization yields 401 with an empty JSON body — same
// shape as the success 200, so an attacker cannot distinguish the two and
// brute-force the secret. No PII, no error code, no log of the secret.
func SendPulseAuth(secret string, log logger.Logger) func(http.Handler) http.Handler {
	expected := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != SendPulseWebhookPath {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get(headerAuthorization)
			if !strings.HasPrefix(header, sendPulseBearerPrefix) {
				writeSendPulseUnauthorized(w, r, log)
				return
			}
			token := []byte(header[len(sendPulseBearerPrefix):])
			if subtle.ConstantTimeCompare(token, expected) != 1 {
				writeSendPulseUnauthorized(w, r, log)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeSendPulseUnauthorized(w http.ResponseWriter, r *http.Request, log logger.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]any{}); err != nil {
		log.Error(r.Context(), "sendpulse webhook 401 encode failed", "error", err)
	}
}
