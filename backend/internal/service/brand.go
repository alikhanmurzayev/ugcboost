package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// BrandRepo is the interface BrandService needs from the brand repository.
type BrandRepo interface {
	Create(ctx context.Context, name string, logoURL *string) (*repository.BrandRow, error)
	GetByID(ctx context.Context, id string) (*repository.BrandRow, error)
	List(ctx context.Context) ([]*repository.BrandWithManagerCount, error)
	ListByUser(ctx context.Context, userID string) ([]*repository.BrandWithManagerCount, error)
	Update(ctx context.Context, id, name string, logoURL *string) (*repository.BrandRow, error)
	Delete(ctx context.Context, id string) error
	AssignManager(ctx context.Context, brandID, userID string) error
	RemoveManager(ctx context.Context, brandID, userID string) error
	ListManagers(ctx context.Context, brandID string) ([]*repository.BrandManagerRow, error)
	IsManager(ctx context.Context, userID, brandID string) (bool, error)
}

// BrandUserRepo is the subset of user repo needed by BrandService.
type BrandUserRepo interface {
	GetByEmail(ctx context.Context, email string) (*repository.UserRow, error)
	Create(ctx context.Context, email, passwordHash, role string) (*repository.UserRow, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// BrandService handles brand business logic.
type BrandService struct {
	brands     BrandRepo
	users      BrandUserRepo
	bcryptCost int
}

// NewBrandService creates a new BrandService.
func NewBrandService(brands BrandRepo, users BrandUserRepo, bcryptCost int) *BrandService {
	return &BrandService{brands: brands, users: users, bcryptCost: bcryptCost}
}

// CreateBrand creates a new brand.
func (s *BrandService) CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Brand name is required")
	}
	row, err := s.brands.Create(ctx, name, logoURL)
	if err != nil {
		return nil, err
	}
	return brandRowToDomain(row), nil
}

// GetBrand returns a brand by ID.
func (s *BrandService) GetBrand(ctx context.Context, id string) (*domain.Brand, error) {
	row, err := s.brands.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return brandRowToDomain(row), nil
}

// ListBrands returns all brands (admin) or user's brands (brand_manager).
func (s *BrandService) ListBrands(ctx context.Context, userID, role string) ([]*domain.BrandListItem, error) {
	var rows []*repository.BrandWithManagerCount
	var err error
	if role == string(domain.RoleAdmin) {
		rows, err = s.brands.List(ctx)
	} else {
		rows, err = s.brands.ListByUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	return brandListRowsToDomain(rows), nil
}

// UpdateBrand updates a brand's name and logo.
func (s *BrandService) UpdateBrand(ctx context.Context, id, name string, logoURL *string) (*domain.Brand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Brand name is required")
	}
	row, err := s.brands.Update(ctx, id, name, logoURL)
	if err != nil {
		return nil, err
	}
	return brandRowToDomain(row), nil
}

// DeleteBrand removes a brand.
func (s *BrandService) DeleteBrand(ctx context.Context, id string) error {
	return s.brands.Delete(ctx, id)
}

// ListManagers returns all managers for a brand.
func (s *BrandService) ListManagers(ctx context.Context, brandID string) ([]*domain.BrandManager, error) {
	rows, err := s.brands.ListManagers(ctx, brandID)
	if err != nil {
		return nil, err
	}
	return managerRowsToDomain(rows), nil
}

// AssignManager assigns a user as brand manager. Creates user if not exists.
// Returns the user and temporary password (if newly created).
func (s *BrandService) AssignManager(ctx context.Context, brandID, email string) (*domain.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, "", domain.NewValidationError(domain.CodeValidation, "Email is required")
	}

	// Check brand exists
	if _, err := s.brands.GetByID(ctx, brandID); err != nil {
		return nil, "", fmt.Errorf("get brand: %w", err)
	}

	var userRow *repository.UserRow
	var tempPassword string

	exists, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, "", fmt.Errorf("check user: %w", err)
	}

	if exists {
		userRow, err = s.users.GetByEmail(ctx, email)
		if err != nil {
			return nil, "", fmt.Errorf("get user: %w", err)
		}
	} else {
		tempPassword, err = generateTempPassword()
		if err != nil {
			return nil, "", fmt.Errorf("generate password: %w", err)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), s.bcryptCost)
		if err != nil {
			return nil, "", fmt.Errorf("hash password: %w", err)
		}
		userRow, err = s.users.Create(ctx, email, string(hash), string(domain.RoleBrandManager))
		if err != nil {
			return nil, "", fmt.Errorf("create user: %w", err)
		}
		slog.Info("temporary password generated for new manager", "email", email)
	}

	if err := s.brands.AssignManager(ctx, brandID, userRow.ID); err != nil {
		return nil, "", fmt.Errorf("assign manager: %w", err)
	}

	u := userRowToDomain(userRow)
	return &u, tempPassword, nil
}

// RemoveManager removes a manager from a brand.
func (s *BrandService) RemoveManager(ctx context.Context, brandID, userID string) error {
	return s.brands.RemoveManager(ctx, brandID, userID)
}

// CanViewBrand checks if a user can view a specific brand.
func (s *BrandService) CanViewBrand(ctx context.Context, userID, role, brandID string) error {
	if role == string(domain.RoleAdmin) {
		return nil
	}
	ok, err := s.brands.IsManager(ctx, userID, brandID)
	if err != nil {
		return fmt.Errorf("check manager: %w", err)
	}
	if !ok {
		return domain.ErrForbidden
	}
	return nil
}

const (
	tempPasswordChars  = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	tempPasswordLength = 12
)

func generateTempPassword() (string, error) {
	b := make([]byte, tempPasswordLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tempPasswordChars))))
		if err != nil {
			return "", err
		}
		b[i] = tempPasswordChars[n.Int64()]
	}
	return string(b), nil
}

func brandRowToDomain(row *repository.BrandRow) *domain.Brand {
	return &domain.Brand{
		ID:        row.ID,
		Name:      row.Name,
		LogoURL:   row.LogoURL,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func brandListRowsToDomain(rows []*repository.BrandWithManagerCount) []*domain.BrandListItem {
	result := make([]*domain.BrandListItem, len(rows))
	for i, row := range rows {
		result[i] = &domain.BrandListItem{
			ID:           row.ID,
			Name:         row.Name,
			LogoURL:      row.LogoURL,
			ManagerCount: row.ManagerCount,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		}
	}
	return result
}

func managerRowsToDomain(rows []*repository.BrandManagerRow) []*domain.BrandManager {
	result := make([]*domain.BrandManager, len(rows))
	for i, row := range rows {
		result[i] = &domain.BrandManager{
			UserID:     row.UserID,
			Email:      row.Email,
			AssignedAt: row.CreatedAt,
		}
	}
	return result
}
