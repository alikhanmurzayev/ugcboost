package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

var _ = mock.Anything // keep testify/mock import even when no direct use below

// testBcryptCost keeps hashing fast in unit tests.
const testBcryptCost = bcrypt.MinCost

var errNotFound = errors.New("not found")

func hashedPassword(plain string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	return string(h)
}

func testUser() repository.UserRow {
	return repository.UserRow{
		ID:           "user-1",
		Email:        "test@example.com",
		PasswordHash: hashedPassword("password123"),
		Role:         string(api.Admin),
	}
}

var futureTime = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

// stubResetNotifier captures calls to OnResetToken for assertions.
type stubResetNotifier struct {
	calls []struct{ email, token string }
}

func (s *stubResetNotifier) OnResetToken(email, rawToken string) {
	s.calls = append(s.calls, struct{ email, token string }{email, rawToken})
}

// expectAudit asserts the full AuditLogRow passed to AuditRepo.Create.
// JSON fields (OldValue/NewValue) use JSONEq via the expectedOld/expectedNew
// string args — pass empty string when the field must be nil.
func expectAudit(t *testing.T, auditRepo *repomocks.MockAuditRepo, want repository.AuditLogRow, expectedOld, expectedNew string) {
	t.Helper()
	auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Run(func(_ context.Context, row repository.AuditLogRow) {
		if expectedOld == "" {
			require.Nil(t, row.OldValue, "OldValue must be nil when none expected")
		} else {
			require.JSONEq(t, expectedOld, string(row.OldValue))
			row.OldValue = nil
		}
		if expectedNew == "" {
			require.Nil(t, row.NewValue, "NewValue must be nil when none expected")
		} else {
			require.JSONEq(t, expectedNew, string(row.NewValue))
			row.NewValue = nil
		}
		require.Equal(t, want, row)
	}).Return(nil).Once()
}

func TestAuthService_Login(t *testing.T) {
	t.Parallel()

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), "nobody@example.com", "password")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), user.Email, "wrongpass")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("token generation error", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("", errors.New("signing failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), user.Email, "password123")
		require.ErrorContains(t, err, "generate access token")
	})

	t.Run("refresh token generation error", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("", "", time.Time{}, errors.New("rand failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), user.Email, "password123")
		require.ErrorContains(t, err, "generate refresh token")
	})

	t.Run("save refresh token error aborts tx", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("raw", "hash", futureTime, nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash", futureTime).
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), user.Email, "password123")
		require.ErrorContains(t, err, "save refresh token")
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("raw", "hash", futureTime, nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash", futureTime).Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Login(context.Background(), user.Email, "password123")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		entityID := user.ID

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("mock-access-token", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("raw-refresh", "hash-refresh", futureTime, nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash-refresh", futureTime).Return(nil)

		expectAudit(t, audit, repository.AuditLogRow{
			ActorID:    &user.ID,
			ActorRole:  user.Role,
			Action:     AuditActionLogin,
			EntityType: AuditEntityTypeUser,
			EntityID:   &entityID,
		}, "", "")

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		result, err := svc.Login(context.Background(), user.Email, "password123")

		require.NoError(t, err)
		require.Equal(t, &LoginResult{
			AccessToken:      "mock-access-token",
			RefreshTokenRaw:  "raw-refresh",
			RefreshExpiresAt: futureTime.Unix(),
			User: domain.User{
				ID:        user.ID,
				Email:     user.Email,
				Role:      api.UserRole(user.Role),
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			},
		}, result)
	})
}

func TestAuthService_Refresh(t *testing.T) {
	t.Parallel()

	// Refresh wraps claim + save in WithTx to keep refresh-token rotation atomic —
	// every scenario therefore expects pool.Begin before any repo call.

	t.Run("invalid token rolls back tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("invalid-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return((*repository.RefreshTokenRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "invalid-token")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("user not found after claim rolls back tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: "user-1"}, nil)
		repo.EXPECT().GetByID(mock.Anything, "user-1").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "token")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("access token generation error rolls back tx", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		tokenHash := HashToken("token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("", errors.New("signing failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "token")
		require.ErrorContains(t, err, "generate access token")
	})

	t.Run("refresh token generation error rolls back tx", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		tokenHash := HashToken("token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("", "", time.Time{}, errors.New("rand failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "token")
		require.ErrorContains(t, err, "generate refresh token")
	})

	t.Run("save refresh token error rolls back tx", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		tokenHash := HashToken("token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("raw", "hash", futureTime, nil)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash", futureTime).
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "token")
		require.ErrorContains(t, err, "save refresh token")
	})

	t.Run("begin tx error propagates", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(nil, errors.New("begin failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.Refresh(context.Background(), "token")
		require.ErrorContains(t, err, "begin failed")
	})

	t.Run("success commits claim + save atomically", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		tokenHash := HashToken("some-raw-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("new-access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("new-raw", "new-hash", futureTime, nil)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "new-hash", futureTime).Return(nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		result, err := svc.Refresh(context.Background(), "some-raw-token")

		require.NoError(t, err)
		require.Equal(t, &RefreshResult{
			AccessToken:      "new-access",
			RefreshTokenRaw:  "new-raw",
			RefreshExpiresAt: futureTime.Unix(),
			User: domain.User{
				ID:        user.ID,
				Email:     user.Email,
				Role:      api.UserRole(user.Role),
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			},
		}, result)
	})
}

func TestAuthService_Logout(t *testing.T) {
	t.Parallel()

	t.Run("delete refresh tokens error aborts tx", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.Logout(context.Background(), "user-1")
		require.ErrorContains(t, err, "db error")
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.Logout(context.Background(), "user-1")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit row with logged-out user as actor", func(t *testing.T) {
		t.Parallel()
		entityID := "user-1"

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)

		actorID := "user-1"
		expectAudit(t, audit, repository.AuditLogRow{
			ActorID:    &actorID,
			ActorRole:  "",
			Action:     AuditActionLogout,
			EntityType: AuditEntityTypeUser,
			EntityID:   &entityID,
		}, "", "")

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.Logout(context.Background(), "user-1")
		require.NoError(t, err)
	})
}

