package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	sq "github.com/Masterminds/squirrel"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// users table and column names.
const (
	tableUsers         = "users"
	colUserID          = "id"
	colUserEmail       = "email"
	colUserPasswordHash = "password_hash"
	colUserRole        = "role"
	colUserCreatedAt   = "created_at"
	colUserUpdatedAt   = "updated_at"
)

// refresh_tokens table and column names.
const (
	tableRefreshTokens     = "refresh_tokens"
	colRefreshUserID       = "user_id"
	colRefreshTokenHash    = "token_hash"
	colRefreshExpiresAt    = "expires_at"
)

// password_reset_tokens table and column names.
const (
	tablePasswordResetTokens = "password_reset_tokens"
	colResetUserID           = "user_id"
	colResetTokenHash        = "token_hash"
	colResetExpiresAt        = "expires_at"
	colResetUsed             = "used"
)

// UserRow maps to the users table.
type UserRow struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	Role         string    `db:"role"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// RefreshTokenRow maps to the refresh_tokens table.
type RefreshTokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}

// PasswordResetTokenRow maps to the password_reset_tokens table.
type PasswordResetTokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	TokenHash string    `db:"token_hash"`
	ExpiresAt time.Time `db:"expires_at"`
	Used      bool      `db:"used"`
	CreatedAt time.Time `db:"created_at"`
}

// UserRepository handles user data access.
type UserRepository struct {
	db dbutil.DB
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db dbutil.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByEmail finds a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (UserRow, error) {
	q := dbutil.Psql.Select(colUserID, colUserEmail, colUserPasswordHash, colUserRole, colUserCreatedAt, colUserUpdatedAt).
		From(tableUsers).
		Where(colUserEmail+" = ?", email)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// GetByID finds a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (UserRow, error) {
	q := dbutil.Psql.Select(colUserID, colUserEmail, colUserPasswordHash, colUserRole, colUserCreatedAt, colUserUpdatedAt).
		From(tableUsers).
		Where(colUserID+" = ?", id)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// Create inserts a new user and returns it.
func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) (UserRow, error) {
	q := dbutil.Psql.Insert(tableUsers).
		Columns(colUserEmail, colUserPasswordHash, colUserRole).
		Values(email, passwordHash, role).
		Suffix("RETURNING " + colUserID + ", " + colUserEmail + ", " + colUserPasswordHash + ", " + colUserRole + ", " + colUserCreatedAt + ", " + colUserUpdatedAt)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// UpdatePassword updates the password hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	q := dbutil.Psql.Update(tableUsers).
		Set(colUserPasswordHash, passwordHash).
		Set(colUserUpdatedAt, sq.Expr("now()")).
		Where(colUserID+" = ?", userID)
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ExistsByEmail checks if a user with the given email exists.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	q := dbutil.Psql.Select("1").From(tableUsers).Where(colUserEmail+" = ?", email).Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SaveRefreshToken stores a hashed refresh token.
func (r *UserRepository) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := dbutil.Psql.Insert(tableRefreshTokens).
		Columns(colRefreshUserID, colRefreshTokenHash, colRefreshExpiresAt).
		Values(userID, tokenHash, expiresAt)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimRefreshToken atomically deletes a valid refresh token and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found or expired.
func (r *UserRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (RefreshTokenRow, error) {
	q := dbutil.Psql.Delete(tableRefreshTokens).
		Where(colRefreshTokenHash+" = ? AND "+colRefreshExpiresAt+" > now()", tokenHash).
		Suffix("RETURNING id, user_id, token_hash, expires_at, created_at")
	return dbutil.One[RefreshTokenRow](ctx, r.db, q)
}

// DeleteUserRefreshTokens removes all refresh tokens for a user.
func (r *UserRepository) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	q := dbutil.Psql.Delete(tableRefreshTokens).Where(colRefreshUserID+" = ?", userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// SaveResetToken stores a hashed password reset token.
func (r *UserRepository) SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := dbutil.Psql.Insert(tablePasswordResetTokens).
		Columns(colResetUserID, colResetTokenHash, colResetExpiresAt).
		Values(userID, tokenHash, expiresAt)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimResetToken atomically marks a valid reset token as used and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found, already used, or expired.
func (r *UserRepository) ClaimResetToken(ctx context.Context, tokenHash string) (PasswordResetTokenRow, error) {
	q := dbutil.Psql.Update(tablePasswordResetTokens).
		Set(colResetUsed, true).
		Where(colResetTokenHash+" = ? AND "+colResetUsed+" = false AND "+colResetExpiresAt+" > now()", tokenHash).
		Suffix("RETURNING id, user_id, token_hash, expires_at, used, created_at")
	return dbutil.One[PasswordResetTokenRow](ctx, r.db, q)
}
