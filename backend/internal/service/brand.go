package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// BrandRepoFactory creates repositories needed by BrandService.
type BrandRepoFactory interface {
	NewBrandRepo(db dbutil.DB) repository.BrandRepo
	NewUserRepo(db dbutil.DB) repository.UserRepo
	NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

// BrandService handles brand business logic.
type BrandService struct {
	pool        dbutil.Pool
	repoFactory BrandRepoFactory
	bcryptCost  int
}

// NewBrandService creates a new BrandService.
func NewBrandService(pool dbutil.Pool, repoFactory BrandRepoFactory, bcryptCost int) *BrandService {
	return &BrandService{pool: pool, repoFactory: repoFactory, bcryptCost: bcryptCost}
}

// CreateBrand creates a new brand and writes the matching audit log
// entry inside the same transaction.
func (s *BrandService) CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Brand name is required")
	}
	var brand *domain.Brand
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		brandRepo := s.repoFactory.NewBrandRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := brandRepo.Create(ctx, name, logoURL)
		if err != nil {
			return err
		}
		brand = brandRowToDomain(row)

		return writeAudit(ctx, auditRepo,
			AuditActionBrandCreate, AuditEntityTypeBrand, brand.ID,
			nil, map[string]string{"name": brand.Name})
	})
	if err != nil {
		return nil, err
	}
	return brand, nil
}

// GetBrand returns a brand by ID.
func (s *BrandService) GetBrand(ctx context.Context, id string) (*domain.Brand, error) {
	brandRepo := s.repoFactory.NewBrandRepo(s.pool)
	row, err := brandRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return brandRowToDomain(row), nil
}

// ListBrands returns brands. If managerID is nil, returns all brands;
// otherwise returns only brands where managerID is a manager.
func (s *BrandService) ListBrands(ctx context.Context, managerID *string) ([]*domain.BrandListItem, error) {
	brandRepo := s.repoFactory.NewBrandRepo(s.pool)
	var rows []*repository.BrandWithManagerCount
	var err error
	if managerID == nil {
		rows, err = brandRepo.List(ctx)
	} else {
		rows, err = brandRepo.ListByUser(ctx, *managerID)
	}
	if err != nil {
		return nil, err
	}
	return brandListRowsToDomain(rows), nil
}

// UpdateBrand updates a brand's name and logo. The change is recorded in
// the audit log in the same transaction.
func (s *BrandService) UpdateBrand(ctx context.Context, id, name string, logoURL *string) (*domain.Brand, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "Brand name is required")
	}
	var brand *domain.Brand
	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		brandRepo := s.repoFactory.NewBrandRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		row, err := brandRepo.Update(ctx, id, name, logoURL)
		if err != nil {
			return err
		}
		brand = brandRowToDomain(row)

		return writeAudit(ctx, auditRepo,
			AuditActionBrandUpdate, AuditEntityTypeBrand, brand.ID,
			nil, map[string]string{"name": brand.Name})
	})
	if err != nil {
		return nil, err
	}
	return brand, nil
}

// DeleteBrand removes a brand and writes the audit log inside the same transaction.
func (s *BrandService) DeleteBrand(ctx context.Context, id string) error {
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		brandRepo := s.repoFactory.NewBrandRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		if err := brandRepo.Delete(ctx, id); err != nil {
			return err
		}

		return writeAudit(ctx, auditRepo,
			AuditActionBrandDelete, AuditEntityTypeBrand, id,
			nil, nil)
	})
}

// ListManagers returns all managers for a brand.
func (s *BrandService) ListManagers(ctx context.Context, brandID string) ([]*domain.BrandManager, error) {
	brandRepo := s.repoFactory.NewBrandRepo(s.pool)
	rows, err := brandRepo.ListManagers(ctx, brandID)
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

	var user *domain.User
	var tempPassword string

	err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		brandRepo := s.repoFactory.NewBrandRepo(tx)
		userRepo := s.repoFactory.NewUserRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		// Check brand exists
		if _, err := brandRepo.GetByID(ctx, brandID); err != nil {
			return fmt.Errorf("get brand: %w", err)
		}

		exists, err := userRepo.ExistsByEmail(ctx, email)
		if err != nil {
			return fmt.Errorf("check user: %w", err)
		}

		var userRow *repository.UserRow
		if exists {
			userRow, err = userRepo.GetByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("get user: %w", err)
			}
		} else {
			tempPassword, err = generateTempPassword()
			if err != nil {
				return fmt.Errorf("generate password: %w", err)
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), s.bcryptCost)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			userRow, err = userRepo.Create(ctx, email, string(hash), string(api.BrandManager))
			if err != nil {
				return fmt.Errorf("create user: %w", err)
			}
			slog.Info("temporary password generated for new manager", "email", email)
		}

		if err := brandRepo.AssignManager(ctx, brandID, userRow.ID); err != nil {
			return fmt.Errorf("assign manager: %w", err)
		}

		u := userRowToDomain(userRow)
		user = &u

		return writeAudit(ctx, auditRepo,
			AuditActionManagerAssign, AuditEntityTypeBrand, brandID,
			nil, map[string]string{"email": user.Email})
	})
	if err != nil {
		return nil, "", err
	}

	return user, tempPassword, nil
}

// RemoveManager removes a manager from a brand and records the audit log in the
// same transaction.
func (s *BrandService) RemoveManager(ctx context.Context, brandID, userID string) error {
	return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
		brandRepo := s.repoFactory.NewBrandRepo(tx)
		auditRepo := s.repoFactory.NewAuditRepo(tx)

		if err := brandRepo.RemoveManager(ctx, brandID, userID); err != nil {
			return err
		}

		return writeAudit(ctx, auditRepo,
			AuditActionManagerRemove, AuditEntityTypeBrand, brandID,
			map[string]string{"userId": userID}, nil)
	})
}

// IsUserBrandManager reports whether the given user is a manager of the brand.
func (s *BrandService) IsUserBrandManager(ctx context.Context, userID, brandID string) (bool, error) {
	brandRepo := s.repoFactory.NewBrandRepo(s.pool)
	ok, err := brandRepo.IsManager(ctx, userID, brandID)
	if err != nil {
		return false, fmt.Errorf("check manager: %w", err)
	}
	return ok, nil
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