func TestAuthService_RequestPasswordReset(t *testing.T) {
	t.Parallel()

	t.Run("user not found returns nil without enumeration", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.RequestPasswordReset(context.Background(), "nobody@example.com")
		require.NoError(t, err)
	})

	t.Run("generate reset token error", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateResetToken().Return("", "", time.Time{}, errors.New("rand failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.RequestPasswordReset(context.Background(), user.Email)
		require.ErrorContains(t, err, "generate reset token")
	})

	t.Run("save reset token error", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateResetToken().Return("raw", "hash", futureTime, nil)
		repo.EXPECT().SaveResetToken(mock.Anything, user.ID, "hash", futureTime).
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.RequestPasswordReset(context.Background(), user.Email)
		require.ErrorContains(t, err, "save reset token")
	})

	t.Run("success without notifier", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		log := logmocks.NewMockLogger(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateResetToken().Return("raw-token", "hash-token", futureTime, nil)
		repo.EXPECT().SaveResetToken(mock.Anything, user.ID, "hash-token", futureTime).Return(nil)

		log.EXPECT().Info(mock.Anything, "password reset token generated", mock.Anything).Run(func(_ context.Context, _ string, args ...any) {
			require.Len(t, args, 4)
			require.Equal(t, "user_id", args[0])
			require.Equal(t, user.ID, args[1])
			require.Equal(t, "expires_at", args[2])
			require.NotContains(t, fmt.Sprint(args...), "raw-token")
		}).Once()

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, log)
		err := svc.RequestPasswordReset(context.Background(), user.Email)
		require.NoError(t, err)
	})

	t.Run("success notifier is invoked with raw token", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		notifier := &stubResetNotifier{}
		log := logmocks.NewMockLogger(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
		tokens.EXPECT().GenerateResetToken().Return("raw-token", "hash-token", futureTime, nil)
		repo.EXPECT().SaveResetToken(mock.Anything, user.ID, "hash-token", futureTime).Return(nil)
		log.EXPECT().Info(mock.Anything, "password reset token generated", mock.Anything).Run(func(_ context.Context, _ string, args ...any) {
			require.NotContains(t, fmt.Sprint(args...), "raw-token")
		}).Once()

		svc := NewAuthService(pool, factory, tokens, notifier, testBcryptCost, log)
		err := svc.RequestPasswordReset(context.Background(), user.Email)
		require.NoError(t, err)
		require.Len(t, notifier.calls, 1)
		require.Equal(t, user.Email, notifier.calls[0].email)
		require.Equal(t, "raw-token", notifier.calls[0].token)
	})
}

func TestAuthService_ResetPassword(t *testing.T) {
	t.Parallel()

	t.Run("hash password error aborts before tx", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		// bcrypt rejects passwords > 72 bytes.
		longPassword := strings.Repeat("a", 73)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		userID, err := svc.ResetPassword(context.Background(), "raw", longPassword)
		require.ErrorContains(t, err, "hash password")
		require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
		require.Empty(t, userID)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("bad-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
			Return((*repository.PasswordResetTokenRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		userID, err := svc.ResetPassword(context.Background(), "bad-token", "newpass123")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
		require.Empty(t, userID)
	})

	t.Run("update password error aborts tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("raw-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
			Return(&repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil)
		repo.EXPECT().UpdatePassword(mock.Anything, "user-1", mock.AnythingOfType("string")).
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.ResetPassword(context.Background(), "raw-token", "newpass123")
		require.ErrorContains(t, err, "update password")
	})

	t.Run("delete refresh tokens error aborts tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("raw-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
			Return(&repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil)
		repo.EXPECT().UpdatePassword(mock.Anything, "user-1", mock.AnythingOfType("string")).Return(nil)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").
			Return(errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.ResetPassword(context.Background(), "raw-token", "newpass123")
		require.ErrorContains(t, err, "db error")
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("raw-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
			Return(&repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil)
		repo.EXPECT().UpdatePassword(mock.Anything, "user-1", mock.AnythingOfType("string")).Return(nil)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.ResetPassword(context.Background(), "raw-token", "newpass123")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with reset user as actor", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("raw-reset-token")
		entityID := "user-1"

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
			Return(&repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil)
		repo.EXPECT().UpdatePassword(mock.Anything, "user-1", mock.AnythingOfType("string")).Return(nil)
		repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)

		actorID := "user-1"
		expectAudit(t, audit, repository.AuditLogRow{
			ActorID:    &actorID,
			ActorRole:  "",
			Action:     AuditActionPasswordReset,
			EntityType: AuditEntityTypeUser,
			EntityID:   &entityID,
		}, "", "")

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		userID, err := svc.ResetPassword(context.Background(), "raw-reset-token", "newpass123")
		require.NoError(t, err)
		require.Equal(t, "user-1", userID)
	})
}

func TestAuthService_GetUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		got, err := svc.GetUser(context.Background(), user.ID)
		require.NoError(t, err)
		require.Equal(t, &domain.User{
			ID:        user.ID,
			Email:     user.Email,
			Role:      api.UserRole(user.Role),
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		}, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.GetUser(context.Background(), "missing")
		require.ErrorIs(t, err, errNotFound)
	})
}

