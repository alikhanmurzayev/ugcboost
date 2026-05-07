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

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

// adminCtx is the canonical authenticated-admin context for happy-path
// tests. Mirrors what middleware injects in production so writeAudit picks
// up actor_id and actor_role correctly (AC line 108).
const adminCtxUserID = "u-admin"

func adminCtx() context.Context {
	return contextWithActor(context.Background(), adminCtxUserID, string(api.Admin))
}

func liveCampaignRow(id string, created time.Time) *repository.CampaignRow {
	return &repository.CampaignRow{
		ID:        id,
		Name:      "Promo X",
		TmaURL:    "https://tma.ugcboost.kz/tz/abc",
		IsDeleted: false,
		CreatedAt: created,
		UpdatedAt: created,
	}
}

func TestCampaignCreatorService_Add(t *testing.T) {
	t.Parallel()

	t.Run("campaign not found surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "missing", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("soft-deleted campaign surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID:        "camp-1",
				Name:      "Promo X",
				IsDeleted: true,
				CreatedAt: created,
				UpdatedAt: created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("campaign GetByID returns generic error wraps with get campaign", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return((*repository.CampaignRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1"})
		require.ErrorContains(t, err, "get campaign")
		require.ErrorContains(t, err, "db unavailable")
		require.NotErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("repo Add returns ErrCreatorAlreadyInCampaign mid-batch rolls back", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			Return((*repository.CampaignCreatorRow)(nil), domain.ErrCreatorAlreadyInCampaign)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1", "cr-2"})
		require.ErrorIs(t, err, domain.ErrCreatorAlreadyInCampaign)
	})

	t.Run("repo Add returns ErrCampaignCreatorCreatorNotFound rolls back", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-bad", domain.CampaignCreatorStatusPlanned).
			Return((*repository.CampaignCreatorRow)(nil), domain.ErrCampaignCreatorCreatorNotFound)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-bad"})
		require.ErrorIs(t, err, domain.ErrCampaignCreatorCreatorNotFound)
	})

	t.Run("repo Add returns ErrCampaignNotFound mid-batch propagates", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			Return((*repository.CampaignCreatorRow)(nil), domain.ErrCampaignNotFound)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("repo Add returns generic error propagates raw", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			Return((*repository.CampaignCreatorRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1"})
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("audit error rolls back", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).
			Return(&repository.CampaignCreatorRow{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
			}, nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.Add(context.Background(), "camp-1", []string{"cr-1"})
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes one audit per creator and emits success log", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
		}
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-1", domain.CampaignCreatorStatusPlanned).Return(row1, nil)
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", "cr-2", domain.CampaignCreatorStatusPlanned).Return(row2, nil)

		expected1 := campaignCreatorRowToDomain(row1)
		expected2 := campaignCreatorRowToDomain(row2)
		expected1JSON, err := json.Marshal(expected1)
		require.NoError(t, err)
		expected2JSON, err := json.Marshal(expected2)
		require.NoError(t, err)

		auditCalls := 0
		audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Nil(t, row.OldValue, "OldValue must be nil for add")
				require.Equal(t, AuditActionCampaignCreatorAdd, row.Action)
				require.Equal(t, AuditEntityTypeCampaignCreator, row.EntityType)
				require.NotNil(t, row.ActorID, "ActorID must carry the admin user id")
				require.Equal(t, adminCtxUserID, *row.ActorID)
				require.Equal(t, string(api.Admin), row.ActorRole)
				require.NotNil(t, row.EntityID)
				switch *row.EntityID {
				case "cc-1":
					require.JSONEq(t, string(expected1JSON), string(row.NewValue))
				case "cc-2":
					require.JSONEq(t, string(expected2JSON), string(row.NewValue))
				default:
					t.Fatalf("unexpected entity_id: %s", *row.EntityID)
				}
				auditCalls++
			}).Return(nil).Times(2)

		log.EXPECT().Info(mock.Anything, "campaign creators added",
			[]any{"campaign_id", "camp-1", "count", 2}).Once()

		svc := NewCampaignCreatorService(pool, factory, log)
		got, err := svc.Add(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, 2, auditCalls)
		require.Equal(t, []*domain.CampaignCreator{expected1, expected2}, got)
	})
}

func TestCampaignCreatorService_Remove(t *testing.T) {
	t.Parallel()

	t.Run("campaign not found surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "missing", "cr-1")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("soft-deleted campaign surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID: "camp-1", IsDeleted: true, CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("missing campaign_creator pair surfaces ErrCampaignCreatorNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return((*repository.CampaignCreatorRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorIs(t, err, domain.ErrCampaignCreatorNotFound)
	})

	t.Run("get returns generic error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return((*repository.CampaignCreatorRow)(nil), errors.New("db unavailable"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "get campaign creator")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("agreed status refused with ErrCampaignCreatorRemoveAfterAgreed", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status: domain.CampaignCreatorStatusAgreed, CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorIs(t, err, domain.ErrCampaignCreatorRemoveAfterAgreed)
	})

	t.Run("delete race surfaces ErrCampaignCreatorNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().DeleteByID(mock.Anything, "cc-1").Return(sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorIs(t, err, domain.ErrCampaignCreatorNotFound)
	})

	t.Run("delete generic error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().DeleteByID(mock.Anything, "cc-1").Return(errors.New("db unavailable"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "delete campaign creator")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("audit error rolls back", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{
				ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
				Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
			}, nil)
		ccRepo.EXPECT().DeleteByID(mock.Anything, "cc-1").Return(nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "audit failed")
	})

	t.Run("success writes audit with full snapshot in OldValue and emits success log", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		row := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
		}
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").Return(row, nil)
		ccRepo.EXPECT().DeleteByID(mock.Anything, "cc-1").Return(nil)

		expected := campaignCreatorRowToDomain(row)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)
		entityID := "cc-1"
		audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Nil(t, row.NewValue, "NewValue must be nil for remove")
				require.JSONEq(t, string(expectedJSON), string(row.OldValue))
				row.OldValue = nil
				require.Equal(t, repository.AuditLogRow{
					ActorID:    pointer.ToString(adminCtxUserID),
					ActorRole:  string(api.Admin),
					Action:     AuditActionCampaignCreatorRemove,
					EntityType: AuditEntityTypeCampaignCreator,
					EntityID:   pointer.ToString(entityID),
				}, row)
			}).Return(nil).Once()
		log.EXPECT().Info(mock.Anything, "campaign creator removed",
			[]any{"campaign_id", "camp-1", "creator_id", "cr-1"}).Once()

		svc := NewCampaignCreatorService(pool, factory, log)
		require.NoError(t, svc.Remove(adminCtx(), "camp-1", "cr-1"))
	})
}

