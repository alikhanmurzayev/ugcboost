package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

type dictionaryServiceRig struct {
	db       dbutil.DB
	factory  *svcmocks.MockDictionaryRepoFactory
	dictRepo *repomocks.MockDictionaryRepo
	logger   *logmocks.MockLogger
}

func newDictionaryServiceRig(t *testing.T) dictionaryServiceRig {
	t.Helper()
	return dictionaryServiceRig{
		db:       dbmocks.NewMockPool(t),
		factory:  svcmocks.NewMockDictionaryRepoFactory(t),
		dictRepo: repomocks.NewMockDictionaryRepo(t),
		logger:   logmocks.NewMockLogger(t),
	}
}

func TestDictionaryService_List(t *testing.T) {
	t.Parallel()

	t.Run("unknown type returns sentinel without hitting repo", func(t *testing.T) {
		t.Parallel()
		rig := newDictionaryServiceRig(t)

		svc := NewDictionaryService(rig.db, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.DictionaryType("unicorns"))
		require.ErrorIs(t, err, domain.ErrDictionaryUnknownType)
	})

	t.Run("categories type maps rows to entries", func(t *testing.T) {
		t.Parallel()
		rig := newDictionaryServiceRig(t)

		rig.factory.EXPECT().NewDictionaryRepo(rig.db).Return(rig.dictRepo)
		rig.dictRepo.EXPECT().ListActive(mock.Anything, repository.TableCategories).
			Return([]*repository.DictionaryEntryRow{
				{Code: "fashion", Name: "Мода / Стиль", Active: true, SortOrder: 10},
				{Code: "beauty", Name: "Бьюти (макияж, уход)", Active: true, SortOrder: 20},
			}, nil)

		svc := NewDictionaryService(rig.db, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.DictionaryTypeCategories)
		require.NoError(t, err)
		require.Equal(t, []domain.DictionaryEntry{
			{Code: "fashion", Name: "Мода / Стиль", SortOrder: 10},
			{Code: "beauty", Name: "Бьюти (макияж, уход)", SortOrder: 20},
		}, got)
	})

	t.Run("cities type uses cities table", func(t *testing.T) {
		t.Parallel()
		rig := newDictionaryServiceRig(t)

		rig.factory.EXPECT().NewDictionaryRepo(rig.db).Return(rig.dictRepo)
		rig.dictRepo.EXPECT().ListActive(mock.Anything, repository.TableCities).
			Return([]*repository.DictionaryEntryRow{
				{Code: "almaty", Name: "Алматы", Active: true, SortOrder: 10},
			}, nil)

		svc := NewDictionaryService(rig.db, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.DictionaryTypeCities)
		require.NoError(t, err)
		require.Equal(t, []domain.DictionaryEntry{
			{Code: "almaty", Name: "Алматы", SortOrder: 10},
		}, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()
		rig := newDictionaryServiceRig(t)

		rig.factory.EXPECT().NewDictionaryRepo(rig.db).Return(rig.dictRepo)
		rig.dictRepo.EXPECT().ListActive(mock.Anything, repository.TableCategories).
			Return(nil, errors.New("db down"))

		svc := NewDictionaryService(rig.db, rig.factory, rig.logger)
		_, err := svc.List(context.Background(), domain.DictionaryTypeCategories)
		require.ErrorContains(t, err, "db down")
	})

	t.Run("empty repo result yields empty entries", func(t *testing.T) {
		t.Parallel()
		rig := newDictionaryServiceRig(t)

		rig.factory.EXPECT().NewDictionaryRepo(rig.db).Return(rig.dictRepo)
		rig.dictRepo.EXPECT().ListActive(mock.Anything, repository.TableCities).Return(nil, nil)

		svc := NewDictionaryService(rig.db, rig.factory, rig.logger)
		got, err := svc.List(context.Background(), domain.DictionaryTypeCities)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}
