package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// AuthRepoFactory creates repositories needed by AuthService.
type AuthRepoFactory interface {
	NewUserRepo(db dbutil.DB) repository.UserRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// TokenGenerator is the interface AuthService needs from the token service.
type TokenGenerator interface {
	GenerateAccessToken(userID, role string) (string, error)
	GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error)
	GenerateResetToken() (raw string, hash string, expiresAt time.Time, err error)
}

// AuthService handles authentication business logic.
type AuthService struct {
	pool          dbutil.Pool
	repoFactory   AuthRepoFactory
	tokens        TokenGenerator
	resetNotifier ResetTokenNotifier
	bcryptCost    int
	logger        logger.Logger
}

// NewAuthService creates a new AuthService. resetNotifier may be nil.
func NewAuthService(pool dbutil.Pool, repoFactory AuthRepoFactory, tokens TokenGenerator, resetNotifier ResetTokenNotifier, bcryptCost int, log logger.Logger) *AuthService {
	return &AuthService{pool: pool, repoFactory: repoFactory, tokens: tokens, resetNotifier: resetNotifier, bcryptCost: bcryptCost, logger: log}
}

// LoginResult contains the result of a successful login.
type LoginResult struct {
	AccessToken      string
	RefreshTokenRaw  string
	RefreshExpiresAt int64
	User             domain.User
}

// Login authenticates a user by email and password. On success it persists a
// refresh token and writes a login audit entry inside the same transaction.
func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	readRepo := s.repoFactory.NewUserRepo(s.pool)

	user, err := readRepo.GetByEmail(ctx, email)
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

	// Write refresh token + audit entry atomically. Login has no authenticated
	// context user, so actor info for the audit row is derived from the user
	// that just authenticated.
	loginCtx := contextWithActor(ctx, user.ID, user.Role)

	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		userRepo := s.repoFactory.NewUserRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		if err := userRepo.SaveRefreshToken(ctx, user.ID, refreshHash, refreshExpires); err != nil {
			return fmt.Errorf("save refresh token: %w", err)
		}

		return writeAudit(loginCtx, auditRepo,
			AuditActionLogin, AuditEntityTypeUser, user.ID, nil, nil)
	})
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:      accessToken,
		RefreshTokenRaw:  refreshRaw,
		RefreshExpiresAt: refreshExpires.Unix(),
		User:             userRowToDomain(user),
	}, nil
}

// RefreshResult contains the result of a token refresh.
type RefreshResult struct {
	AccessToken      string
	RefreshTokenRaw  string
	RefreshExpiresAt int64
	User             domain.User
}

// Refresh validates a refresh token, rotates it, and returns new tokens.
// Claim + save happen in a single transaction so a save failure cannot leave
// the user with their old refresh token deleted and no new one.
func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken string) (*RefreshResult, error) {
	hash := HashToken(rawRefreshToken)

	var result *RefreshResult
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		userRepo := s.repoFactory.NewUserRepo(tx)

		rt, err := userRepo.ClaimRefreshToken(ctx, hash)
		if err != nil {
			return domain.ErrUnauthorized
		}

		user, err := userRepo.GetByID(ctx, rt.UserID)
		if err != nil {
			return domain.ErrUnauthorized
		}

		accessToken, err := s.tokens.GenerateAccessToken(user.ID, user.Role)
		if err != nil {
			return fmt.Errorf("generate access token: %w", err)
		}

		newRaw, newHash, newExpires, err := s.tokens.GenerateRefreshToken()
		if err != nil {
			return fmt.Errorf("generate refresh token: %w", err)
		}

		if err := userRepo.SaveRefreshToken(ctx, user.ID, newHash, newExpires); err != nil {
			return fmt.Errorf("save refresh token: %w", err)
		}

		result = &RefreshResult{
			AccessToken:      accessToken,
			RefreshTokenRaw:  newRaw,
			RefreshExpiresAt: newExpires.Unix(),
			User:             userRowToDomain(user),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LogoutByRefresh resolves the user from the supplied refresh-token cookie,
// revokes every refresh token they own, and records the audit entry — all in
// one transaction. An empty or unknown cookie is a no-op so anonymous
// /auth/logout calls (cookie already cleared, expired token, etc.) still
// succeed and let the handler emit the clear-cookie response.
func (s *AuthService) LogoutByRefresh(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	hash := HashToken(rawRefreshToken)
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		userRepo := s.repoFactory.NewUserRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		rt, err := userRepo.ClaimRefreshToken(ctx, hash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("claim refresh token: %w", err)
		}

		if err := userRepo.DeleteUserRefreshTokens(ctx, rt.UserID); err != nil {
			return err
		}

		return writeAudit(contextWithActor(ctx, rt.UserID, ""), auditRepo,
			AuditActionLogout, AuditEntityTypeUser, rt.UserID, nil, nil)
	})
}

