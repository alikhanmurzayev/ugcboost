package repository

import (
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestRepoFactory(t *testing.T) {
	t.Parallel()

	factory := NewRepoFactory()
	require.NotNil(t, factory)

	db := pgxmockPool(t)

	t.Run("NewUserRepo returns UserRepo bound to db", func(t *testing.T) {
		t.Parallel()
		userRepo := factory.NewUserRepo(db)
		require.NotNil(t, userRepo)
		concrete, ok := userRepo.(*userRepository)
		require.True(t, ok, "factory returns *userRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewBrandRepo returns BrandRepo bound to db", func(t *testing.T) {
		t.Parallel()
		brandRepo := factory.NewBrandRepo(db)
		require.NotNil(t, brandRepo)
		concrete, ok := brandRepo.(*brandRepository)
		require.True(t, ok, "factory returns *brandRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewAuditRepo returns AuditRepo bound to db", func(t *testing.T) {
		t.Parallel()
		auditRepo := factory.NewAuditRepo(db)
		require.NotNil(t, auditRepo)
		concrete, ok := auditRepo.(*auditRepository)
		require.True(t, ok, "factory returns *auditRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewDictionaryRepo returns DictionaryRepo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewDictionaryRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*dictionaryRepository)
		require.True(t, ok, "factory returns *dictionaryRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewCreatorApplicationRepo returns repo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewCreatorApplicationRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*creatorApplicationRepository)
		require.True(t, ok, "factory returns *creatorApplicationRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewCreatorApplicationCategoryRepo returns repo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewCreatorApplicationCategoryRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*creatorApplicationCategoryRepository)
		require.True(t, ok, "factory returns *creatorApplicationCategoryRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewCreatorApplicationSocialRepo returns repo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewCreatorApplicationSocialRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*creatorApplicationSocialRepository)
		require.True(t, ok, "factory returns *creatorApplicationSocialRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewCreatorApplicationConsentRepo returns repo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewCreatorApplicationConsentRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*creatorApplicationConsentRepository)
		require.True(t, ok, "factory returns *creatorApplicationConsentRepository")
		require.Same(t, db, concrete.db)
	})

	t.Run("NewCreatorApplicationTelegramLinkRepo returns repo bound to db", func(t *testing.T) {
		t.Parallel()
		repo := factory.NewCreatorApplicationTelegramLinkRepo(db)
		require.NotNil(t, repo)
		concrete, ok := repo.(*creatorApplicationTelegramLinkRepository)
		require.True(t, ok, "factory returns *creatorApplicationTelegramLinkRepository")
		require.Same(t, db, concrete.db)
	})
}

// pgxmockPool returns a pgxmock pool without any expectation assertions —
// factory tests don't invoke any SQL, so we only need a non-nil dbutil.DB.
func pgxmockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return mock
}
