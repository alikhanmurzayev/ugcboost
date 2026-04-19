package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// Recovery catches panics and returns a 500 response.
func Recovery(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error(r.Context(), "panic recovered",
						"panic", rec,
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
					)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(api.ErrorResponse{
						Error: api.APIError{
							Code:    domain.CodeInternal,
							Message: "Internal server error",
						},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
