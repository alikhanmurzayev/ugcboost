package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// Recovery catches panics and returns a 500 response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(domain.APIResponse{
					Error: &domain.APIError{
						Code:    domain.CodeInternal,
						Message: "Internal server error",
					},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
