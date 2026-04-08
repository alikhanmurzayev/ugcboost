package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// --- mock repo ---

type mockUserRepo struct {
	getByEmail             func(ctx context.Context, email string) (repository.UserRow, error)
	getByID                func(ctx context.Context, id string) (repository.UserRow, error)
	create                 func(ctx context.Context, email, hash, role string) (repository.UserRow, error)
	existsByEmail          func(ctx context.Context, email string) (bool, error)
	updatePassword         func(ctx context.Context, userID, hash string) error
	saveRefreshToken       func(ctx context.Context, userID, hash string, exp time.Time) error
	claimRefreshToken      func(ctx context.Context, hash string) (repository.RefreshTokenRow, error)
	deleteUserRefreshTokens func(ctx context.Context, userID string) error
	saveResetToken         func(ctx context.Context, userID, hash string, exp time.Time) error
	claimResetToken        func(ctx context.Context, hash string) (repository.PasswordResetTokenRow, error)
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (repository.UserRow, error) {
	return m.getByEmail(ctx, email)
}
func (m *mockUserRepo) GetByID(ctx context.Context, id string) (repository.UserRow, error) {
	return m.getByID(ctx, id)
}
func (m *mockUserRepo) Create(ctx context.Context, email, hash, role string) (repository.UserRow, error) {
	return m.create(ctx, email, hash, role)
}
func (m *mockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return m.existsByEmail(ctx, email)
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID, hash string) error {
	return m.updatePassword(ctx, userID, hash)
}
func (m *mockUserRepo) SaveRefreshToken(ctx context.Context, userID, hash string, exp time.Time) error {
	return m.saveRefreshToken(ctx, userID, hash, exp)
}
func (m *mockUserRepo) ClaimRefreshToken(ctx context.Context, hash string) (repository.RefreshTokenRow, error) {
	return m.claimRefreshToken(ctx, hash)
}
func (m *mockUserRepo) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	return m.deleteUserRefreshTokens(ctx, userID)
}
func (m *mockUserRepo) SaveResetToken(ctx context.Context, userID, hash string, exp time.Time) error {
	return m.saveResetToken(ctx, userID, hash, exp)
}
func (m *mockUserRepo) ClaimResetToken(ctx context.Context, hash string) (repository.PasswordResetTokenRow, error) {
	return m.claimResetToken(ctx, hash)
}

// --- helpers ---

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

func newTestService(repo *mockUserRepo) *AuthService {
	tokens := NewTokenService("test-secret", 15*time.Minute)
	return NewAuthService(repo, tokens)
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	user := testUser()
	repo := &mockUserRepo{
		getByEmail: func(_ context.Context, email string) (repository.UserRow, error) {
			if email == user.Email {
				return user, nil
			}
			return repository.UserRow{}, errNotFound
		},
		saveRefreshToken: func(_ context.Context, _ string, _ string, _ time.Time) error {
			return nil
		},
	}

	result, err := newTestService(repo).Login(context.Background(), user.Email, "password123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected access token")
	}
	if result.RefreshTokenRaw == "" {
		t.Error("expected refresh token")
	}
	if result.User.ID != user.ID {
		t.Errorf("expected user ID %q, got %q", user.ID, result.User.ID)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	user := testUser()
	repo := &mockUserRepo{
		getByEmail: func(_ context.Context, _ string) (repository.UserRow, error) {
			return user, nil
		},
	}

	_, err := newTestService(repo).Login(context.Background(), user.Email, "wrongpass")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mockUserRepo{
		getByEmail: func(_ context.Context, _ string) (repository.UserRow, error) {
			return repository.UserRow{}, errNotFound
		},
	}

	_, err := newTestService(repo).Login(context.Background(), "nobody@example.com", "password")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// --- Refresh tests ---

func TestRefresh_Success(t *testing.T) {
	user := testUser()
	repo := &mockUserRepo{
		claimRefreshToken: func(_ context.Context, _ string) (repository.RefreshTokenRow, error) {
			return repository.RefreshTokenRow{UserID: user.ID}, nil
		},
		getByID: func(_ context.Context, id string) (repository.UserRow, error) {
			if id == user.ID {
				return user, nil
			}
			return repository.UserRow{}, errNotFound
		},
		saveRefreshToken: func(_ context.Context, _ string, _ string, _ time.Time) error {
			return nil
		},
	}

	result, err := newTestService(repo).Refresh(context.Background(), "some-raw-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AccessToken == "" {
		t.Error("expected new access token")
	}
	if result.RefreshTokenRaw == "" {
		t.Error("expected new refresh token")
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	repo := &mockUserRepo{
		claimRefreshToken: func(_ context.Context, _ string) (repository.RefreshTokenRow, error) {
			return repository.RefreshTokenRow{}, errNotFound
		},
	}

	_, err := newTestService(repo).Refresh(context.Background(), "invalid-token")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// --- Logout tests ---

func TestLogout_Success(t *testing.T) {
	repo := &mockUserRepo{
		deleteUserRefreshTokens: func(_ context.Context, userID string) error {
			if userID != "user-1" {
				t.Errorf("expected user-1, got %q", userID)
			}
			return nil
		},
	}

	err := newTestService(repo).Logout(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// --- ResetPassword tests ---

func TestResetPassword_Success(t *testing.T) {
	var passwordUpdated, tokensDeleted bool

	repo := &mockUserRepo{
		claimResetToken: func(_ context.Context, _ string) (repository.PasswordResetTokenRow, error) {
			return repository.PasswordResetTokenRow{ID: "rt-1", UserID: "user-1"}, nil
		},
		updatePassword: func(_ context.Context, userID, _ string) error {
			if userID != "user-1" {
				t.Errorf("expected user-1, got %q", userID)
			}
			passwordUpdated = true
			return nil
		},
		deleteUserRefreshTokens: func(_ context.Context, userID string) error {
			if userID != "user-1" {
				t.Errorf("expected user-1, got %q", userID)
			}
			tokensDeleted = true
			return nil
		},
	}

	err := newTestService(repo).ResetPassword(context.Background(), "raw-reset-token", "newpass123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !passwordUpdated {
		t.Error("password was not updated")
	}
	if !tokensDeleted {
		t.Error("refresh tokens were not deleted")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	repo := &mockUserRepo{
		claimResetToken: func(_ context.Context, _ string) (repository.PasswordResetTokenRow, error) {
			return repository.PasswordResetTokenRow{}, errNotFound
		},
	}

	err := newTestService(repo).ResetPassword(context.Background(), "bad-token", "newpass123")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// --- SeedAdmin tests ---

func TestSeedAdmin_CreatesWhenMissing(t *testing.T) {
	var created bool
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
		create: func(_ context.Context, email, _, role string) (repository.UserRow, error) {
			if email != "admin@test.com" {
				t.Errorf("unexpected email %q", email)
			}
			if role != "admin" {
				t.Errorf("unexpected role %q", role)
			}
			created = true
			return repository.UserRow{ID: "new-admin"}, nil
		},
	}

	err := newTestService(repo).SeedAdmin(context.Background(), "admin@test.com", "secret")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !created {
		t.Error("admin was not created")
	}
}

func TestSeedAdmin_SkipsWhenExists(t *testing.T) {
	repo := &mockUserRepo{
		existsByEmail: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}

	err := newTestService(repo).SeedAdmin(context.Background(), "admin@test.com", "secret")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSeedAdmin_SkipsWhenEmpty(t *testing.T) {
	err := newTestService(&mockUserRepo{}).SeedAdmin(context.Background(), "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
