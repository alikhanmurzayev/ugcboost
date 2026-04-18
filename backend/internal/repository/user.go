package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Users table and column names.
const (
	TableUsers             = "users"
	UserColumnID           = "id"
	UserColumnEmail        = "email"
	UserColumnPasswordHash = "password_hash"
	UserColumnRole         = "role"
	UserColumnCreatedAt    = "created_at"
	UserColumnUpdatedAt    = "updated_at"
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
	TablePasswordResetTokens  = "password_reset_tokens"
	ResetTokenColumnID        = "id"
	ResetTokenColumnUserID    = "user_id"
	ResetTokenColumnTokenHash = "token_hash"
	ResetTokenColumnExpiresAt = "expires_at"
	ResetTokenColumnUsed      = "used"
	ResetTokenColumnCreatedAt = "created_at"
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
	userInsertColumns = sortColumns(userInsertMapper.TagValues()) //nolint:unused // will be used for batch user inserts
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

// UserRepo lists all public methods of the user repository.
type UserRepo interface {
	GetByEmail(ctx context.Context, email string) (*UserRow, error)
	GetByID(ctx context.Context, id string) (*UserRow, error)
	Create(ctx context.Context, email, passwordHash, role string) (*UserRow, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ClaimRefreshToken(ctx context.Context, tokenHash string) (*RefreshTokenRow, error)
	DeleteUserRefreshTokens(ctx context.Context, userID string) error
	SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ClaimResetToken(ctx context.Context, tokenHash string) (*PasswordResetTokenRow, error)
}

type userRepository struct {
	db dbutil.DB
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*UserRow, error) {
	q := sq.Select(userSelectColumns...).
		From(TableUsers).
		Where(sq.Eq{UserColumnEmail: email})
	return dbutil.One[UserRow](ctx, r.db, q)
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*UserRow, error) {
	q := sq.Select(userSelectColumns...).
		From(TableUsers).
		Where(sq.Eq{UserColumnID: id})
	return dbutil.One[UserRow](ctx, r.db, q)
}

func (r *userRepository) Create(ctx context.Context, email, passwordHash, role string) (*UserRow, error) {
	q := sq.Insert(TableUsers).
		SetMap(toMap(UserRow{Email: email, PasswordHash: passwordHash, Role: role}, userInsertMapper)).
		Suffix(returningClause(userSelectColumns))
	return dbutil.One[UserRow](ctx, r.db, q)
}

func (r *userRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	q := sq.Update(TableUsers).
		Set(UserColumnPasswordHash, passwordHash).
		Set(UserColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{UserColumnID: userID})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	q := sq.Select("1").
		From(TableUsers).
		Where(sq.Eq{UserColumnEmail: email}).
		Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *userRepository) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := sq.Insert(TableRefreshTokens).
		SetMap(toMap(RefreshTokenRow{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, refreshTokenInsertMapper))
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

func (r *userRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (*RefreshTokenRow, error) {
	q := sq.Delete(TableRefreshTokens).
		Where(sq.Eq{RefreshTokenColumnTokenHash: tokenHash}).
		Where(sq.Expr(RefreshTokenColumnExpiresAt + " > now()")).
		Suffix(returningClause(refreshTokenSelectColumns))
	return dbutil.One[RefreshTokenRow](ctx, r.db, q)
}

func (r *userRepository) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	q := sq.Delete(TableRefreshTokens).
		Where(sq.Eq{RefreshTokenColumnUserID: userID})
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

func (r *userRepository) SaveResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	q := sq.Insert(TablePasswordResetTokens).
		SetMap(toMap(PasswordResetTokenRow{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, resetTokenInsertMapper))
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

func (r *userRepository) ClaimResetToken(ctx context.Context, tokenHash string) (*PasswordResetTokenRow, error) {
	q := sq.Update(TablePasswordResetTokens).
		Set(ResetTokenColumnUsed, true).
		Where(sq.Eq{ResetTokenColumnTokenHash: tokenHash}).
		Where(sq.Eq{ResetTokenColumnUsed: false}).
		Where(sq.Expr(ResetTokenColumnExpiresAt + " > now()")).
		Suffix(returningClause(resetTokenSelectColumns))
	return dbutil.One[PasswordResetTokenRow](ctx, r.db, q)
}
