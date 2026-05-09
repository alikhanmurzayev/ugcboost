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

	"github.com/alikhanmurzayev/ugcboost/backend/internal/contract"
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
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx").
			Return((*repository.CampaignRow)(nil), errors.New("db error"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"})
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
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx").
			Return((*repository.CampaignRow)(nil), domain.ErrCampaignNameTaken)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"})
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
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.CreateCampaign(context.Background(), domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"})
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
		campaigns.EXPECT().Create(mock.Anything, "Promo X", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		expected := &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
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

		svc := NewCampaignService(pool, factory, nil, log)
		got, err := svc.CreateCampaign(context.Background(), domain.CampaignInput{Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"})
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

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
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

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
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
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		got, err := svc.GetByID(context.Background(), "c-1")
		require.NoError(t, err)
		require.Equal(t, &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
			IsDeleted: true,
			CreatedAt: created,
			UpdatedAt: created,
		}, got)
	})
}

func TestCampaignService_UpdateCampaign(t *testing.T) {
	t.Parallel()

	t.Run("not found before update", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "missing",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("get returns generic error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return((*repository.CampaignRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorContains(t, err, "get campaign")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("soft-deleted campaign refused with ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("not found between get and update", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(false, nil)
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx", "new_padding_secrettokenxx").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("name taken propagates ErrCampaignNameTaken", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(false, nil)
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx", "new_padding_secrettokenxx").
			Return((*repository.CampaignRow)(nil), domain.ErrCampaignNameTaken)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorIs(t, err, domain.ErrCampaignNameTaken)
	})

	t.Run("update returns generic error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(false, nil)
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx", "new_padding_secrettokenxx").
			Return((*repository.CampaignRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorContains(t, err, "update campaign")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("audit error rolls back", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(false, nil)
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx", "new_padding_secrettokenxx").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo Y",
				TmaURL:    "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: updated,
			}, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with old and new domain payload", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)

		oldCampaign := &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: created,
		}
		newCampaign := &domain.Campaign{
			ID:        "c-1",
			Name:      "Promo Y",
			TmaURL:    "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: updated,
		}
		oldJSON, err := json.Marshal(oldCampaign)
		require.NoError(t, err)
		newJSON, err := json.Marshal(newCampaign)
		require.NoError(t, err)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        oldCampaign.ID,
				Name:      oldCampaign.Name,
				TmaURL:    oldCampaign.TmaURL,
				IsDeleted: oldCampaign.IsDeleted,
				CreatedAt: oldCampaign.CreatedAt,
				UpdatedAt: oldCampaign.UpdatedAt,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(false, nil)
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx", "new_padding_secrettokenxx").
			Return(&repository.CampaignRow{
				ID:        newCampaign.ID,
				Name:      newCampaign.Name,
				TmaURL:    newCampaign.TmaURL,
				IsDeleted: newCampaign.IsDeleted,
				CreatedAt: newCampaign.CreatedAt,
				UpdatedAt: newCampaign.UpdatedAt,
			}, nil)
		entityID := "c-1"
		audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.JSONEq(t, string(oldJSON), string(row.OldValue))
				require.JSONEq(t, string(newJSON), string(row.NewValue))
				row.OldValue = nil
				row.NewValue = nil
				require.Equal(t, repository.AuditLogRow{
					Action:     AuditActionCampaignUpdate,
					EntityType: AuditEntityTypeCampaign,
					EntityID:   pointer.ToString(entityID),
				}, row)
			}).Return(nil).Once()
		log.EXPECT().Info(mock.Anything, "campaign updated", []any{"campaign_id", "c-1"}).Once()

		svc := NewCampaignService(pool, factory, nil, log)
		err = svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.NoError(t, err)
	})

	t.Run("tma_url lock fires when invited rows exist", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID:        "c-1",
				Name:      "Promo X",
				TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").Return(true, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorIs(t, err, domain.ErrCampaignTmaURLLocked)
	})

	t.Run("tma_url lock skipped when tma_url unchanged (no-op same value)", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)

		oldRow := &repository.CampaignRow{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: created,
		}
		newRow := &repository.CampaignRow{
			ID:        "c-1",
			Name:      "Promo Y",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: updated,
		}
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").Return(oldRow, nil)
		// No NewCampaignCreatorRepo / ExistsInvitedInCampaign call: lock skipped.
		campaigns.EXPECT().Update(mock.Anything, "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx").
			Return(newRow, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
		log.EXPECT().Info(mock.Anything, "campaign updated", mock.Anything).Once()

		svc := NewCampaignService(pool, factory, nil, log)
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"})
		require.NoError(t, err)
	})

	t.Run("tma_url lock check error wraps with context", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().GetByID(mock.Anything, "c-1").
			Return(&repository.CampaignRow{
				ID: "c-1", Name: "Promo X", TmaURL: "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
				IsDeleted: false, CreatedAt: created, UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().ExistsInvitedInCampaign(mock.Anything, "c-1").
			Return(false, errors.New("db down"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.UpdateCampaign(context.Background(), "c-1",
			domain.CampaignInput{Name: "Promo Y", TmaURL: "https://tma.ugcboost.kz/tz/new_padding_secrettokenxx"})
		require.ErrorContains(t, err, "check tma_url lock")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignService_List(t *testing.T) {
	t.Parallel()

	t.Run("repo error wraps with context", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		// Captured input asserts that the service passes a trimmed + lowercased
		// search and untouched sort/order/page/perPage to the repo — drift
		// between domain.CampaignListInput and repository.CampaignListParams
		// would slip through if we matched on mock.Anything.
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, p repository.CampaignListParams) {
				require.Equal(t, repository.CampaignListParams{
					Search:    "promo",
					IsDeleted: nil,
					Sort:      domain.CampaignSortCreatedAt,
					Order:     domain.SortOrderDesc,
					Page:      1,
					PerPage:   10,
				}, p)
			}).
			Return(nil, int64(0), errors.New("db down"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.List(context.Background(), domain.CampaignListInput{
			Search:  "  PROMO  ", // trim + lowercase happen in the service
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderDesc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "list campaigns")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("empty result short-circuits with echoed pagination", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Return(nil, int64(0), nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		got, err := svc.List(context.Background(), domain.CampaignListInput{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    3,
			PerPage: 25,
		})
		require.NoError(t, err)
		require.Equal(t, &domain.CampaignListPage{
			Items:   nil,
			Total:   0,
			Page:    3,
			PerPage: 25,
		}, got)
	})

	t.Run("success maps rows to domain campaigns", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)

		isDeleted := true
		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().List(mock.Anything, mock.Anything).
			Run(func(_ context.Context, p repository.CampaignListParams) {
				require.Equal(t, repository.CampaignListParams{
					Search:    "promo",
					IsDeleted: pointer.ToBool(true),
					Sort:      domain.CampaignSortName,
					Order:     domain.SortOrderAsc,
					Page:      2,
					PerPage:   5,
				}, p)
			}).
			Return([]*repository.CampaignRow{
				{ID: "c-1", Name: "Promo A", TmaURL: "https://tma.ugcboost.kz/tz/a_padding_secrettokenxxxx", IsDeleted: false, CreatedAt: created, UpdatedAt: updated},
				{ID: "c-2", Name: "Promo B", TmaURL: "https://tma.ugcboost.kz/tz/b_padding_secrettokenxxxx", IsDeleted: true, CreatedAt: created, UpdatedAt: updated},
			}, int64(7), nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		got, err := svc.List(context.Background(), domain.CampaignListInput{
			Search:    "promo",
			IsDeleted: &isDeleted,
			Sort:      domain.CampaignSortName,
			Order:     domain.SortOrderAsc,
			Page:      2,
			PerPage:   5,
		})
		require.NoError(t, err)
		require.Equal(t, &domain.CampaignListPage{
			Items: []*domain.Campaign{
				{ID: "c-1", Name: "Promo A", TmaURL: "https://tma.ugcboost.kz/tz/a_padding_secrettokenxxxx", IsDeleted: false, CreatedAt: created, UpdatedAt: updated},
				{ID: "c-2", Name: "Promo B", TmaURL: "https://tma.ugcboost.kz/tz/b_padding_secrettokenxxxx", IsDeleted: true, CreatedAt: created, UpdatedAt: updated},
			},
			Total:   7,
			Page:    2,
			PerPage: 5,
		}, got)
	})
}

func TestCampaignService_AssertActiveCampaigns(t *testing.T) {
	t.Parallel()

	t.Run("empty slice is noop, no repo call", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		require.NoError(t, svc.AssertActiveCampaigns(context.Background(), nil))
		require.NoError(t, svc.AssertActiveCampaigns(context.Background(), []string{}))
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().ListByIDs(mock.Anything, []string{"c-1"}).
			Return(nil, errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.AssertActiveCampaigns(context.Background(), []string{"c-1"})
		require.ErrorContains(t, err, "list campaigns by ids")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("all active returns nil", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().ListByIDs(mock.Anything, []string{"c-1", "c-2"}).
			Return([]*repository.CampaignRow{
				{ID: "c-1", Name: "A", TmaURL: "u-a", IsDeleted: false, CreatedAt: created, UpdatedAt: created},
				{ID: "c-2", Name: "B", TmaURL: "u-b", IsDeleted: false, CreatedAt: created, UpdatedAt: created},
			}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		require.NoError(t, svc.AssertActiveCampaigns(context.Background(), []string{"c-1", "c-2"}))
	})

	t.Run("missing id returns ErrCampaignNotAvailableForAdd", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().ListByIDs(mock.Anything, []string{"c-1", "c-missing"}).
			Return([]*repository.CampaignRow{
				{ID: "c-1", Name: "A", TmaURL: "u-a", IsDeleted: false, CreatedAt: created, UpdatedAt: created},
			}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.AssertActiveCampaigns(context.Background(), []string{"c-1", "c-missing"})
		require.ErrorIs(t, err, domain.ErrCampaignNotAvailableForAdd)
	})

	t.Run("soft-deleted id returns ErrCampaignNotAvailableForAdd", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().ListByIDs(mock.Anything, []string{"c-1", "c-deleted"}).
			Return([]*repository.CampaignRow{
				{ID: "c-1", Name: "A", TmaURL: "u-a", IsDeleted: false, CreatedAt: created, UpdatedAt: created},
				{ID: "c-deleted", Name: "B", TmaURL: "u-b", IsDeleted: true, CreatedAt: created, UpdatedAt: created},
			}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		err := svc.AssertActiveCampaigns(context.Background(), []string{"c-1", "c-deleted"})
		require.ErrorIs(t, err, domain.ErrCampaignNotAvailableForAdd)
	})
}

func TestCampaignService_UploadContractTemplate(t *testing.T) {
	t.Parallel()

	const campaignID = "c-1"
	validPDF := []byte("%PDF-1.4 fake template body")

	t.Run("empty body returns ContractRequired", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, nil)
		var cve *domain.ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, domain.CodeContractRequired, cve.Code)
	})

	t.Run("extractor failure returns ContractInvalidPDF", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return(nil, errors.New("pdf.NewReader: malformed"))

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		var cve *domain.ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, domain.CodeContractInvalidPDF, cve.Code)
	})

	t.Run("missing placeholder returns ContractMissingPlaceholder with details", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return([]contract.Placeholder{
				{Page: 1, Name: "CreatorFIO"},
				{Page: 1, Name: "CreatorIIN"},
			}, nil)

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		var cve *domain.ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, domain.CodeContractMissingPlaceholder, cve.Code)
		require.Equal(t, []string{"IssuedDate"}, cve.Missing)
	})

	t.Run("unknown placeholder returns ContractUnknownPlaceholder with details", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return([]contract.Placeholder{
				{Page: 1, Name: "CreatorFIO"},
				{Page: 1, Name: "CreatorIIN"},
				{Page: 1, Name: "IssuedDate"},
				{Page: 1, Name: "BrandName"},
			}, nil)

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		var cve *domain.ContractValidationError
		require.ErrorAs(t, err, &cve)
		require.Equal(t, domain.CodeContractUnknownPlaceholder, cve.Code)
		require.Equal(t, []string{"BrandName"}, cve.Unknown)
	})

	t.Run("repo update sql.ErrNoRows maps to ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return([]contract.Placeholder{
				{Page: 1, Name: "CreatorFIO"},
				{Page: 1, Name: "CreatorIIN"},
				{Page: 1, Name: "IssuedDate"},
			}, nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().UpdateContractTemplate(mock.Anything, campaignID, validPDF).
			Return(sql.ErrNoRows)

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("repo update other error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return([]contract.Placeholder{
				{Page: 1, Name: "CreatorFIO"},
				{Page: 1, Name: "CreatorIIN"},
				{Page: 1, Name: "IssuedDate"},
			}, nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().UpdateContractTemplate(mock.Anything, campaignID, validPDF).
			Return(errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, extractor, logmocks.NewMockLogger(t))
		_, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		require.ErrorContains(t, err, "update contract template")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("happy path writes audit + returns hash and placeholders", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		extractor := svcmocks.NewMockContractExtractor(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)

		extractor.EXPECT().ExtractPlaceholders(validPDF).
			Return([]contract.Placeholder{
				{Page: 1, Name: "CreatorFIO"},
				{Page: 1, Name: "CreatorIIN"},
				{Page: 1, Name: "IssuedDate"},
			}, nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignRepo(mock.Anything).Return(campaigns)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		campaigns.EXPECT().UpdateContractTemplate(mock.Anything, campaignID, validPDF).
			Return(nil)

		var capturedAuditNewValue []byte
		audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignContractTemplateUploaded, row.Action)
				require.Equal(t, AuditEntityTypeCampaign, row.EntityType)
				require.Equal(t, pointer.ToString(campaignID), row.EntityID)
				require.Nil(t, row.OldValue)
				require.NotEmpty(t, row.NewValue)
				capturedAuditNewValue = row.NewValue
			}).Return(nil).Once()

		log.EXPECT().Info(mock.Anything, "contract template uploaded", mock.Anything).Once()

		svc := NewCampaignService(pool, factory, extractor, log)
		got, err := svc.UploadContractTemplate(context.Background(), campaignID, validPDF)
		require.NoError(t, err)
		require.Len(t, got.Hash, 64)
		require.Equal(t, domain.KnownContractPlaceholders, got.Placeholders)

		var auditMeta map[string]any
		require.NoError(t, json.Unmarshal(capturedAuditNewValue, &auditMeta))
		require.Equal(t, got.Hash, auditMeta["hash"])
		require.Equal(t, float64(len(validPDF)), auditMeta["size_bytes"])
		placeholdersAny, ok := auditMeta["placeholders"].([]any)
		require.True(t, ok)
		var placeholders []string
		for _, p := range placeholdersAny {
			placeholders = append(placeholders, p.(string))
		}
		require.Equal(t, domain.KnownContractPlaceholders, placeholders)
	})
}

