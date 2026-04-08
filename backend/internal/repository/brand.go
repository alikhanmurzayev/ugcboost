package repository

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
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
	q := dbutil.Psql.Insert("brands").
		Columns("name", "logo_url").
		Values(name, logoURL).
		Suffix("RETURNING id, name, logo_url, created_at, updated_at")
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// GetByID finds a brand by ID.
func (r *BrandRepository) GetByID(ctx context.Context, id string) (BrandRow, error) {
	q := dbutil.Psql.Select("id", "name", "logo_url", "created_at", "updated_at").
		From("brands").
		Where("id = ?", id)
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// List returns all brands with manager count.
func (r *BrandRepository) List(ctx context.Context) ([]BrandWithManagerCount, error) {
	q := dbutil.Psql.Select(
		"b.id", "b.name", "b.logo_url",
		"COUNT(bm.id) AS manager_count",
		"b.created_at", "b.updated_at",
	).
		From("brands b").
		LeftJoin("brand_managers bm ON bm.brand_id = b.id").
		GroupBy("b.id").
		OrderBy("b.created_at DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

// ListByUser returns brands for a specific user (brand_manager).
func (r *BrandRepository) ListByUser(ctx context.Context, userID string) ([]BrandWithManagerCount, error) {
	q := dbutil.Psql.Select(
		"b.id", "b.name", "b.logo_url",
		"(SELECT COUNT(*) FROM brand_managers bm2 WHERE bm2.brand_id = b.id) AS manager_count",
		"b.created_at", "b.updated_at",
	).
		From("brands b").
		Join("brand_managers bm ON bm.brand_id = b.id AND bm.user_id = ?", userID).
		OrderBy("b.created_at DESC")
	return dbutil.Many[BrandWithManagerCount](ctx, r.db, q)
}

// Update updates a brand's name and logo_url.
func (r *BrandRepository) Update(ctx context.Context, id, name string, logoURL *string) (BrandRow, error) {
	q := dbutil.Psql.Update("brands").
		Set("name", name).
		Set("logo_url", logoURL).
		Set("updated_at", sq.Expr("now()")).
		Where("id = ?", id).
		Suffix("RETURNING id, name, logo_url, created_at, updated_at")
	return dbutil.One[BrandRow](ctx, r.db, q)
}

// Delete removes a brand by ID.
func (r *BrandRepository) Delete(ctx context.Context, id string) error {
	q := dbutil.Psql.Delete("brands").Where("id = ?", id)
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
	q := dbutil.Psql.Insert("brand_managers").
		Columns("brand_id", "user_id").
		Values(brandID, userID)
	_, err := dbutil.Exec(ctx, r.db, q)
	return err
}

// RemoveManager deletes a brand_managers record.
func (r *BrandRepository) RemoveManager(ctx context.Context, brandID, userID string) error {
	q := dbutil.Psql.Delete("brand_managers").
		Where("brand_id = ? AND user_id = ?", brandID, userID)
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
	q := dbutil.Psql.Select("bm.user_id", "u.email", "bm.created_at").
		From("brand_managers bm").
		Join("users u ON u.id = bm.user_id").
		Where("bm.brand_id = ?", brandID).
		OrderBy("bm.created_at ASC")
	return dbutil.Many[BrandManagerRow](ctx, r.db, q)
}

// GetBrandIDsForUser returns all brand IDs a user manages.
func (r *BrandRepository) GetBrandIDsForUser(ctx context.Context, userID string) ([]string, error) {
	q := dbutil.Psql.Select("brand_id").
		From("brand_managers").
		Where("user_id = ?", userID)
	return dbutil.Vals[string](ctx, r.db, q)
}

// IsManager checks if a user manages a specific brand.
func (r *BrandRepository) IsManager(ctx context.Context, userID, brandID string) (bool, error) {
	q := dbutil.Psql.Select("1").
		From("brand_managers").
		Where("user_id = ? AND brand_id = ?", userID, brandID).
		Limit(1)
	_, err := dbutil.Val[int](ctx, r.db, q)
	if err != nil {
		return false, nil
	}
	return true, nil
}