func TestAuthService_GetUserByEmail(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		got, err := svc.GetUserByEmail(context.Background(), user.Email)
		require.NoError(t, err)
		require.Equal(t, &domain.User{
			ID:        user.ID,
			Email:     user.Email,
			Role:      api.UserRole(user.Role),
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		}, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, "missing@example.com").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.GetUserByEmail(context.Background(), "missing@example.com")
		require.ErrorIs(t, err, errNotFound)
	})
}

func TestAuthService_SeedUser(t *testing.T) {
	t.Parallel()

	t.Run("hash error propagates", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		// Production calls NewUserRepo before hashing — mock it but no repo calls follow.
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)

		longPassword := strings.Repeat("a", 73)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.SeedUser(context.Background(), "seed@example.com", longPassword, "admin")
		require.ErrorContains(t, err, "hash password")
		require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().Create(mock.Anything, "seed@example.com", mock.AnythingOfType("string"), "admin").
			Return((*repository.UserRow)(nil), errors.New("unique violation"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		_, err := svc.SeedUser(context.Background(), "seed@example.com", "pass", "admin")
		require.ErrorContains(t, err, "unique violation")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		created := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().Create(mock.Anything, "seed@example.com", mock.AnythingOfType("string"), "admin").
			Return(&repository.UserRow{
				ID: "u-seed", Email: "seed@example.com", Role: "admin",
				CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		got, err := svc.SeedUser(context.Background(), "seed@example.com", "pass", "admin")
		require.NoError(t, err)
		require.Equal(t, &domain.User{
			ID:        "u-seed",
			Email:     "seed@example.com",
			Role:      api.Admin,
			CreatedAt: created,
			UpdatedAt: created,
		}, got)
	})
}

func TestAuthService_SeedAdmin(t *testing.T) {
	t.Parallel()

	t.Run("skips when email empty", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Info(mock.Anything, "admin seed skipped: ADMIN_EMAIL or ADMIN_PASSWORD not set").Once()

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, log)
		err := svc.SeedAdmin(context.Background(), "", "pass")
		require.NoError(t, err)
	})

	t.Run("skips when password empty", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Info(mock.Anything, "admin seed skipped: ADMIN_EMAIL or ADMIN_PASSWORD not set").Once()

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, log)
		err := svc.SeedAdmin(context.Background(), "admin@example.com", "")
		require.NoError(t, err)
	})

	t.Run("exists check error", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").
			Return(false, errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")
		require.ErrorContains(t, err, "check admin exists")
	})

	t.Run("skips when admin exists", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		log := logmocks.NewMockLogger(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(true, nil)
		log.EXPECT().Info(mock.Anything, "admin already exists", []any{"email", "admin@test.com"}).Once()

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, log)
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")
		require.NoError(t, err)
	})

	t.Run("hash error", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(false, nil)

		longPassword := strings.Repeat("x", 73)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.SeedAdmin(context.Background(), "admin@test.com", longPassword)
		require.ErrorContains(t, err, "hash admin password")
		require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
	})

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(false, nil)
		repo.EXPECT().Create(mock.Anything, "admin@test.com", mock.AnythingOfType("string"), string(api.Admin)).
			Return((*repository.UserRow)(nil), errors.New("db error"))

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, logmocks.NewMockLogger(t))
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")
		require.ErrorContains(t, err, "create admin")
	})

	t.Run("success creates admin", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)
		log := logmocks.NewMockLogger(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(false, nil)
		repo.EXPECT().Create(mock.Anything, "admin@test.com", mock.AnythingOfType("string"), string(api.Admin)).
			Return(&repository.UserRow{ID: "new-admin"}, nil)
		log.EXPECT().Info(mock.Anything, "admin user created", []any{"email", "admin@test.com"}).Once()

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost, log)
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")
		require.NoError(t, err)
	})
}
