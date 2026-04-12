package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
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

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	user := testUser()

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)
	repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "hash-refresh", futureTime).Return(nil)

	tokens := mocks.NewMockTokenGenerator(t)
	tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("mock-access-token", nil)
	tokens.EXPECT().GenerateRefreshToken().Return("raw-refresh", "hash-refresh", futureTime, nil)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	result, err := svc.Login(context.Background(), user.Email, "password123")

	require.NoError(t, err)
	assert.Equal(t, "mock-access-token", result.AccessToken)
	assert.Equal(t, "raw-refresh", result.RefreshTokenRaw)
	assert.Equal(t, futureTime.Unix(), result.RefreshExpiresAt)
	assert.Equal(t, user.ID, result.User.ID)
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	user := testUser()

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().GetByEmail(mock.Anything, user.Email).Return(&user, nil)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	_, err := svc.Login(context.Background(), user.Email, "wrongpass")

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
		Return((*repository.UserRow)(nil), errNotFound)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	_, err := svc.Login(context.Background(), "nobody@example.com", "password")

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

// --- Refresh tests ---

func TestRefresh_Success(t *testing.T) {
	t.Parallel()
	user := testUser()
	tokenHash := HashToken("some-raw-token")

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
		Return(&repository.RefreshTokenRow{UserID: user.ID}, nil)
	repo.EXPECT().GetByID(mock.Anything, user.ID).Return(&user, nil)
	repo.EXPECT().SaveRefreshToken(mock.Anything, user.ID, "new-hash", futureTime).Return(nil)

	tokens := mocks.NewMockTokenGenerator(t)
	tokens.EXPECT().GenerateAccessToken(user.ID, user.Role).Return("new-access", nil)
	tokens.EXPECT().GenerateRefreshToken().Return("new-raw", "new-hash", futureTime, nil)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	result, err := svc.Refresh(context.Background(), "some-raw-token")

	require.NoError(t, err)
	assert.Equal(t, "new-access", result.AccessToken)
	assert.Equal(t, "new-raw", result.RefreshTokenRaw)
}

func TestRefresh_InvalidToken(t *testing.T) {
	t.Parallel()
	tokenHash := HashToken("invalid-token")

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ClaimRefreshToken(mock.Anything, tokenHash).
		Return((*repository.RefreshTokenRow)(nil), errNotFound)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	_, err := svc.Refresh(context.Background(), "invalid-token")

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

// --- Logout tests ---

func TestLogout_Success(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	err := svc.Logout(context.Background(), "user-1")

	assert.NoError(t, err)
}

// --- ResetPassword tests ---

func TestResetPassword_Success(t *testing.T) {
	t.Parallel()
	tokenHash := HashToken("raw-reset-token")

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
		Return(&repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil)
	repo.EXPECT().UpdatePassword(mock.Anything, "user-1", mock.AnythingOfType("string")).Return(nil)
	repo.EXPECT().DeleteUserRefreshTokens(mock.Anything, "user-1").Return(nil)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	userID, err := svc.ResetPassword(context.Background(), "raw-reset-token", "newpass123")

	assert.NoError(t, err)
	assert.Equal(t, "user-1", userID)
}

func TestResetPassword_InvalidToken(t *testing.T) {
	t.Parallel()
	tokenHash := HashToken("bad-token")

	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ClaimResetToken(mock.Anything, tokenHash).
		Return((*repository.PasswordResetTokenRow)(nil), errNotFound)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	userID, err := svc.ResetPassword(context.Background(), "bad-token", "newpass123")

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
	assert.Empty(t, userID)
}

// --- SeedAdmin tests ---

func TestSeedAdmin_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(false, nil)
	repo.EXPECT().Create(mock.Anything, "admin@test.com", mock.AnythingOfType("string"), "admin").
		Return(&repository.UserRow{ID: "new-admin"}, nil)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")

	assert.NoError(t, err)
}

func TestSeedAdmin_SkipsWhenExists(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockUserRepo(t)
	repo.EXPECT().ExistsByEmail(mock.Anything, "admin@test.com").Return(true, nil)

	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	err := svc.SeedAdmin(context.Background(), "admin@test.com", "secret")

	assert.NoError(t, err)
}

func TestSeedAdmin_SkipsWhenEmpty(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockUserRepo(t)
	tokens := mocks.NewMockTokenGenerator(t)

	svc := NewAuthService(repo, tokens, nil, testBcryptCost)
	err := svc.SeedAdmin(context.Background(), "", "")

	assert.NoError(t, err)
}
