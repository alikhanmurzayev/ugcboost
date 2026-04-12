package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

type contextKey string

const (
	ContextKeyUserID contextKey = "userID"
	ContextKeyRole   contextKey = "role"
)

// TokenValidator validates a JWT and returns claims.
type TokenValidator interface {
	ValidateAccessToken(tokenStr string) (userID string, role string, err error)
}

// Auth returns middleware that extracts and validates JWT from the Authorization header.
func Auth(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Missing authorization header"},
				})
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Invalid authorization header format"},
				})
				return
			}

			userID, role, err := validator.ValidateAccessToken(parts[1])
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Invalid or expired token"},
				})
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, ContextKeyRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks the user has one of the allowed roles.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if !allowedSet[role] {
				writeJSON(w, http.StatusForbidden, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeForbidden, Message: "Insufficient permissions"},
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthFromScopes returns middleware that validates JWT only when BearerAuthScopes is present in context.
// The generated ServerInterfaceWrapper sets this context value for protected endpoints.
func AuthFromScopes(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this route requires auth (set by generated wrapper)
			if r.Context().Value(api.BearerAuthScopes) == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Same auth logic as existing Auth middleware
			header := r.Header.Get("Authorization")
			if header == "" {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Missing authorization header"},
				})
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Invalid authorization header format"},
				})
				return
			}

			userID, role, err := validator.ValidateAccessToken(parts[1])
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, domain.APIResponse{
					Error: &domain.APIError{Code: domain.CodeUnauthorized, Message: "Invalid or expired token"},
				})
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, ContextKeyRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyUserID).(string)
	return v
}

// RoleFromContext extracts the user role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyRole).(string)
	return v
}
