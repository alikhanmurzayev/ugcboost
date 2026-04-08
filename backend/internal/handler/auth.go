package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// Auth is the interface AuthHandler needs from the auth service.
type Auth interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*service.RefreshResult, error)
	Logout(ctx context.Context, userID string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error)
	GetUser(ctx context.Context, userID string) (repository.UserRow, error)
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	auth    Auth
	auditor Auditor
	secure  bool // true = Secure cookie flag (production)
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(auth Auth, secure bool) *AuthHandler {
	return &AuthHandler{auth: auth, secure: secure}
}

// SetAuditor sets the optional audit logger.
func (h *AuthHandler) SetAuditor(a Auditor) { h.auditor = a }

// Login handles POST /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Invalid request body"))
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Email and password are required"))
		return
	}

	result, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		respondError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

	if h.auditor != nil {
		h.auditor.Log(r.Context(), service.AuditEntry{
			ActorID: result.User.ID, ActorRole: result.User.Role,
			Action: "login", EntityType: "user", EntityID: result.User.ID,
			IPAddress: clientIP(r),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"accessToken": result.AccessToken,
		"user": map[string]any{
			"id":    result.User.ID,
			"email": result.User.Email,
			"role":  result.User.Role,
		},
	})
}

// Refresh handles POST /auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	result, err := h.auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		respondError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshTokenRaw, result.RefreshExpiresAt)

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
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	_ = h.auth.Logout(r.Context(), userID)

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteStrictMode,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Logged out",
	})
}

// RequestPasswordReset handles POST /auth/password-reset-request
func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Invalid request body"))
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Email is required"))
		return
	}

	// Always return 200 to prevent email enumeration
	_ = h.auth.RequestPasswordReset(r.Context(), req.Email)

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "If the email exists, a reset link has been sent",
	})
}

// ResetPassword handles POST /auth/password-reset
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Invalid request body"))
		return
	}

	if req.Token == "" {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Token is required"))
		return
	}
	if len(req.NewPassword) < 6 {
		respondError(w, r, domain.NewValidationError("VALIDATION_ERROR", "Password must be at least 6 characters"))
		return
	}

	resetUserID, err := h.auth.ResetPassword(r.Context(), req.Token, req.NewPassword)
	if err != nil {
		respondError(w, r, err)
		return
	}

	if h.auditor != nil {
		h.auditor.Log(r.Context(), service.AuditEntry{
			ActorID: resetUserID, EntityType: "user", EntityID: resetUserID,
			Action: "password_reset", IPAddress: clientIP(r),
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"message": "Password updated successfully",
	})
}

// GetMe handles GET /auth/me
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondError(w, r, domain.ErrUnauthorized)
		return
	}

	user, err := h.auth.GetUser(r.Context(), userID)
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

func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, token string, expiresUnix int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expiresUnix, 0),
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteStrictMode,
	})
}
