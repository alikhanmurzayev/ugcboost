package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_GetByEmail(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT created_at, email, id, password_hash, role, updated_at FROM users WHERE email = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}
		createdAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("alice@example.com").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "email", "id", "password_hash", "role", "updated_at"}).
				AddRow(createdAt, "alice@example.com", "u-1", "hash", "admin", updatedAt))

		got, err := repo.GetByEmail(context.Background(), "alice@example.com")
		require.NoError(t, err)
		require.Equal(t, &UserRow{
			ID:           "u-1",
			Email:        "alice@example.com",
			PasswordHash: "hash",
			Role:         "admin",
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing@example.com").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByEmail(context.Background(), "missing@example.com")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("boom@example.com").
			WillReturnError(errors.New("connection refused"))

		_, err := repo.GetByEmail(context.Background(), "boom@example.com")
		require.ErrorContains(t, err, "connection refused")
	})
}

func TestUserRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT created_at, email, id, password_hash, role, updated_at FROM users WHERE id = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}
		createdAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-42").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "email", "id", "password_hash", "role", "updated_at"}).
				AddRow(createdAt, "bob@example.com", "u-42", "hash", "brand_manager", updatedAt))

		got, err := repo.GetByID(context.Background(), "u-42")
		require.NoError(t, err)
		require.Equal(t, &UserRow{
			ID:           "u-42",
			Email:        "bob@example.com",
			PasswordHash: "hash",
			Role:         "brand_manager",
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "u-missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("u-boom").
			WillReturnError(errors.New("timeout"))

		_, err := repo.GetByID(context.Background(), "u-boom")
		require.ErrorContains(t, err, "timeout")
	})
}

func TestUserRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO users (email,password_hash,role) VALUES ($1,$2,$3) RETURNING created_at, email, id, password_hash, role, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}
		createdAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("bob@example.com", "hashed-pw", "brand_manager").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "email", "id", "password_hash", "role", "updated_at"}).
				AddRow(createdAt, "bob@example.com", "u-new", "hashed-pw", "brand_manager", createdAt))

		got, err := repo.Create(context.Background(), "bob@example.com", "hashed-pw", "brand_manager")
		require.NoError(t, err)
		require.Equal(t, &UserRow{
			ID:           "u-new",
			Email:        "bob@example.com",
			PasswordHash: "hashed-pw",
			Role:         "brand_manager",
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		}, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("bob@example.com", "hashed-pw", "brand_manager").
			WillReturnError(errors.New("unique constraint violation"))

		_, err := repo.Create(context.Background(), "bob@example.com", "hashed-pw", "brand_manager")
		require.ErrorContains(t, err, "unique constraint violation")
	})
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("new-hash", "u-1").
			WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

		err := repo.UpdatePassword(context.Background(), "u-1", "new-hash")
		require.NoError(t, err)
	})

	t.Run("no rows affected returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("new-hash", "u-missing").
			WillReturnResult(pgconn.NewCommandTag("UPDATE 0"))

		err := repo.UpdatePassword(context.Background(), "u-missing", "new-hash")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("new-hash", "u-1").
			WillReturnError(errors.New("connection lost"))

		err := repo.UpdatePassword(context.Background(), "u-1", "new-hash")
		require.ErrorContains(t, err, "connection lost")
	})
}

func TestUserRepository_ExistsByEmail(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT 1 FROM users WHERE email = $1 LIMIT 1"

	t.Run("exists returns true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("alice@example.com").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))

		exists, err := repo.ExistsByEmail(context.Background(), "alice@example.com")
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("no row returns false without error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("nope@example.com").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}))

		exists, err := repo.ExistsByEmail(context.Background(), "nope@example.com")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("boom@example.com").
			WillReturnError(errors.New("db unavailable"))

		exists, err := repo.ExistsByEmail(context.Background(), "boom@example.com")
		require.ErrorContains(t, err, "db unavailable")
		require.False(t, exists)
	})
}

func TestUserRepository_SaveRefreshToken(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO refresh_tokens (expires_at,token_hash,user_id) VALUES ($1,$2,$3)"
	expiresAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(expiresAt, "token-hash", "u-1").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

		err := repo.SaveRefreshToken(context.Background(), "u-1", "token-hash", expiresAt)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(expiresAt, "token-hash", "u-1").
			WillReturnError(errors.New("fk violation"))

		err := repo.SaveRefreshToken(context.Background(), "u-1", "token-hash", expiresAt)
		require.ErrorContains(t, err, "fk violation")
	})
}

