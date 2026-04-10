package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)


// brands table and column names.
const (
	tableBrands       = "brands"
	colBrandID        = "id"
	colBrandName      = "name"
	colBrandLogoURL   = "logo_url"
	colBrandCreatedAt = "created_at"
	colBrandUpdatedAt = "updated_at"
)

// brand_managers table and column names.
const (
	tableBrandManagers   = "brand_managers"
	colBMBrandID         = "brand_id"
	colBMUserID          = "user_id"
)

// BrandRow maps to the brands table.
type BrandRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	LogoURL   *string   `db:"logo_url"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

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

// BrandRepository handles brand data access.
type BrandRepository struct {
	db dbutil.DB
}

// NewBrandRepository creates a new BrandRepository.
func NewBrandRepository(db dbutil.DB) *BrandRepository {
	return &BrandRepository{db: db}
}

// Create inserts a new brand and returns it.
func (r *BrandRepository) Create(ctx context.Context, name string, logoURL *string) (BrandRow, error) {
	q := dbutil.Psql.Insert(tableBrands).
		Columns(colBrandName, colBrandLogoURL).
		Values(name, logoURL).
		Suffix("RETURNING " + colBrandID + ", " + colBrandName + ", " + colBrandLogoURL + ", " + colBrandCreatedAt + ", " + colBrandUpdatedAt)
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// GetByID finds a brand by ID.
func (r *BrandRepository) GetByID(ctx context.Context, id string) (BrandRow, error) {
	q := dbutil.Psql.Select(colBrandID, colBrandName, colBrandLogoURL, colBrandCreatedAt, colBrandUpdatedAt).
		From(tableBrands).
		Where(colBrandID+" = ?", id)
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// List returns all brands with manager count.
func (r *BrandRepository) List(ctx context.Context) ([]BrandWithManagerCount, error) {
	q := dbutil.Psql.Select(
		"b."+colBrandID, "b."+colBrandName, "b."+colBrandLogoURL,
		"COUNT(bm.id) AS manager_count",
		"b."+colBrandCreatedAt, "b."+colBrandUpdatedAt,
	).
		From(tableBrands+" b").
		LeftJoin(tableBrandManagers+" bm ON bm."+colBMBrandID+" = b."+colBrandID).
		GroupBy("b.id").
		OrderBy("b.created_at DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

// ListByUser returns brands for a specific user (brand_manager).
func (r *BrandRepository) ListByUser(ctx context.Context, userID string) ([]BrandWithManagerCount, error) {
	q := dbutil.Psql.Select(
		"b."+colBrandID, "b."+colBrandName, "b."+colBrandLogoURL,
		"(SELECT COUNT(*) FROM "+tableBrandManagers+" bm2 WHERE bm2."+colBMBrandID+" = b."+colBrandID+") AS manager_count",
		"b."+colBrandCreatedAt, "b."+colBrandUpdatedAt,
	).
		From(tableBrands+" b").
		Join(tableBrandManagers+" bm ON bm."+colBMBrandID+" = b."+colBrandID+" AND bm."+colBMUserID+" = ?", userID).
		OrderBy("b.created_at DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

// Update updates a brand's name and logo_url.
func (r *BrandRepository) Update(ctx context.Context, id, name string, logoURL *string) (BrandRow, error) {
	q := dbutil.Psql.Update(tableBrands).
		Set(colBrandName, name).
		Set(colBrandLogoURL, logoURL).
		Set(colBrandUpdatedAt, sq.Expr("now()")).
		Where(colBrandID+" = ?", id).
		Suffix("RETURNING " + colBrandID + ", " + colBrandName + ", " + colBrandLogoURL + ", " + colBrandCreatedAt + ", " + colBrandUpdatedAt)
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// Delete removes a brand by ID.
func (r *BrandRepository) Delete(ctx context.Context, id string) error {
	q := dbutil.Psql.Delete(tableBrands).Where(colBrandID+" = ?", id)
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("brand not found: %w", pgx.ErrNoRows)
	}
	return nil
}

// AssignManager creates a brand_managers record.
func (r *BrandRepository) AssignManager(ctx context.Context, brandID, userID string) error {
	q := dbutil.Psql.Insert(tableBrandManagers).
		Columns(colBMBrandID, colBMUserID).
		Values(brandID, userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// RemoveManager deletes a brand_managers record.
func (r *BrandRepository) RemoveManager(ctx context.Context, brandID, userID string) error {
	q := dbutil.Psql.Delete(tableBrandManagers).
		Where(colBMBrandID+" = ? AND "+colBMUserID+" = ?", brandID, userID)
	n, err := dbutil.Exec(ctx, r.db, q)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("manager assignment not found: %w", pgx.ErrNoRows)
	}
	return nil
}

// ListManagers returns all managers for a brand.
func (r *BrandRepository) ListManagers(ctx context.Context, brandID string) ([]BrandManagerRow, error) {
	q := dbutil.Psql.Select("bm."+colBMUserID, "u."+colUserEmail, "bm."+colBrandCreatedAt).
		From(tableBrandManagers+" bm").
		Join(tableUsers+" u ON u."+colUserID+" = bm."+colBMUserID).
		Where("bm."+colBMBrandID+" = ?", brandID).
		OrderBy("bm.created_at ASC")
	return dbutil.Many[BrandManagerRow](ctx, r.db, q)
}

// GetBrandIDsForUser returns all brand IDs a user manages.
func (r *BrandRepository) GetBrandIDsForUser(ctx context.Context, userID string) ([]string, error) {
	q := dbutil.Psql.Select(colBMBrandID).
		From(tableBrandManagers).
		Where(colBMUserID+" = ?", userID)
	return dbutil.Vals[string](ctx, r.db, q)
}

// IsManager checks if a user manages a specific brand.
func (r *BrandRepository) IsManager(ctx context.Context, userID, brandID string) (bool, error) {
	q := dbutil.Psql.Select("1").
		From(tableBrandManagers).
		Where(colBMUserID+" = ? AND "+colBMBrandID+" = ?", userID, brandID).
		Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
