package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

// captureQuery sets up a mock DB.Query expectation that captures SQL and args.
// Returns an error to prevent pgx row scanning (success path is covered by E2E).
func captureQuery(t *testing.T, db *mocks.MockDB, numQueryArgs int) (sql *string, args *[]any) {
	t.Helper()
	var capturedSQL string
	var capturedArgs []any

	matchers := []interface{}{mock.Anything, mock.Anything}
	for i := 0; i < numQueryArgs; i++ {
		matchers = append(matchers, mock.Anything)
	}

	db.On("Query", matchers...).
		Run(func(callArgs mock.Arguments) {
			capturedSQL = callArgs.String(1)
			capturedArgs = make([]any, len(callArgs)-2)
			for i := 2; i < len(callArgs); i++ {
				capturedArgs[i-2] = callArgs[i]
			}
		}).
		Return(nil, errors.New("mock: query intercepted")).
		Once()

	return &capturedSQL, &capturedArgs
}

// captureExec sets up a mock DB.Exec expectation that captures SQL and args.
// Returns a success CommandTag.
func captureExec(t *testing.T, db *mocks.MockDB, numExecArgs int) (sql *string, args *[]any) {
	t.Helper()
	var capturedSQL string
	var capturedArgs []any

	matchers := []interface{}{mock.Anything, mock.Anything}
	for i := 0; i < numExecArgs; i++ {
		matchers = append(matchers, mock.Anything)
	}

	db.On("Exec", matchers...).
		Run(func(callArgs mock.Arguments) {
			capturedSQL = callArgs.String(1)
			capturedArgs = make([]any, len(callArgs)-2)
			for i := 2; i < len(callArgs); i++ {
				capturedArgs[i-2] = callArgs[i]
			}
		}).
		Return(pgconn.NewCommandTag("OK"), nil).
		Once()

	return &capturedSQL, &capturedArgs
}

// --- GetByEmail ---

func TestGetByEmail_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.GetByEmail(context.Background(), "alice@example.com")

	assert.Equal(t,
		"SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE email = $1",
		*gotSQL)
	assert.Equal(t, []any{"alice@example.com"}, *gotArgs)
}

// --- GetByID ---

func TestGetByID_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.GetByID(context.Background(), "user-42")

	assert.Equal(t,
		"SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE id = $1",
		*gotSQL)
	assert.Equal(t, []any{"user-42"}, *gotArgs)
}

// --- Create ---

func TestCreate_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 3)

	_, _ = repo.Create(context.Background(), "bob@example.com", "hashed-pw", "brand_manager")

	assert.Equal(t,
		"INSERT INTO users (email,password_hash,role) VALUES ($1,$2,$3) RETURNING id, email, password_hash, role, created_at, updated_at",
		*gotSQL)
	assert.Equal(t, []any{"bob@example.com", "hashed-pw", "brand_manager"}, *gotArgs)
}

// --- UpdatePassword ---

func TestUpdatePassword_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureExec(t, db, 2)

	err := repo.UpdatePassword(context.Background(), "user-1", "new-hash")
	// captureExec returns CommandTag("OK") which reports 0 rows affected,
	// so UpdatePassword returns pgx.ErrNoRows. That's fine for SQL assertion.
	_ = err

	assert.Equal(t,
		"UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2",
		*gotSQL)
	assert.Equal(t, []any{"new-hash", "user-1"}, *gotArgs)
}

// --- ExistsByEmail ---

func TestExistsByEmail_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	exists, err := repo.ExistsByEmail(context.Background(), "test@example.com")
	// captureQuery returns a generic error (not pgx.ErrNoRows),
	// so ExistsByEmail now propagates it. Fine for SQL assertion.
	assert.Error(t, err)
	assert.False(t, exists)

	assert.Equal(t,
		"SELECT 1 FROM users WHERE email = $1 LIMIT 1",
		*gotSQL)
	assert.Equal(t, []any{"test@example.com"}, *gotArgs)
}

// --- SaveRefreshToken ---

func TestSaveRefreshToken_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	expiresAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	gotSQL, gotArgs := captureExec(t, db, 3)

	err := repo.SaveRefreshToken(context.Background(), "user-1", "token-hash", expiresAt)
	assert.NoError(t, err)

	assert.Equal(t,
		"INSERT INTO refresh_tokens (user_id,token_hash,expires_at) VALUES ($1,$2,$3)",
		*gotSQL)
	assert.Equal(t, []any{"user-1", "token-hash", expiresAt}, *gotArgs)
}

// --- ClaimRefreshToken ---

func TestClaimRefreshToken_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 1)

	_, _ = repo.ClaimRefreshToken(context.Background(), "token-hash")

	assert.Equal(t,
		"DELETE FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now() RETURNING id, user_id, token_hash, expires_at, created_at",
		*gotSQL)
	assert.Equal(t, []any{"token-hash"}, *gotArgs)
}

// --- DeleteUserRefreshTokens ---

func TestDeleteUserRefreshTokens_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureExec(t, db, 1)

	err := repo.DeleteUserRefreshTokens(context.Background(), "user-1")
	assert.NoError(t, err)

	assert.Equal(t,
		"DELETE FROM refresh_tokens WHERE user_id = $1",
		*gotSQL)
	assert.Equal(t, []any{"user-1"}, *gotArgs)
}

// --- SaveResetToken ---

func TestSaveResetToken_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	expiresAt := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)
	gotSQL, gotArgs := captureExec(t, db, 3)

	err := repo.SaveResetToken(context.Background(), "user-1", "reset-hash", expiresAt)
	assert.NoError(t, err)

	assert.Equal(t,
		"INSERT INTO password_reset_tokens (user_id,token_hash,expires_at) VALUES ($1,$2,$3)",
		*gotSQL)
	assert.Equal(t, []any{"user-1", "reset-hash", expiresAt}, *gotArgs)
}

// --- ClaimResetToken ---

func TestClaimResetToken_SQL(t *testing.T) {
	db := mocks.NewMockDB(t)
	repo := NewUserRepository(db)
	gotSQL, gotArgs := captureQuery(t, db, 2)

	_, _ = repo.ClaimResetToken(context.Background(), "reset-hash")

	assert.Equal(t,
		"UPDATE password_reset_tokens SET used = $1 WHERE token_hash = $2 AND used = false AND expires_at > now() RETURNING id, user_id, token_hash, expires_at, used, created_at",
		*gotSQL)
	assert.Equal(t, []any{true, "reset-hash"}, *gotArgs)
}
