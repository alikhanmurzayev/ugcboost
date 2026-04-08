package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
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
	q := dbutil.Psql.Select("id", "email", "password_hash", "role", "created_at", "updated_at").
		From("users").
		Where("email = ?", email)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// GetByID finds a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (UserRow, error) {
	q := dbutil.Psql.Select("id", "email", "password_hash", "role", "created_at", "updated_at").
		From("users").
		Where("id = ?", id)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// Create inserts a new user and returns it.
func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) (UserRow, error) {
	q := dbutil.Psql.Insert("users").
		Columns("email", "password_hash", "role").
		Values(email, passwordHash, role).
		Suffix("RETURNING id, email, password_hash, role, created_at, updated_at")
	return dbutil.One[UserRow](ctx, r.db, q)
}

// UpdatePassword updates the password hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	q := dbutil.Psql.Update("users").
		Set("password_hash", passwordHash).
		Set("updated_at", sq.Expr("now()")).
		Where("id = ?", userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ExistsByEmail checks if a user with the given email exists.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	q := dbutil.Psql.Select("1").From("users").Where("email = ?", email).Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		return false, nil // not found = doesn't exist
	}
	return true, nil
}

// SaveRefreshToken stores a hashed refresh token.
func (r *UserRepository) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := dbutil.Psql.Insert("refresh_tokens").
		Columns("user_id", "token_hash", "expires_at").
		Values(userID, tokenHash, expiresAt)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimRefreshToken atomically deletes a valid refresh token and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found or expired.
func (r *UserRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (RefreshTokenRow, error) {
	q := dbutil.Psql.Delete("refresh_tokens").
		Where("token_hash = ? AND expires_at > now()", tokenHash).
		Suffix("RETURNING id, user_id, token_hash, expires_at, created_at")
	return dbutil.One[RefreshTokenRow](ctx, r.db, q)
}

// DeleteUserRefreshTokens removes all refresh tokens for a user.
func (r *UserRepository) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	q := dbutil.Psql.Delete("refresh_tokens").Where("user_id = ?", userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// SaveResetToken stores a hashed password reset token.
func (r *UserRepository) SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := dbutil.Psql.Insert("password_reset_tokens").
		Columns("user_id", "token_hash", "expires_at").
		Values(userID, tokenHash, expiresAt)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimResetToken atomically marks a valid reset token as used and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found, already used, or expired.
func (r *UserRepository) ClaimResetToken(ctx context.Context, tokenHash string) (PasswordResetTokenRow, error) {
	q := dbutil.Psql.Update("password_reset_tokens").
		Set("used", true).
		Where("token_hash = ? AND used = false AND expires_at > now()", tokenHash).
		Suffix("RETURNING id, user_id, token_hash, expires_at, used, created_at")
	return dbutil.One[PasswordResetTokenRow](ctx, r.db, q)
}
