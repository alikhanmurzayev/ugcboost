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

// NewCategoryRepo creates a category repository bound to the given DB.
func (f *RepoFactory) NewCategoryRepo(db dbutil.DB) CategoryRepo {
	return &categoryRepository{db: db}
}

// NewCreatorApplicationRepo creates a creator application repository bound to the given DB.
func (f *RepoFactory) NewCreatorApplicationRepo(db dbutil.DB) CreatorApplicationRepo {
	return &creatorApplicationRepository{db: db}
}

// NewCreatorApplicationCategoryRepo creates a repo for the creator_application_categories join table.
func (f *RepoFactory) NewCreatorApplicationCategoryRepo(db dbutil.DB) CreatorApplicationCategoryRepo {
	return &creatorApplicationCategoryRepository{db: db}
}

// NewCreatorApplicationSocialRepo creates a repo for the creator_application_socials table.
func (f *RepoFactory) NewCreatorApplicationSocialRepo(db dbutil.DB) CreatorApplicationSocialRepo {
	return &creatorApplicationSocialRepository{db: db}
}

// NewCreatorApplicationConsentRepo creates a repo for the creator_application_consents table.
func (f *RepoFactory) NewCreatorApplicationConsentRepo(db dbutil.DB) CreatorApplicationConsentRepo {
	return &creatorApplicationConsentRepository{db: db}
}
