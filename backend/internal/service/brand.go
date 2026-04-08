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
	Create(ctx context.Context, name string, logoURL *string) (repository.BrandRow, error)
	GetByID(ctx context.Context, id string) (repository.BrandRow, error)
	List(ctx context.Context) ([]repository.BrandWithManagerCount, error)
	ListByUser(ctx context.Context, userID string) ([]repository.BrandWithManagerCount, error)
	Update(ctx context.Context, id, name string, logoURL *string) (repository.BrandRow, error)
	Delete(ctx context.Context, id string) error
	AssignManager(ctx context.Context, brandID, userID string) error
	RemoveManager(ctx context.Context, brandID, userID string) error
	ListManagers(ctx context.Context, brandID string) ([]repository.BrandManagerRow, error)
	IsManager(ctx context.Context, userID, brandID string) (bool, error)
}

// BrandUserRepo is the subset of user repo needed by BrandService.
type BrandUserRepo interface {
	GetByEmail(ctx context.Context, email string) (repository.UserRow, error)
	Create(ctx context.Context, email, passwordHash, role string) (repository.UserRow, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// BrandService handles brand business logic.
type BrandService struct {
	brands BrandRepo
	users  BrandUserRepo
}

// NewBrandService creates a new BrandService.
func NewBrandService(brands BrandRepo, users BrandUserRepo) *BrandService {
	return &BrandService{brands: brands, users: users}
}

// CreateBrand creates a new brand.
func (s *BrandService) CreateBrand(ctx context.Context, name string, logoURL *string) (repository.BrandRow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return repository.BrandRow{}, domain.NewValidationError("VALIDATION_ERROR", "Brand name is required")
	}
	return s.brands.Create(ctx, name, logoURL)
}

// GetBrand returns a brand by ID.
func (s *BrandService) GetBrand(ctx context.Context, id string) (repository.BrandRow, error) {
	return s.brands.GetByID(ctx, id)
}

// ListBrands returns all brands (admin) or user's brands (brand_manager).
func (s *BrandService) ListBrands(ctx context.Context, userID, role string) ([]repository.BrandWithManagerCount, error) {
	if role == "admin" {
		return s.brands.List(ctx)
	}
	return s.brands.ListByUser(ctx, userID)
}

// UpdateBrand updates a brand's name and logo.
func (s *BrandService) UpdateBrand(ctx context.Context, id, name string, logoURL *string) (repository.BrandRow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return repository.BrandRow{}, domain.NewValidationError("VALIDATION_ERROR", "Brand name is required")
	}
	return s.brands.Update(ctx, id, name, logoURL)
}

// DeleteBrand removes a brand.
func (s *BrandService) DeleteBrand(ctx context.Context, id string) error {
	return s.brands.Delete(ctx, id)
}

// ListManagers returns all managers for a brand.
func (s *BrandService) ListManagers(ctx context.Context, brandID string) ([]repository.BrandManagerRow, error) {
	return s.brands.ListManagers(ctx, brandID)
}

// AssignManager assigns a user as brand manager. Creates user if not exists.
// Returns the user and temporary password (if newly created).
func (s *BrandService) AssignManager(ctx context.Context, brandID, email string) (repository.UserRow, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return repository.UserRow{}, "", domain.NewValidationError("VALIDATION_ERROR", "Email is required")
	}

	// Check brand exists
	if _, err := s.brands.GetByID(ctx, brandID); err != nil {
		return repository.UserRow{}, "", fmt.Errorf("get brand: %w", err)
	}

	var user repository.UserRow
	var tempPassword string

	exists, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return repository.UserRow{}, "", fmt.Errorf("check user: %w", err)
	}

	if exists {
		user, err = s.users.GetByEmail(ctx, email)
		if err != nil {
			return repository.UserRow{}, "", fmt.Errorf("get user: %w", err)
		}
	} else {
		tempPassword, err = generateTempPassword()
		if err != nil {
			return repository.UserRow{}, "", fmt.Errorf("generate password: %w", err)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcryptCost)
		if err != nil {
			return repository.UserRow{}, "", fmt.Errorf("hash password: %w", err)
		}
		user, err = s.users.Create(ctx, email, string(hash), "brand_manager")
		if err != nil {
			return repository.UserRow{}, "", fmt.Errorf("create user: %w", err)
		}
		slog.Info("temporary password for new manager", "email", email, "password", tempPassword)
	}

	if err := s.brands.AssignManager(ctx, brandID, user.ID); err != nil {
		return repository.UserRow{}, "", fmt.Errorf("assign manager: %w", err)
	}

	return user, tempPassword, nil
}

// RemoveManager removes a manager from a brand.
func (s *BrandService) RemoveManager(ctx context.Context, brandID, userID string) error {
	return s.brands.RemoveManager(ctx, brandID, userID)
}

// CanViewBrand checks if a user can view a specific brand.
func (s *BrandService) CanViewBrand(ctx context.Context, userID, role, brandID string) error {
	if role == "admin" {
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

const tempPasswordChars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateTempPassword() (string, error) {
	b := make([]byte, 12)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tempPasswordChars))))
		if err != nil {
			return "", err
		}
		b[i] = tempPasswordChars[n.Int64()]
	}
	return string(b), nil
}