func TestCampaignService_GetContractTemplate(t *testing.T) {
	t.Parallel()

	const campaignID = "c-1"

	t.Run("not found maps sql.ErrNoRows to ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetContractTemplate(mock.Anything, campaignID).
			Return(nil, sql.ErrNoRows)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.GetContractTemplate(context.Background(), campaignID)
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("empty pdf maps to ErrContractTemplateNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetContractTemplate(mock.Anything, campaignID).
			Return([]byte{}, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.GetContractTemplate(context.Background(), campaignID)
		require.ErrorIs(t, err, domain.ErrContractTemplateNotFound)
	})

	t.Run("propagates other repo errors wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetContractTemplate(mock.Anything, campaignID).
			Return(nil, errors.New("db unavailable"))

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		_, err := svc.GetContractTemplate(context.Background(), campaignID)
		require.ErrorContains(t, err, "get contract template")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("happy path returns bytes", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		pdf := []byte("%PDF-1.4 stored bytes")

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetContractTemplate(mock.Anything, campaignID).
			Return(pdf, nil)

		svc := NewCampaignService(pool, factory, nil, logmocks.NewMockLogger(t))
		got, err := svc.GetContractTemplate(context.Background(), campaignID)
		require.NoError(t, err)
		require.Equal(t, pdf, got)
	})
}
