package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// Login handles POST /auth/login.
func (s *Server) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	email := strings.TrimSpace(strings.ToLower(string(request.Body.Email)))
	if email == "" || request.Body.Password == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Email and password are required")
	}

	result, err := s.authService.Login(ctx, email, request.Body.Password)
	if err != nil {
		return nil, err
	}

	return api.Login200JSONResponse{
		Body: api.LoginResult{
			Data: api.LoginData{
				AccessToken: result.AccessToken,
				User:        domainUserToAPI(result.User),
			},
		},
		Headers: api.Login200ResponseHeaders{
			SetCookie: s.refreshCookieString(result.RefreshTokenRaw, result.RefreshExpiresAt),
		},
	}, nil
}

// RefreshToken handles POST /auth/refresh.
func (s *Server) RefreshToken(ctx context.Context, _ api.RefreshTokenRequestObject) (api.RefreshTokenResponseObject, error) {
	rawCookie := middleware.RefreshCookieFromContext(ctx)
	if rawCookie == "" {
		return nil, domain.ErrUnauthorized
	}

	result, err := s.authService.Refresh(ctx, rawCookie)
	if err != nil {
		return nil, err
	}

	return api.RefreshToken200JSONResponse{
		Body: api.LoginResult{
			Data: api.LoginData{
				AccessToken: result.AccessToken,
				User:        domainUserToAPI(result.User),
			},
		},
		Headers: api.RefreshToken200ResponseHeaders{
			SetCookie: s.refreshCookieString(result.RefreshTokenRaw, result.RefreshExpiresAt),
		},
	}, nil
}

// Logout handles POST /auth/logout. Public endpoint — identity is derived from
// the refresh-token cookie alone, so an expired access token cannot strand the
// user with a live refresh token. The clear-cookie response is unconditional.
func (s *Server) Logout(ctx context.Context, _ api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	raw := middleware.RefreshCookieFromContext(ctx)
	if err := s.authService.LogoutByRefresh(ctx, raw); err != nil {
		s.logger.Error(ctx, "failed to revoke refresh tokens on logout", "error", err)
	}

	return api.Logout200JSONResponse{
		Body: api.MessageResponse{
			Data: api.MessageData{Message: "Logged out"},
		},
		Headers: api.Logout200ResponseHeaders{
			SetCookie: s.clearRefreshCookieString(),
		},
	}, nil
}

// RequestPasswordReset handles POST /auth/password-reset-request.
func (s *Server) RequestPasswordReset(ctx context.Context, request api.RequestPasswordResetRequestObject) (api.RequestPasswordResetResponseObject, error) {
	email := strings.TrimSpace(strings.ToLower(string(request.Body.Email)))
	if email == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Email is required")
	}

	// Always return 200 to prevent email enumeration.
	if err := s.authService.RequestPasswordReset(ctx, email); err != nil {
		s.logger.Error(ctx, "password reset request failed", "error", err)
	}

	return api.RequestPasswordReset200JSONResponse{
		Data: api.MessageData{Message: "If the email exists, a reset link has been sent"},
	}, nil
}

// ResetPassword handles POST /auth/password-reset.
func (s *Server) ResetPassword(ctx context.Context, request api.ResetPasswordRequestObject) (api.ResetPasswordResponseObject, error) {
	if request.Body.Token == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Token is required")
	}
	if len(request.Body.NewPassword) < minPasswordLength {
		return nil, domain.NewValidationError(domain.CodeValidation, "Password must be at least 6 characters")
	}

	if _, err := s.authService.ResetPassword(ctx, request.Body.Token, request.Body.NewPassword); err != nil {
		return nil, err
	}

	return api.ResetPassword200JSONResponse{
		Data: api.MessageData{Message: "Password updated successfully"},
	}, nil
}

// GetMe handles GET /auth/me.
func (s *Server) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	userID := middleware.UserIDFromContext(ctx)
	if userID == "" {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.authService.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return api.GetMe200JSONResponse{
		Data: domainUserToAPI(*user),
	}, nil
}

// refreshCookieString builds the Set-Cookie header value for a freshly minted
// refresh token. We render the cookie via http.Cookie.String() instead of
// http.SetCookie so the strict-server response variant carries the value
// through Headers.SetCookie verbatim.
func (s *Server) refreshCookieString(token string, expiresUnix int64) string {
	c := http.Cookie{
		Name:     CookieRefreshToken,
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expiresUnix, 0),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
	return c.String()
}

// clearRefreshCookieString builds the Set-Cookie header value that clears the
// refresh token on logout (MaxAge=-1, empty value).
func (s *Server) clearRefreshCookieString() string {
	c := http.Cookie{
		Name:     CookieRefreshToken,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
	return c.String()
}

func domainUserToAPI(u domain.User) api.User {
	return api.User{
		Id:    u.ID,
		Email: openapi_types.Email(u.Email),
		Role:  api.UserRole(u.Role),
	}
}
