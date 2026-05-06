package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

func TestCampaignService_CreateCampaign(t *testing.T) {
	t.Parallel()

	t.Run("repo error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return((*repository.CampaignRow)(nil), errors.New("db error"))

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorContains(t, err, "db error")
	})

	t.Run("name taken propagates ErrCampaignNameTaken", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return((*repository.CampaignRow)(nil), domain.ErrCampaignNameTaken)

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorIs(t, err, domain.ErrCampaignNameTaken)
	})

	t.Run("audit error aborts tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with full domain payload", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		expected := &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: created,
		}
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)
		entityID := "c-1"
		audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Nil(t, row.OldValue, "OldValue must be nil for create")
				require.JSONEq(t, string(expectedJSON), string(row.NewValue))
				row.NewValue = nil
				require.Equal(t, repository.AuditLogRow{
					Action:     AuditActionCampaignCreate,
					EntityType: AuditEntityTypeCampaign,
					EntityID:   pointer.ToString(entityID),
				}, row)
			}).Return(nil).Once()
		log.EXPECT().Info(mock.Anything, "campaign created", []any{"campaign_id", "c-1"}).Once()

		svc := NewCampaignService(pool, factory, log)
		got, err := svc.CreateCampaign(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestCampaignService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("not found maps sql.ErrNoRows to ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("repo error wraps with context", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return((*repository.CampaignRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.GetByID(context.Background(), "c-1")
		require.ErrorContains(t, err, "get campaign")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("success returns mapped domain campaign", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)

		svc := NewCampaignService(pool, factory, logmocks.NewMockLogger(t))
		got, err := svc.GetByID(context.Background(), "c-1")
		require.NoError(t, err)
		require.Equal(t, &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: true,
			CreatedAt: created,
			UpdatedAt: created,
		}, got)
	})
}