func TestCampaignCreatorService_List(t *testing.T) {
	t.Parallel()

	t.Run("campaign not found surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.List(context.Background(), "missing")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("soft-deleted campaign surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID: "camp-1", IsDeleted: true, CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.List(context.Background(), "camp-1")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("repo error wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		ccRepo.EXPECT().ListByCampaign(mock.Anything, "camp-1").
			Return(nil, errors.New("db unavailable"))

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		_, err := svc.List(context.Background(), "camp-1")
		require.ErrorContains(t, err, "list campaign creators")
		require.ErrorContains(t, err, "db unavailable")
	})

	t.Run("success returns mapped domain rows", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
		}
		ccRepo.EXPECT().ListByCampaign(mock.Anything, "camp-1").
			Return([]*repository.CampaignCreatorRow{row1, row2}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		got, err := svc.List(context.Background(), "camp-1")
		require.NoError(t, err)
		require.Equal(t, []*domain.CampaignCreator{
			campaignCreatorRowToDomain(row1),
			campaignCreatorRowToDomain(row2),
		}, got)
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		ccRepo.EXPECT().ListByCampaign(mock.Anything, "camp-1").
			Return([]*repository.CampaignCreatorRow{}, nil)

		svc := NewCampaignCreatorService(pool, factory, logmocks.NewMockLogger(t))
		got, err := svc.List(context.Background(), "camp-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})
}
