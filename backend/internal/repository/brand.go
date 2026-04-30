package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/elgris/stom"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// Brands table and column names.
const (
	TableBrands          = "brands"
	BrandColumnID        = "id"
	BrandColumnName      = "name"
	BrandColumnLogoURL   = "logo_url"
	BrandColumnCreatedAt = "created_at"
	BrandColumnUpdatedAt = "updated_at"
)

// BrandManagers table and column names.
const (
	TableBrandManagers          = "brand_managers"
	BrandManagerColumnID        = "id"
	BrandManagerColumnBrandID   = "brand_id"
	BrandManagerColumnUserID    = "user_id"
	BrandManagerColumnCreatedAt = "created_at"
)

// BrandRow maps to the brands table.
type BrandRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"     insert:"name"`
	LogoURL   *string   `db:"logo_url" insert:"logo_url"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

var (
	brandSelectColumns = sortColumns(stom.MustNewStom(BrandRow{}).SetTag(string(tagSelect)).TagValues())
	brandInsertMapper  = stom.MustNewStom(BrandRow{}).SetTag(string(tagInsert))
)

// BrandWithManagerCount is a brand with manager count for list views.
type BrandWithManagerCount struct {
	ID           string    `db:"id"`
	Name         string    `db:"name"`
	LogoURL      *string   `db:"logo_url"`
	ManagerCount int       `db:"manager_count"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// BrandManagerRow maps to the brand_managers table joined with users.
type BrandManagerRow struct {
	UserID    string    `db:"user_id"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
}

// BrandRepo lists all public methods of the brand repository.
type BrandRepo interface {
	Create(ctx context.Context, name string, logoURL *string) (*BrandRow, error)
	GetByID(ctx context.Context, id string) (*BrandRow, error)
	List(ctx context.Context) ([]*BrandWithManagerCount, error)
	ListByUser(ctx context.Context, userID string) ([]*BrandWithManagerCount, error)
	Update(ctx context.Context, id, name string, logoURL *string) (*BrandRow, error)
	Delete(ctx context.Context, id string) error
	AssignManager(ctx context.Context, brandID, userID string) error
	RemoveManager(ctx context.Context, brandID, userID string) error
	ListManagers(ctx context.Context, brandID string) ([]*BrandManagerRow, error)
	GetBrandIDsForUser(ctx context.Context, userID string) ([]string, error)
	IsManager(ctx context.Context, userID, brandID string) (bool, error)
}

type brandRepository struct {
	db dbutil.DB
}

func (r *brandRepository) Create(ctx context.Context, name string, logoURL *string) (*BrandRow, error) {
	q := sq.Insert(TableBrands).
		SetMap(toMap(BrandRow{Name: name, LogoURL: logoURL}, brandInsertMapper)).
		Suffix(returningClause(brandSelectColumns))
	return dbutil.One[BrandRow](ctx, r.db, q)
}

func (r *brandRepository) GetByID(ctx context.Context, id string) (*BrandRow, error) {
	q := sq.Select(brandSelectColumns...).
		From(TableBrands).
		Where(sq.Eq{BrandColumnID: id})
	return dbutil.One[BrandRow](ctx, r.db, q)
}

func (r *brandRepository) List(ctx context.Context) ([]*BrandWithManagerCount, error) {
	q := sq.Select(
		"b."+BrandColumnID, "b."+BrandColumnName, "b."+BrandColumnLogoURL,
		"COUNT(bm."+BrandManagerColumnID+") AS manager_count",
		"b."+BrandColumnCreatedAt, "b."+BrandColumnUpdatedAt,
	).
		From(TableBrands + " b").
		LeftJoin(TableBrandManagers + " bm ON bm." + BrandManagerColumnBrandID + " = b." + BrandColumnID).
		GroupBy("b." + BrandColumnID).
		OrderBy("b." + BrandColumnCreatedAt + " DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

func (r *brandRepository) ListByUser(ctx context.Context, userID string) ([]*BrandWithManagerCount, error) {
	q := sq.Select(
		"b."+BrandColumnID, "b."+BrandColumnName, "b."+BrandColumnLogoURL,
		"(SELECT COUNT(*) FROM "+TableBrandManagers+" bm2 WHERE bm2."+BrandManagerColumnBrandID+" = b."+BrandColumnID+") AS manager_count",
		"b."+BrandColumnCreatedAt, "b."+BrandColumnUpdatedAt,
	).
		From(TableBrands+" b").
		Join(TableBrandManagers+" bm ON bm."+BrandManagerColumnBrandID+" = b."+BrandColumnID+" AND bm."+BrandManagerColumnUserID+" = ?", userID).
		OrderBy("b." + BrandColumnCreatedAt + " DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

func (r *brandRepository) Update(ctx context.Context, id, name string, logoURL *string) (*BrandRow, error) {
	q := sq.Update(TableBrands).
		Set(BrandColumnName, name).
		Set(BrandColumnLogoURL, logoURL).
		Set(BrandColumnUpdatedAt, sq.Expr("now()")).
		Where(sq.Eq{BrandColumnID: id}).
		Suffix(returningClause(brandSelectColumns))
	return dbutil.One[BrandRow](ctx, r.db, q)
}

func (r *brandRepository) Delete(ctx context.Context, id string) error {
	q := sq.Delete(TableBrands).
		Where(sq.Eq{BrandColumnID: id})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("brand not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *brandRepository) AssignManager(ctx context.Context, brandID, userID string) error {
	q := sq.Insert(TableBrandManagers).
		Columns(BrandManagerColumnBrandID, BrandManagerColumnUserID).
		Values(brandID, userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

func (r *brandRepository) RemoveManager(ctx context.Context, brandID, userID string) error {
	q := sq.Delete(TableBrandManagers).
		Where(sq.Eq{BrandManagerColumnBrandID: brandID}).
		Where(sq.Eq{BrandManagerColumnUserID: userID})
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("manager assignment not found: %w", sql.ErrNoRows)
	}
	return nil
}

func (r *brandRepository) ListManagers(ctx context.Context, brandID string) ([]*BrandManagerRow, error) {
	q := sq.Select("bm."+BrandManagerColumnUserID, "u."+UserColumnEmail, "bm."+BrandManagerColumnCreatedAt).
		From(TableBrandManagers + " bm").
		Join(TableUsers + " u ON u." + UserColumnID + " = bm." + BrandManagerColumnUserID).
		Where(sq.Eq{"bm." + BrandManagerColumnBrandID: brandID}).
		OrderBy("bm." + BrandManagerColumnCreatedAt + " ASC")
	return dbutil.Many[BrandManagerRow](ctx, r.db, q)
}

func (r *brandRepository) GetBrandIDsForUser(ctx context.Context, userID string) ([]string, error) {
	q := sq.Select(BrandManagerColumnBrandID).
		From(TableBrandManagers).
		Where(sq.Eq{BrandManagerColumnUserID: userID})
	return dbutil.Vals[string](ctx, r.db, q)
}

func (r *brandRepository) IsManager(ctx context.Context, userID, brandID string) (bool, error) {
	q := sq.Select("1").
		From(TableBrandManagers).
		Where(sq.Eq{BrandManagerColumnUserID: userID}).
		Where(sq.Eq{BrandManagerColumnBrandID: brandID}).
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