// RequestPasswordReset generates a reset token. Always returns nil to prevent email enumeration.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	userRepo := s.repoFactory.NewUserRepo(s.pool)

	user, err := userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil //nolint:nilerr // intentional: don't reveal if email exists (prevent enumeration)
	}

	raw, hash, expiresAt, err := s.tokens.GenerateResetToken()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	if err := userRepo.SaveResetToken(ctx, user.ID, hash, expiresAt); err != nil {
		return fmt.Errorf("save reset token: %w", err)
	}

	if s.resetNotifier != nil {
		s.resetNotifier.OnResetToken(email, raw)
	}

	s.logger.Info(ctx, "password reset token generated",
		"user_id", user.ID,
		"expires_at", expiresAt,
	)

	return nil
}

// ResetPassword validates a reset token and updates the password.
// Returns the user ID of the account whose password was reset.
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error) {
	hash := HashToken(rawToken)

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	var userID string
	err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		userRepo := s.repoFactory.NewUserRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		// Atomic claim: UPDATE SET used=true...RETURNING prevents TOCTOU race
		rt, err := userRepo.ClaimResetToken(ctx, hash)
		if err != nil {
			return domain.ErrUnauthorized
		}
		userID = rt.UserID

		if err := userRepo.UpdatePassword(ctx, userID, string(passwordHash)); err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		// Invalidate all refresh tokens on password change
		if err := userRepo.DeleteUserRefreshTokens(ctx, userID); err != nil {
			return err
		}

		// Actor for password reset is the user whose password just changed.
		resetCtx := contextWithActor(ctx, userID, "")
		return writeAudit(resetCtx, auditRepo,
			AuditActionPasswordReset, AuditEntityTypeUser, userID, nil, nil)
	})
	if err != nil {
		return "", err
	}

	return userID, nil
}

// GetUser returns a user by ID.
func (s *AuthService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	userRepo := s.repoFactory.NewUserRepo(s.pool)
	row, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u := userRowToDomain(row)
	return &u, nil
}

// GetUserByEmail returns a user by email.
func (s *AuthService) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	userRepo := s.repoFactory.NewUserRepo(s.pool)
	row, err := userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	u := userRowToDomain(row)
	return &u, nil
}

// SeedUser creates a user with the given role. Used by test endpoints.
func (s *AuthService) SeedUser(ctx context.Context, email, password, role string) (*domain.User, error) {
	userRepo := s.repoFactory.NewUserRepo(s.pool)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	row, err := userRepo.Create(ctx, email, string(hash), role)
	if err != nil {
		return nil, err
	}
	u := userRowToDomain(row)
	return &u, nil
}

// SeedAdmin creates the admin user if it doesn't exist.
func (s *AuthService) SeedAdmin(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		s.logger.Info(ctx, "admin seed skipped: ADMIN_EMAIL or ADMIN_PASSWORD not set")
		return nil
	}

	userRepo := s.repoFactory.NewUserRepo(s.pool)

	exists, err := userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}
	if exists {
		s.logger.Info(ctx, "admin already exists", "email", email)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	_, err = userRepo.Create(ctx, email, string(hash), string(api.Admin))
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	s.logger.Info(ctx, "admin user created", "email", email)
	return nil
}

func userRowToDomain(row *repository.UserRow) domain.User {
	return domain.User{
		ID:        row.ID,
		Email:     row.Email,
		Role:      api.UserRole(row.Role),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
