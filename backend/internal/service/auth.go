package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

const bcryptCost = 12

// UserRepo is the interface AuthService needs from the user repository.
type UserRepo interface {
	GetByEmail(ctx context.Context, email string) (repository.UserRow, error)
	GetByID(ctx context.Context, id string) (repository.UserRow, error)
	Create(ctx context.Context, email, passwordHash, role string) (repository.UserRow, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ClaimRefreshToken(ctx context.Context, tokenHash string) (repository.RefreshTokenRow, error)
	DeleteUserRefreshTokens(ctx context.Context, userID string) error
	SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ClaimResetToken(ctx context.Context, tokenHash string) (repository.PasswordResetTokenRow, error)
}

// TokenGenerator is the interface AuthService needs from the token service.
type TokenGenerator interface {
	GenerateAccessToken(userID, role string) (string, error)
	GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
	GenerateResetToken() (raw string, hash string, expiresAt time.Time, err error)
}

// AuthService handles authentication business logic.
type AuthService struct {
	users  UserRepo
	tokens TokenGenerator
}

// NewAuthService creates a new AuthService.
func NewAuthService(users UserRepo, tokens TokenGenerator) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

// LoginResult contains the result of a successful login.
type LoginResult struct {
	AccessToken      string
	RefreshTokenRaw  string
	RefreshExpiresAt int64
	User             repository.UserRow
}

// Login authenticates a user by email and password.
func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrUnauthorized
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshRaw, refreshHash, refreshExpires, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.users.SaveRefreshToken(ctx, user.ID, refreshHash, refreshExpires); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &LoginResult{
		AccessToken:      accessToken,
		RefreshTokenRaw:  refreshRaw,
		RefreshExpiresAt: refreshExpires.Unix(),
		User:             user,
	}, nil
}

// RefreshResult contains the result of a token refresh.
type RefreshResult struct {
	AccessToken      string
	RefreshTokenRaw  string
	RefreshExpiresAt int64
	User             repository.UserRow
}

// Refresh validates a refresh token, rotates it, and returns new tokens.
func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResult, error) {
	hash := HashToken(rawRefreshToken)

	// Atomic claim: DELETE...RETURNING prevents race condition on reuse
	rt, err := s.users.ClaimRefreshToken(ctx, hash)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	accessToken, err := s.tokens.GenerateAccessToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	newRaw, newHash, newExpires, err := s.tokens.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.users.SaveRefreshToken(ctx, user.ID, newHash, newExpires); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &RefreshResult{
		AccessToken:      accessToken,
		RefreshTokenRaw:  newRaw,
		RefreshExpiresAt: newExpires.Unix(),
		User:             user,
	}, nil
}

// Logout invalidates all refresh tokens for the user.
func (s *AuthService) Logout(ctx context.Context, userID string) error {
	return s.users.DeleteUserRefreshTokens(ctx, userID)
}

// RequestPasswordReset generates a reset token. Always returns nil to prevent email enumeration.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil // don't reveal if email exists
	}

	_, hash, expiresAt, err := s.tokens.GenerateResetToken()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	if err := s.users.SaveResetToken(ctx, user.ID, hash, expiresAt); err != nil {
		return fmt.Errorf("save reset token: %w", err)
	}

	// MVP: log to stdout. Email sending in Epic 3.
	// Never log raw token — only user_id and expiry.
	slog.Info("password reset token generated",
		"user_id", user.ID,
		"expires_at", expiresAt,
	)

	return nil
}

// ResetPassword validates a reset token and updates the password.
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	hash := HashToken(rawToken)

	// Atomic claim: UPDATE SET used=true...RETURNING prevents TOCTOU race
	rt, err := s.users.ClaimResetToken(ctx, hash)
	if err != nil {
		return domain.ErrUnauthorized
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.users.UpdatePassword(ctx, rt.UserID, string(passwordHash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Invalidate all refresh tokens on password change
	if err := s.users.DeleteUserRefreshTokens(ctx, rt.UserID); err != nil {
		return fmt.Errorf("delete refresh tokens: %w", err)
	}

	return nil
}

// GetUser returns a user by ID.
func (s *AuthService) GetUser(ctx context.Context, userID string) (repository.UserRow, error) {
	return s.users.GetByID(ctx, userID)
}

// SeedAdmin creates the admin user if it doesn't exist.
func (s *AuthService) SeedAdmin(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		slog.Info("admin seed skipped: ADMIN_EMAIL or ADMIN_PASSWORD not set")
		return nil
	}

	exists, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}
	if exists {
		slog.Info("admin already exists", "email", email)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	_, err = s.users.Create(ctx, email, string(hash), "admin")
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	slog.Info("admin user created", "email", email)
	return nil
}
