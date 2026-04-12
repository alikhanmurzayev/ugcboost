package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

func TestUserRepository_GetByEmail(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.GetByEmail(context.Background(), "alice@example.com")

		require.Equal(t,
			"SELECT created_at, email, id, password_hash, role, updated_at FROM users WHERE email = $1",
			*gotSQL)
		require.Equal(t, []any{"alice@example.com"}, *gotArgs)
	})
}

func TestUserRepository_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.GetByID(context.Background(), "user-42")

		require.Equal(t,
			"SELECT created_at, email, id, password_hash, role, updated_at FROM users WHERE id = $1",
			*gotSQL)
		require.Equal(t, []any{"user-42"}, *gotArgs)
	})
}

func TestUserRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 3)

		_, _ = repo.Create(context.Background(), "bob@example.com", "hashed-pw", "brand_manager")

		require.Equal(t,
			"INSERT INTO users (email,password_hash,role) VALUES ($1,$2,$3) RETURNING created_at, email, id, password_hash, role, updated_at",
			*gotSQL)
		require.Equal(t, []any{"bob@example.com", "hashed-pw", "brand_manager"}, *gotArgs)
	})
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureExec(t, db, 2)

		// captureExec returns CommandTag("OK") which reports 0 rows affected,
		// so UpdatePassword returns pgx.ErrNoRows. That's fine for SQL assertion.
		_ = repo.UpdatePassword(context.Background(), "user-1", "new-hash")

		require.Equal(t,
			"UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2",
			*gotSQL)
		require.Equal(t, []any{"new-hash", "user-1"}, *gotArgs)
	})
}

func TestUserRepository_ExistsByEmail(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 1)

		// captureQuery returns a generic error (not pgx.ErrNoRows),
		// so ExistsByEmail now propagates it. Fine for SQL assertion.
		exists, err := repo.ExistsByEmail(context.Background(), "test@example.com")
		require.Error(t, err)
		require.False(t, exists)

		require.Equal(t,
			"SELECT 1 FROM users WHERE email = $1 LIMIT 1",
			*gotSQL)
		require.Equal(t, []any{"test@example.com"}, *gotArgs)
	})
}

func TestUserRepository_SaveRefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		expiresAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		gotSQL, gotArgs := captureExec(t, db, 3)

		err := repo.SaveRefreshToken(context.Background(), "user-1", "token-hash", expiresAt)
		require.NoError(t, err)

		require.Equal(t,
			"INSERT INTO refresh_tokens (expires_at,token_hash,user_id) VALUES ($1,$2,$3)",
			*gotSQL)
		require.Equal(t, []any{expiresAt, "token-hash", "user-1"}, *gotArgs)
	})
}

func TestUserRepository_ClaimRefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 1)

		_, _ = repo.ClaimRefreshToken(context.Background(), "token-hash")

		require.Equal(t,
			"DELETE FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now() RETURNING created_at, expires_at, id, token_hash, user_id",
			*gotSQL)
		require.Equal(t, []any{"token-hash"}, *gotArgs)
	})
}

func TestUserRepository_DeleteUserRefreshTokens(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureExec(t, db, 1)

		err := repo.DeleteUserRefreshTokens(context.Background(), "user-1")
		require.NoError(t, err)

		require.Equal(t,
			"DELETE FROM refresh_tokens WHERE user_id = $1",
			*gotSQL)
		require.Equal(t, []any{"user-1"}, *gotArgs)
	})
}

func TestUserRepository_SaveResetToken(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		expiresAt := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)
		gotSQL, gotArgs := captureExec(t, db, 3)

		err := repo.SaveResetToken(context.Background(), "user-1", "reset-hash", expiresAt)
		require.NoError(t, err)

		require.Equal(t,
			"INSERT INTO password_reset_tokens (expires_at,token_hash,user_id) VALUES ($1,$2,$3)",
			*gotSQL)
		require.Equal(t, []any{expiresAt, "reset-hash", "user-1"}, *gotArgs)
	})
}

func TestUserRepository_ClaimResetToken(t *testing.T) {
	t.Parallel()

	t.Run("SQL", func(t *testing.T) {
		t.Parallel()
		db := mocks.NewMockDB(t)
		repo := NewUserRepository(db)
		gotSQL, gotArgs := captureQuery(t, db, 2)

		_, _ = repo.ClaimResetToken(context.Background(), "reset-hash")

		require.Equal(t,
			"UPDATE password_reset_tokens SET used = $1 WHERE token_hash = $2 AND used = false AND expires_at > now() RETURNING created_at, expires_at, id, token_hash, used, user_id",
			*gotSQL)
		require.Equal(t, []any{true, "reset-hash"}, *gotArgs)
	})
}
