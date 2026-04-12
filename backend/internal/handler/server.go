package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
)

// AuthService is the interface Server needs from the auth service.
type AuthService interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*service.RefreshResult, error)
	Logout(ctx context.Context, userID string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error)
	GetUser(ctx context.Context, userID string) (*domain.User, error)
	SeedUser(ctx context.Context, email, password, role string) (*domain.User, error)
}

// BrandService is the interface Server needs from the brand service.
type BrandService interface {
	CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error)
	GetBrand(ctx context.Context, id string) (*domain.Brand, error)
	ListBrands(ctx context.Context, userID, role string) ([]*domain.BrandListItem, error)
	UpdateBrand(ctx context.Context, id, name string, logoURL *string) (*domain.Brand, error)
	DeleteBrand(ctx context.Context, id string) error
	ListManagers(ctx context.Context, brandID string) ([]*domain.BrandManager, error)
	AssignManager(ctx context.Context, brandID, email string) (*domain.User, string, error)
	RemoveManager(ctx context.Context, brandID, userID string) error
	CanViewBrand(ctx context.Context, userID, role, brandID string) error
}

// AuditLogService is the interface Server needs from the audit service.
type AuditLogService interface {
	Log(ctx context.Context, entry service.AuditEntry) error
	List(ctx context.Context, f domain.AuditFilter, page, perPage int) ([]*domain.AuditLog, int64, error)
}

// Server implements api.ServerInterface.
type Server struct {
	authService  AuthService
	brandService BrandService
	auditService AuditLogService
	cookieSecure bool
}

var _ api.ServerInterface = (*Server)(nil)

// NewServer creates a new Server.
func NewServer(auth AuthService, brands BrandService, audit AuditLogService, cookieSecure bool) *Server {
	return &Server{authService: auth, brandService: brands, auditService: audit, cookieSecure: cookieSecure}
}
