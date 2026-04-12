package repository

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
	"github.com/jackc/pgx/v5"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Users table and column names.
const (
	TableUsers              = "users"
	UserColumnID            = "id"
	UserColumnEmail         = "email"
	UserColumnPasswordHash  = "password_hash"
	UserColumnRole          = "role"
	UserColumnCreatedAt     = "created_at"
	UserColumnUpdatedAt     = "updated_at"
)

// RefreshTokens table and column names.
const (
	TableRefreshTokens          = "refresh_tokens"
	RefreshTokenColumnID        = "id"
	RefreshTokenColumnUserID    = "user_id"
	RefreshTokenColumnTokenHash = "token_hash"
	RefreshTokenColumnExpiresAt = "expires_at"
	RefreshTokenColumnCreatedAt = "created_at"
)

// PasswordResetTokens table and column names.
const (
	TablePasswordResetTokens       = "password_reset_tokens"
	ResetTokenColumnID             = "id"
	ResetTokenColumnUserID         = "user_id"
	ResetTokenColumnTokenHash      = "token_hash"
	ResetTokenColumnExpiresAt      = "expires_at"
	ResetTokenColumnUsed           = "used"
	ResetTokenColumnCreatedAt      = "created_at"
)

// UserRow maps to the users table.
type UserRow struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"         insert:"email"`
	PasswordHash string    `db:"password_hash"  insert:"password_hash"`
	Role         string    `db:"role"           insert:"role"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

var (
	userSelectColumns = sortColumns(stom.MustNewStom(UserRow{}).SetTag(string(tagSelect)).TagValues())
	userInsertMapper  = stom.MustNewStom(UserRow{}).SetTag(string(tagInsert))
	userInsertColumns = sortColumns(userInsertMapper.TagValues())
)

// RefreshTokenRow maps to the refresh_tokens table.
type RefreshTokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"    insert:"user_id"`
	TokenHash string    `db:"token_hash" insert:"token_hash"`
	ExpiresAt time.Time `db:"expires_at" insert:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}

var (
	refreshTokenSelectColumns = sortColumns(stom.MustNewStom(RefreshTokenRow{}).SetTag(string(tagSelect)).TagValues())
	refreshTokenInsertMapper  = stom.MustNewStom(RefreshTokenRow{}).SetTag(string(tagInsert))
)

// PasswordResetTokenRow maps to the password_reset_tokens table.
type PasswordResetTokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"    insert:"user_id"`
	TokenHash string    `db:"token_hash" insert:"token_hash"`
	ExpiresAt time.Time `db:"expires_at" insert:"expires_at"`
	Used      bool      `db:"used"`
	CreatedAt time.Time `db:"created_at"`
}

var (
	resetTokenSelectColumns = sortColumns(stom.MustNewStom(PasswordResetTokenRow{}).SetTag(string(tagSelect)).TagValues())
	resetTokenInsertMapper  = stom.MustNewStom(PasswordResetTokenRow{}).SetTag(string(tagInsert))
)

// UserRepository handles user data access.
type UserRepository struct {
	db dbutil.DB
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db dbutil.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByEmail finds a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*UserRow, error) {
	q := dbutil.Psql.Select(userSelectColumns...).
		From(TableUsers).
		Where(UserColumnEmail+" = ?", email)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// GetByID finds a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*UserRow, error) {
	q := dbutil.Psql.Select(userSelectColumns...).
		From(TableUsers).
		Where(UserColumnID+" = ?", id)
	return dbutil.One[UserRow](ctx, r.db, q)
}

// Create inserts a new user and returns it.
func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) (*UserRow, error) {
	q := dbutil.Psql.Insert(TableUsers).
		SetMap(toMap(UserRow{Email: email, PasswordHash: passwordHash, Role: role}, userInsertMapper)).
		Suffix(returningClause(userSelectColumns))
	return dbutil.One[UserRow](ctx, r.db, q)
}

// UpdatePassword updates the password hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	q := dbutil.Psql.Update(TableUsers).
		Set(UserColumnPasswordHash, passwordHash).
		Set(UserColumnUpdatedAt, sq.Expr("now()")).
		Where(UserColumnID+" = ?", userID)
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
	q := dbutil.Psql.Select("1").From(TableUsers).Where(UserColumnEmail+" = ?", email).Limit(1)
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
	q := dbutil.Psql.Insert(TableRefreshTokens).
		SetMap(toMap(RefreshTokenRow{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, refreshTokenInsertMapper))
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimRefreshToken atomically deletes a valid refresh token and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found or expired.
func (r *UserRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (*RefreshTokenRow, error) {
	q := dbutil.Psql.Delete(TableRefreshTokens).
		Where(RefreshTokenColumnTokenHash+" = ? AND "+RefreshTokenColumnExpiresAt+" > now()", tokenHash).
		Suffix(returningClause(refreshTokenSelectColumns))
	return dbutil.One[RefreshTokenRow](ctx, r.db, q)
}

// DeleteUserRefreshTokens removes all refresh tokens for a user.
func (r *UserRepository) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	q := dbutil.Psql.Delete(TableRefreshTokens).Where(RefreshTokenColumnUserID+" = ?", userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// SaveResetToken stores a hashed password reset token.
func (r *UserRepository) SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := dbutil.Psql.Insert(TablePasswordResetTokens).
		SetMap(toMap(PasswordResetTokenRow{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, resetTokenInsertMapper))
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// ClaimResetToken atomically marks a valid reset token as used and returns it.
// Returns pgx.ErrNoRows (wrapped) if token not found, already used, or expired.
func (r *UserRepository) ClaimResetToken(ctx context.Context, tokenHash string) (*PasswordResetTokenRow, error) {
	q := dbutil.Psql.Update(TablePasswordResetTokens).
		Set(ResetTokenColumnUsed, true).
		Where(ResetTokenColumnTokenHash+" = ? AND "+ResetTokenColumnUsed+" = false AND "+ResetTokenColumnExpiresAt+" > now()", tokenHash).
		Suffix(returningClause(resetTokenSelectColumns))
	return dbutil.One[PasswordResetTokenRow](ctx, r.db, q)
}