func TestUserRepository_ClaimRefreshToken(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM refresh_tokens WHERE token_hash = $1 AND expires_at > now() RETURNING created_at, expires_at, id, token_hash, user_id"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}
		createdAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		expiresAt := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("token-hash").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "expires_at", "id", "token_hash", "user_id"}).
				AddRow(createdAt, expiresAt, "rt-1", "token-hash", "u-1"))

		got, err := repo.ClaimRefreshToken(context.Background(), "token-hash")
		require.NoError(t, err)
		require.Equal(t, &RefreshTokenRow{
			ID:        "rt-1",
			UserID:    "u-1",
			TokenHash: "token-hash",
			ExpiresAt: expiresAt,
			CreatedAt: createdAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing-hash").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.ClaimRefreshToken(context.Background(), "missing-hash")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("hash").
			WillReturnError(errors.New("lock timeout"))

		_, err := repo.ClaimRefreshToken(context.Background(), "hash")
		require.ErrorContains(t, err, "lock timeout")
	})
}

func TestUserRepository_DeleteUserRefreshTokens(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM refresh_tokens WHERE user_id = $1"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("u-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 3"))

		err := repo.DeleteUserRefreshTokens(context.Background(), "u-1")
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("u-1").
			WillReturnError(errors.New("db error"))

		err := repo.DeleteUserRefreshTokens(context.Background(), "u-1")
		require.ErrorContains(t, err, "db error")
	})
}

func TestUserRepository_SaveResetToken(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO password_reset_tokens (expires_at,token_hash,user_id) VALUES ($1,$2,$3)"
	expiresAt := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(expiresAt, "reset-hash", "u-1").
			WillReturnResult(pgconn.NewCommandTag("INSERT 0 1"))

		err := repo.SaveResetToken(context.Background(), "u-1", "reset-hash", expiresAt)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs(expiresAt, "reset-hash", "u-1").
			WillReturnError(errors.New("insert failed"))

		err := repo.SaveResetToken(context.Background(), "u-1", "reset-hash", expiresAt)
		require.ErrorContains(t, err, "insert failed")
	})
}

func TestUserRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const (
		sqlAudit    = "DELETE FROM audit_logs WHERE actor_id = $1"
		sqlBrandMgr = "DELETE FROM brand_managers WHERE user_id = $1"
		sqlUser     = "DELETE FROM users WHERE id = $1"
	)

	t.Run("success deletes audit, brand_managers, users in order", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 3"))
		mock.ExpectExec(sqlBrandMgr).WithArgs("u-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 2"))
		mock.ExpectExec(sqlUser).WithArgs("u-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.DeleteForTests(context.Background(), "u-1")
		require.NoError(t, err)
	})

	t.Run("no audit rows still deletes user", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-2").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlBrandMgr).WithArgs("u-2").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlUser).WithArgs("u-2").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.DeleteForTests(context.Background(), "u-2")
		require.NoError(t, err)
	})

	t.Run("user not found returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlBrandMgr).WithArgs("u-missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlUser).WithArgs("u-missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "u-missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("audit delete error aborts sequence", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-3").
			WillReturnError(errors.New("audit boom"))

		err := repo.DeleteForTests(context.Background(), "u-3")
		require.ErrorContains(t, err, "audit boom")
	})

	t.Run("brand_managers delete error aborts sequence", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-4").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlBrandMgr).WithArgs("u-4").
			WillReturnError(errors.New("fk violation"))

		err := repo.DeleteForTests(context.Background(), "u-4")
		require.ErrorContains(t, err, "fk violation")
	})

	t.Run("user delete error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectExec(sqlAudit).WithArgs("u-5").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlBrandMgr).WithArgs("u-5").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))
		mock.ExpectExec(sqlUser).WithArgs("u-5").
			WillReturnError(errors.New("connection lost"))

		err := repo.DeleteForTests(context.Background(), "u-5")
		require.ErrorContains(t, err, "connection lost")
	})
}

func TestUserRepository_ClaimResetToken(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE password_reset_tokens SET used = $1 WHERE token_hash = $2 AND used = $3 AND expires_at > now() RETURNING created_at, expires_at, id, token_hash, used, user_id"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}
		createdAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		expiresAt := time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "reset-hash", false).
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "expires_at", "id", "token_hash", "used", "user_id"}).
				AddRow(createdAt, expiresAt, "rt-1", "reset-hash", true, "u-1"))

		got, err := repo.ClaimResetToken(context.Background(), "reset-hash")
		require.NoError(t, err)
		require.Equal(t, &PasswordResetTokenRow{
			ID:        "rt-1",
			UserID:    "u-1",
			TokenHash: "reset-hash",
			ExpiresAt: expiresAt,
			Used:      true,
			CreatedAt: createdAt,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "missing-hash", false).
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.ClaimResetToken(context.Background(), "missing-hash")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("wraps other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &userRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs(true, "hash", false).
			WillReturnError(errors.New("constraint error"))

		_, err := repo.ClaimResetToken(context.Background(), "hash")
		require.ErrorContains(t, err, "constraint error")
	})
}
