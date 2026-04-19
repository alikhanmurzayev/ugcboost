package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// Login handles POST /auth/login
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), s.logger)
		return
	}

	email := strings.TrimSpace(strings.ToLower(string(req.Email)))

	if email == "" || req.Password == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Email and password are required"), s.logger)
		return
	}

	result, err := s.authService.Login(r.Context(), email, req.Password)
	if err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	s.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

	respondJSON(w, r, http.StatusOK, api.LoginResult{
		Data: api.LoginData{
			AccessToken: result.AccessToken,
			User:        domainUserToAPI(result.User),
		},
	}, s.logger)
}

// RefreshToken handles POST /auth/refresh
func (s *Server) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieRefreshToken)
	if err != nil || cookie.Value == "" {
		respondError(w, r, domain.ErrUnauthorized, s.logger)
		return
	}

	result, err := s.authService.Refresh(r.Context(), cookie.Value)
	if err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	s.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

	respondJSON(w, r, http.StatusOK, api.LoginResult{
		Data: api.LoginData{
			AccessToken: result.AccessToken,
			User:        domainUserToAPI(result.User),
		},
	}, s.logger)
}

// Logout handles POST /auth/logout
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized, s.logger)
		return
	}

	if err := s.authService.Logout(r.Context(), userID); err != nil {
		s.logger.Error(r.Context(), "failed to revoke refresh tokens on logout", "error", err, "userID", userID)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     CookieRefreshToken,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})

	respondJSON(w, r, http.StatusOK, api.MessageResponse{
		Data: api.MessageData{Message: "Logged out"},
	}, s.logger)
}

// RequestPasswordReset handles POST /auth/password-reset-request
func (s *Server) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req api.PasswordResetRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), s.logger)
		return
	}

	email := strings.TrimSpace(strings.ToLower(string(req.Email)))
	if email == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Email is required"), s.logger)
		return
	}

	// Always return 200 to prevent email enumeration
	if err := s.authService.RequestPasswordReset(r.Context(), email); err != nil {
		s.logger.Error(r.Context(), "password reset request failed", "error", err)
	}

	respondJSON(w, r, http.StatusOK, api.MessageResponse{
		Data: api.MessageData{Message: "If the email exists, a reset link has been sent"},
	}, s.logger)
}

// ResetPassword handles POST /auth/password-reset
func (s *Server) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req api.PasswordResetBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), s.logger)
		return
	}

	if req.Token == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Token is required"), s.logger)
		return
	}
	if len(req.NewPassword) < minPasswordLength {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Password must be at least 6 characters"), s.logger)
		return
	}

	if _, err := s.authService.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	respondJSON(w, r, http.StatusOK, api.MessageResponse{
		Data: api.MessageData{Message: "Password updated successfully"},
	}, s.logger)
}

// GetMe handles GET /auth/me
func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized, s.logger)
		return
	}

	user, err := s.authService.GetUser(r.Context(), userID)
	if err != nil {
		respondError(w, r, err, s.logger)
		return
	}

	respondJSON(w, r, http.StatusOK, api.UserResponse{
		Data: domainUserToAPI(*user),
	}, s.logger)
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, token string, expiresUnix int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieRefreshToken,
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expiresUnix, 0),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func domainUserToAPI(u domain.User) api.User {
	return api.User{
		Id:    u.ID,
		Email: openapi_types.Email(u.Email),
		Role:  api.UserRole(u.Role),
	}
}
