package repository

import (
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// RepoFactory is a stateless factory that creates repositories.
// Each method accepts a dbutil.DB so repos work transparently with both
// pgxpool.Pool (normal operations) and pgx.Tx (inside transactions).
type RepoFactory struct{}

// NewRepoFactory creates a new repository factory.
func NewRepoFactory() *RepoFactory { return &RepoFactory{} }

// NewUserRepo creates a user repository bound to the given DB.
func (f *RepoFactory) NewUserRepo(db dbutil.DB) UserRepo {
	return &userRepository{db: db}
}

// NewBrandRepo creates a brand repository bound to the given DB.
func (f *RepoFactory) NewBrandRepo(db dbutil.DB) BrandRepo {
	return &brandRepository{db: db}
}

// NewAuditRepo creates an audit repository bound to the given DB.
func (f *RepoFactory) NewAuditRepo(db dbutil.DB) AuditRepo {
	return &auditRepository{db: db}
}
