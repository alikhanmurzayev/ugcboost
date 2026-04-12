package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// Login handles POST /auth/login
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Email and password are required"))
		return
	}

	result, err := s.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		respondError(w, r, err)
		return
	}

	s.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: result.User.ID, ActorRole: string(result.User.Role),
		Action: "login", EntityType: "user", EntityID: result.User.ID,
		IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"accessToken": result.AccessToken,
		"user": map[string]any{
			"id":    result.User.ID,
			"email": result.User.Email,
			"role":  result.User.Role,
		},
	})
}

// RefreshToken handles POST /auth/refresh
func (s *Server) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieRefreshToken)
	if err != nil || cookie.Value == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	result, err := s.authService.Refresh(r.Context(), cookie.Value)
	if err != nil {
		respondError(w, r, err)
		return
	}

	s.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

	respondJSON(w, http.StatusOK, map[string]any{
		"accessToken": result.AccessToken,
		"user": map[string]any{
			"id":    result.User.ID,
			"email": result.User.Email,
			"role":  result.User.Role,
		},
	})
}

// Logout handles POST /auth/logout
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	if err := s.authService.Logout(r.Context(), userID); err != nil {
		slog.Error("failed to revoke refresh tokens on logout", "error", err, "userID", userID)
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: userID, ActorRole: middleware.RoleFromContext(r.Context()),
		Action: "logout", EntityType: "user", EntityID: userID,
		IPAddress: clientIP(r),
	})

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

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Logged out",
	})
}

// RequestPasswordReset handles POST /auth/password-reset-request
func (s *Server) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Email is required"))
		return
	}

	// Always return 200 to prevent email enumeration
	if err := s.authService.RequestPasswordReset(r.Context(), req.Email); err != nil {
		slog.Error("password reset request failed", "error", err)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "If the email exists, a reset link has been sent",
	})
}

// ResetPassword handles POST /auth/password-reset
func (s *Server) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"))
		return
	}

	if req.Token == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Token is required"))
		return
	}
	if len(req.NewPassword) < 6 {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Password must be at least 6 characters"))
		return
	}

	resetUserID, err := s.authService.ResetPassword(r.Context(), req.Token, req.NewPassword)
	if err != nil {
		respondError(w, r, err)
		return
	}

	logAudit(r.Context(), s.auditService, service.AuditEntry{
		ActorID: resetUserID, EntityType: "user", EntityID: resetUserID,
		Action: "password_reset", IPAddress: clientIP(r),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Password updated successfully",
	})
}

// GetMe handles GET /auth/me
func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	user, err := s.authService.GetUser(r.Context(), userID)
	if err != nil {
		respondError(w, r, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
	})
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
