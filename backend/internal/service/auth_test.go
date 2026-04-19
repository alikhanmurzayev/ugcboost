package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

// testBcryptCost is used in all service tests to keep hashing fast.
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
		Role:         "admin",
	}
}

var futureTime = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

func TestAuthService_Login(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := testUser()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		// Initial read outside tx, then tx for save + audit.
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash-refresh", futureTime).Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionLogin && row.ActorID == user.ID
		})).Return(nil)

		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("mock-access-token", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("raw-refresh", "hash-refresh", futureTime, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		result, err := svc.Login(context.Background(), user.Email, "password123")

		require.NoError(t, err)
		require.Equal(t, "mock-access-token", result.AccessToken)
		require.Equal(t, "raw-refresh", result.RefreshTokenRaw)
		require.Equal(t, futureTime.Unix(), result.RefreshExpiresAt)
		require.Equal(t, user.ID, result.User.ID)
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

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		_, err := svc.Login(context.Background(), user.Email, "wrongpass")

		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})

	t.Run("user not found", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return((*repository.UserRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		_, err := svc.Login(context.Background(), "nobody@example.com", "password")

		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})
}

func TestAuthService_Refresh(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		user := testUser()
		tokenHash := HashToken("some-raw-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
		repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
		repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "new-hash", futureTime).Return(nil)

		tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("new-access", nil)
		tokens.EXPECT().GenerateRefreshToken().Return("new-raw", "new-hash", futureTime, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		result, err := svc.Refresh(context.Background(), "some-raw-token")

		require.NoError(t, err)
		require.Equal(t, "new-access", result.AccessToken)
		require.Equal(t, "new-raw", result.RefreshTokenRaw)
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("invalid-token")

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
			Return((*repository.RefreshTokenRow)(nil), errNotFound)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		_, err := svc.Refresh(context.Background(), "invalid-token")

		require.ErrorIs(t, err, domain.ErrUnauthorized)
	})
}

func TestAuthService_Logout(t *testing.T) {
	t.Parallel()

	t.Run("success writes audit in tx", func(t *testing.T) {
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
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionLogout && row.ActorID == "user-1"
		})).Return(nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		err := svc.Logout(context.Background(), "user-1")

		require.NoError(t, err)
	})
}

func TestAuthService_ResetPassword(t *testing.T) {
	t.Parallel()

	t.Run("success writes audit in same tx", func(t *testing.T) {
		t.Parallel()
		tokenHash := HashToken("raw-reset-token")

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
		audit.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
			return row.Action == AuditActionPasswordReset && row.ActorID == "user-1"
		})).Return(nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		userID, err := svc.ResetPassword(context.Background(), "raw-reset-token", "newpass123")

		require.NoError(t, err)
		require.Equal(t, "user-1", userID)
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

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		userID, err := svc.ResetPassword(context.Background(), "bad-token", "newpass123")

		require.ErrorIs(t, err, domain.ErrUnauthorized)
		require.Empty(t, userID)
	})
}

func TestAuthService_SeedAdmin(t *testing.T) {
	t.Parallel()

	t.Run("creates when missing", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(false, nil)
		repo.EXPECT().Create(mock.Anything, "admin@test.com", mock.AnythingOfType("string"), "admin").
			Return(&repository.UserRow{ID: "new-admin"}, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")

		require.NoError(t, err)
	})

	t.Run("skips when exists", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		repo := repomocks.NewMockUserRepo(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		factory.EXPECT().NewUserRepo(mock.Anything).Return(repo)
		repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(true, nil)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")

		require.NoError(t, err)
	})

	t.Run("skips when empty", func(t *testing.T) {
		t.Parallel()

		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockAuthRepoFactory(t)
		tokens := svcmocks.NewMockTokenGenerator(t)

		svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
		err := svc.SeedAdmin(context.Background(), "", "")

		require.NoError(t, err)
	})
}
