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
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
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
		TmaURL:    "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx",
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		log.EXPECT().Debug(mock.Anything, "campaign creators added",
			[]any{"campaign_id", "camp-1", "count", 2}).Once()

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), log)
		got, err := svc.Add(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, 2, auditCalls)
		require.Equal(t, []*domain.CampaignCreator{expected1, expected2}, got)
	})

	t.Run("sorts creator ids before insertion for deterministic lock order", func(t *testing.T) {
		// The service clones + sorts creatorIDs so two concurrent batches with
		// overlapping creators acquire the partial unique index in the same
		// order and cannot deadlock (PG 40P01). The contract is observable:
		// repo.Add must be called in ASCII-sorted order regardless of input.
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

		var addOrder []string
		ccRepo.EXPECT().Add(mock.Anything, "camp-1", mock.AnythingOfType("string"), domain.CampaignCreatorStatusPlanned).
			Run(func(_ context.Context, _ string, creatorID string, _ string) {
				addOrder = append(addOrder, creatorID)
			}).
			Return(&repository.CampaignCreatorRow{
				ID: "cc", CampaignID: "camp-1", CreatorID: "cr",
				Status: domain.CampaignCreatorStatusPlanned, CreatedAt: created, UpdatedAt: created,
			}, nil).Times(3)

		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Times(3)
		log.EXPECT().Debug(mock.Anything, "campaign creators added", mock.Anything).Once()

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), log)
		// Reverse-sorted input — service must reorder to ASCII-ascending.
		_, err := svc.Add(adminCtx(), "camp-1", []string{"cr-z", "cr-m", "cr-a"})
		require.NoError(t, err)
		require.Equal(t, []string{"cr-a", "cr-m", "cr-z"}, addOrder)
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		err := svc.Remove(context.Background(), "camp-1", "cr-1")
		require.ErrorContains(t, err, "get campaign creator")
		require.ErrorContains(t, err, "db unavailable")
	})

	for _, status := range []string{
		domain.CampaignCreatorStatusAgreed,
		domain.CampaignCreatorStatusSigning,
		domain.CampaignCreatorStatusSigned,
		domain.CampaignCreatorStatusSigningDeclined,
	} {
		t.Run("status="+status+" refused with ErrCampaignCreatorRemoveAfterAgreed", func(t *testing.T) {
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
					Status: status, CreatedAt: created, UpdatedAt: created,
				}, nil)

			svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
			err := svc.Remove(context.Background(), "camp-1", "cr-1")
			require.ErrorIs(t, err, domain.ErrCampaignCreatorRemoveAfterAgreed)
		})
	}

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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), log)
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
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

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		got, err := svc.List(context.Background(), "camp-1")
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// notifyKitOpts captures per-test customisation knobs for setupNotifyKit.
type notifyKitOpts struct {
	tmaURL string
}

// notifyKit bundles every mock newCampaignCreatorService needs for chunk-12
// flow tests so each t.Run scenario stays focused on the specific assertion
// (validation, partial-success, audit shape) instead of repeating the wiring.
type notifyKit struct {
	pool      *dbmocks.MockPool
	factory   *svcmocks.MockCampaignCreatorRepoFactory
	campaigns *repomocks.MockCampaignRepo
	ccRepo    *repomocks.MockCampaignCreatorRepo
	creator   *repomocks.MockCreatorRepo
	audit     *repomocks.MockAuditRepo
	notifier  *svcmocks.MockCampaignInviteNotifier
	log       *logmocks.MockLogger
	svc       *CampaignCreatorService
	tmaURL    string
}

func setupNotifyKit(t *testing.T, opts notifyKitOpts) *notifyKit {
	t.Helper()
	tmaURL := opts.tmaURL
	if tmaURL == "" {
		tmaURL = "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx"
	}
	pool := dbmocks.NewMockPool(t)
	factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
	campaigns := repomocks.NewMockCampaignRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	creator := repomocks.NewMockCreatorRepo(t)
	audit := repomocks.NewMockAuditRepo(t)
	notifier := svcmocks.NewMockCampaignInviteNotifier(t)
	log := logmocks.NewMockLogger(t)
	svc := NewCampaignCreatorService(pool, factory, notifier, log)
	return &notifyKit{
		pool: pool, factory: factory, campaigns: campaigns, ccRepo: ccRepo,
		creator: creator, audit: audit, notifier: notifier, log: log,
		svc: svc, tmaURL: tmaURL,
	}
}

func (k *notifyKit) liveCampaign(id string) *repository.CampaignRow {
	created := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	return &repository.CampaignRow{
		ID: id, Name: "Promo X", TmaURL: k.tmaURL,
		IsDeleted: false, HasContractTemplate: true,
		CreatedAt: created, UpdatedAt: created,
	}
}

func TestCampaignCreatorService_Notify(t *testing.T) {
	t.Parallel()

	t.Run("rejects with CONTRACT_TEMPLATE_REQUIRED when campaign has no template", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})

		camp := k.liveCampaign("camp-1")
		camp.HasContractTemplate = false

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(camp, nil)

		_, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1"})
		var ve *domain.ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, domain.CodeContractTemplateRequired, ve.Code)
	})

	t.Run("happy path delivers all and writes invite audit per creator", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		oldRow1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		oldRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2"}).
			Return([]*repository.CampaignCreatorRow{oldRow1, oldRow2}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1", "cr-2"}).
			Return(map[string]int64{"cr-1": 1001, "cr-2": 1002}, nil)

		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1001), telegram.CampaignInviteText(), k.tmaURL).Return(nil).Once()
		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1002), telegram.CampaignInviteText(), k.tmaURL).Return(nil).Once()

		newRow1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusInvited, InvitedCount: 1,
			InvitedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		newRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusInvited, InvitedCount: 1,
			InvitedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Times(2)
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-1").Return(newRow1, nil).Once()
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-2").Return(newRow2, nil).Once()

		oldSnap1, err := json.Marshal(campaignCreatorRowToDomain(oldRow1))
		require.NoError(t, err)
		newSnap1, err := json.Marshal(campaignCreatorRowToDomain(newRow1))
		require.NoError(t, err)
		oldSnap2, err := json.Marshal(campaignCreatorRowToDomain(oldRow2))
		require.NoError(t, err)
		newSnap2, err := json.Marshal(campaignCreatorRowToDomain(newRow2))
		require.NoError(t, err)

		auditSeen := map[string]bool{}
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignCreatorInvite, row.Action)
				require.Equal(t, AuditEntityTypeCampaignCreator, row.EntityType)
				require.NotNil(t, row.ActorID, "ActorID must carry the admin user id")
				require.Equal(t, adminCtxUserID, *row.ActorID)
				require.Equal(t, string(api.Admin), row.ActorRole)
				require.NotNil(t, row.EntityID)
				switch *row.EntityID {
				case "cc-1":
					require.JSONEq(t, string(oldSnap1), string(row.OldValue))
					require.JSONEq(t, string(newSnap1), string(row.NewValue))
				case "cc-2":
					require.JSONEq(t, string(oldSnap2), string(row.OldValue))
					require.JSONEq(t, string(newSnap2), string(row.NewValue))
				default:
					t.Fatalf("unexpected entity_id: %s", *row.EntityID)
				}
				auditSeen[*row.EntityID] = true
			}).Return(nil).Times(2)

		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Empty(t, undelivered)
		require.Equal(t, map[string]bool{"cc-1": true, "cc-2": true}, auditSeen)
	})

	t.Run("re-invite from declined resets reminded counter via ApplyInvite", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
		earlier := now.Add(-24 * time.Hour)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		declinedRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusDeclined,
			InvitedCount: 1, InvitedAt: &earlier,
			RemindedCount: 2, RemindedAt: &earlier,
			DecidedAt: &earlier,
			CreatedAt: earlier, UpdatedAt: earlier,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{declinedRow}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{"cr-1": 1001}, nil)

		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1001), telegram.CampaignInviteText(), k.tmaURL).Return(nil).Once()

		// ApplyInvite for declined-source returns the row with reset counters —
		// the SQL CASE branches in the repo do this; the service trusts the
		// returned snapshot. The audit then records the reset transition.
		newRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusInvited,
			InvitedCount: 2, InvitedAt: &now,
			RemindedCount: 0, RemindedAt: nil,
			DecidedAt: nil,
			CreatedAt: earlier, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Once()
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-1").Return(newRow, nil).Once()

		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				oldSnap := domain.CampaignCreator{}
				require.NoError(t, json.Unmarshal(row.OldValue, &oldSnap))
				require.Equal(t, domain.CampaignCreatorStatusDeclined, oldSnap.Status)
				newSnap := domain.CampaignCreator{}
				require.NoError(t, json.Unmarshal(row.NewValue, &newSnap))
				require.Equal(t, domain.CampaignCreatorStatusInvited, newSnap.Status)
				require.Equal(t, 2, newSnap.InvitedCount)
				require.Equal(t, 0, newSnap.RemindedCount)
				require.Nil(t, newSnap.RemindedAt)
				require.Nil(t, newSnap.DecidedAt)
			}).Return(nil).Once()

		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Empty(t, undelivered)
	})

	t.Run("batch validation collects all details and skips delivery", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		// cr-1: agreed (wrong_status). cr-2: not in campaign. cr-3: planned (ok)
		// — but the batch must still fail because of cr-1 / cr-2.
		row3 := &repository.CampaignCreatorRow{
			ID: "cc-3", CampaignID: "camp-1", CreatorID: "cr-3",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusAgreed, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2", "cr-3"}).
			Return([]*repository.CampaignCreatorRow{row1, row3}, nil)

		// No notifier / creator-repo / audit / Begin calls expected — strict-
		// 422 short-circuits before delivery.
		_, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1", "cr-2", "cr-3"})

		var bie *domain.CampaignCreatorBatchInvalidError
		require.ErrorAs(t, err, &bie)
		require.Len(t, bie.Details, 2)
		require.ElementsMatch(t, []domain.BatchValidationDetail{
			{CreatorID: "cr-1", Reason: domain.BatchInvalidReasonWrongStatus, CurrentStatus: domain.CampaignCreatorStatusAgreed},
			{CreatorID: "cr-2", Reason: domain.BatchInvalidReasonNotInCampaign},
		}, bie.Details)
	})

	t.Run("partial-success: bot_blocked entry, others succeed", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2"}).
			Return([]*repository.CampaignCreatorRow{row1, row2}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1", "cr-2"}).
			Return(map[string]int64{"cr-1": 1001, "cr-2": 1002}, nil)

		// cr-1 fails delivery → no DB write, recorded as bot_blocked.
		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1001), telegram.CampaignInviteText(), k.tmaURL).
			Return(errors.New("Forbidden: bot was blocked by the user")).Once()
		// cr-2 succeeds → ApplyInvite + audit fire.
		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1002), telegram.CampaignInviteText(), k.tmaURL).
			Return(nil).Once()

		newRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusInvited, InvitedCount: 1,
			InvitedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Once()
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-2").Return(newRow2, nil).Once()

		// Audit must fire only for cr-2 (the row that actually got delivered).
		// The .Run callback pins the entity id so a future regression that
		// writes an audit for the bot_blocked cr-1 is caught immediately
		// instead of silently passing the .Once() counter.
		auditSeen := map[string]bool{}
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignCreatorInvite, row.Action)
				require.NotNil(t, row.EntityID)
				auditSeen[*row.EntityID] = true
			}).Return(nil).Once()

		k.log.EXPECT().Warn(mock.Anything, "campaign batch: telegram delivery failed", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonBotBlocked},
		}, undelivered)
		require.Equal(t, map[string]bool{"cc-2": true}, auditSeen)
	})

	t.Run("soft-deleted campaign returns 404", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID: "camp-1", IsDeleted: true, CreatedAt: now, UpdatedAt: now,
			}, nil)

		_, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("missing campaign returns 404", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "missing").Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		_, err := k.svc.Notify(adminCtx(), "missing", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("send succeeds but apply tx fails — reported as unknown, batch continues", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2"}).
			Return([]*repository.CampaignCreatorRow{row1, row2}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1", "cr-2"}).
			Return(map[string]int64{"cr-1": 1001, "cr-2": 1002}, nil)

		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1001), telegram.CampaignInviteText(), k.tmaURL).Return(nil).Once()
		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1002), telegram.CampaignInviteText(), k.tmaURL).Return(nil).Once()

		// cr-1: apply fails after a successful send → must surface as unknown
		// undelivered without aborting the batch.
		// cr-2: apply succeeds normally and writes audit.
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Times(2)
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-1").Return(nil, sql.ErrNoRows).Once()
		newRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusInvited, InvitedCount: 1,
			InvitedAt: &now, CreatedAt: now, UpdatedAt: now,
		}
		k.ccRepo.EXPECT().ApplyInvite(mock.Anything, "cc-2").Return(newRow2, nil).Once()
		// Audit fires only for cr-2 (cr-1 was sent but apply errored, so its
		// row state is uncertain — by contract we record undelivered and skip
		// the audit). Pinning entity_id catches a future code-path that
		// accidentally writes an audit for the unpersisted cr-1.
		auditSeen := map[string]bool{}
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.NotNil(t, row.EntityID)
				auditSeen[*row.EntityID] = true
			}).Return(nil).Once()

		k.log.EXPECT().Error(mock.Anything, "campaign batch: telegram sent but persist failed", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonUnknown},
		}, undelivered)
		require.Equal(t, map[string]bool{"cc-2": true}, auditSeen)
	})

	t.Run("missing telegram_user_id reported as unknown without delivery", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{row1}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{}, nil)

		k.log.EXPECT().Error(mock.Anything, "campaign batch: missing telegram_user_id", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.Notify(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonUnknown},
		}, undelivered)
	})
}

func TestCampaignCreatorService_RemindInvitation(t *testing.T) {
	t.Parallel()

	t.Run("happy path bumps reminded counter and writes remind audit", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
		earlier := now.Add(-time.Hour)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		invitedRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusInvited,
			InvitedCount: 1, InvitedAt: &earlier,
			CreatedAt: earlier, UpdatedAt: earlier,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{invitedRow}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{"cr-1": 1001}, nil)

		k.notifier.EXPECT().SendCampaignInvite(mock.Anything, int64(1001), telegram.CampaignRemindInvitationText(), k.tmaURL).Return(nil).Once()

		newRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusInvited,
			InvitedCount: 1, InvitedAt: &earlier,
			RemindedCount: 1, RemindedAt: &now,
			CreatedAt: earlier, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Once()
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyRemind(mock.Anything, "cc-1").Return(newRow, nil).Once()

		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignCreatorRemind, row.Action)
				newSnap := domain.CampaignCreator{}
				require.NoError(t, json.Unmarshal(row.NewValue, &newSnap))
				require.Equal(t, 1, newSnap.RemindedCount)
				require.NotNil(t, newSnap.RemindedAt)
			}).Return(nil).Once()

		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindInvitation(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Empty(t, undelivered)
	})

	t.Run("planned creator rejected with wrong_status", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusPlanned, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{row}, nil)

		_, err := k.svc.RemindInvitation(adminCtx(), "camp-1", []string{"cr-1"})

		var bie *domain.CampaignCreatorBatchInvalidError
		require.ErrorAs(t, err, &bie)
		require.Equal(t, []domain.BatchValidationDetail{{
			CreatorID: "cr-1", Reason: domain.BatchInvalidReasonWrongStatus,
			CurrentStatus: domain.CampaignCreatorStatusPlanned,
		}}, bie.Details)
	})
}

func TestCampaignCreatorService_RemindSigning(t *testing.T) {
	t.Parallel()

	t.Run("missing campaign returns 404", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "missing").Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		_, err := k.svc.RemindSigning(adminCtx(), "missing", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("soft-deleted campaign returns 404", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID: "camp-1", IsDeleted: true, CreatedAt: now, UpdatedAt: now,
			}, nil)

		_, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1"})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("not_in_campaign creator rejected", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-orphan"}).
			Return([]*repository.CampaignCreatorRow{}, nil)

		_, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-orphan"})

		var bie *domain.CampaignCreatorBatchInvalidError
		require.ErrorAs(t, err, &bie)
		require.Equal(t, []domain.BatchValidationDetail{{
			CreatorID: "cr-orphan", Reason: domain.BatchInvalidReasonNotInCampaign,
		}}, bie.Details)
		k.notifier.AssertNotCalled(t, "SendCampaignInvite", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		k.ccRepo.AssertNotCalled(t, "ApplyRemind", mock.Anything, mock.Anything)
	})

	t.Run("wrong_status table-driven for every non-signing status", func(t *testing.T) {
		t.Parallel()

		// Every status except `signing` must be rejected with wrong_status —
		// table-driven keeps the matrix exhaustive and a future enum addition
		// gets caught the moment a new status is forgotten here.
		cases := []string{
			domain.CampaignCreatorStatusPlanned,
			domain.CampaignCreatorStatusInvited,
			domain.CampaignCreatorStatusAgreed,
			domain.CampaignCreatorStatusSigned,
			domain.CampaignCreatorStatusSigningDeclined,
			domain.CampaignCreatorStatusDeclined,
		}
		for _, current := range cases {
			t.Run(current, func(t *testing.T) {
				t.Parallel()
				k := setupNotifyKit(t, notifyKitOpts{})
				now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

				k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
				k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

				row := &repository.CampaignCreatorRow{
					ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
					Status: current, CreatedAt: now, UpdatedAt: now,
				}
				k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
				k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
					Return([]*repository.CampaignCreatorRow{row}, nil)

				_, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1"})

				var bie *domain.CampaignCreatorBatchInvalidError
				require.ErrorAs(t, err, &bie)
				require.Equal(t, []domain.BatchValidationDetail{{
					CreatorID: "cr-1", Reason: domain.BatchInvalidReasonWrongStatus,
					CurrentStatus: current,
				}}, bie.Details)
				k.notifier.AssertNotCalled(t, "SendCampaignInvite", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("bot_blocked entry recorded, ApplyRemind not called, batch continues", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2"}).
			Return([]*repository.CampaignCreatorRow{row1, row2}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1", "cr-2"}).
			Return(map[string]int64{"cr-1": 1001, "cr-2": 1002}, nil)

		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1001), telegram.CampaignRemindSigningText()).
			Return(errors.New("Forbidden: bot was blocked by the user")).Once()
		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1002), telegram.CampaignRemindSigningText()).
			Return(nil).Once()

		newRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status:        domain.CampaignCreatorStatusSigning,
			RemindedCount: 1, RemindedAt: &now,
			CreatedAt: now, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Once()
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyRemind(mock.Anything, "cc-2").Return(newRow2, nil).Once()

		auditSeen := map[string]bool{}
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignCreatorRemindSigning, row.Action)
				require.NotNil(t, row.EntityID)
				auditSeen[*row.EntityID] = true
			}).Return(nil).Once()

		k.log.EXPECT().Warn(mock.Anything, "campaign batch: telegram delivery failed", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonBotBlocked},
		}, undelivered)
		require.Equal(t, map[string]bool{"cc-2": true}, auditSeen)
		k.ccRepo.AssertNotCalled(t, "ApplyRemind", mock.Anything, "cc-1")
	})

	t.Run("unknown telegram error recorded without ApplyRemind", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{row1}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{"cr-1": 1001}, nil)

		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1001), telegram.CampaignRemindSigningText()).
			Return(errors.New("network timeout")).Once()

		k.log.EXPECT().Warn(mock.Anything, "campaign batch: telegram delivery failed", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonUnknown},
		}, undelivered)
		k.ccRepo.AssertNotCalled(t, "ApplyRemind", mock.Anything, mock.Anything)
	})

	t.Run("missing telegram_user_id reported as unknown without delivery", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{row1}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{}, nil)

		k.log.EXPECT().Error(mock.Anything, "campaign batch: missing telegram_user_id", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonUnknown},
		}, undelivered)
		k.notifier.AssertNotCalled(t, "SendCampaignReminder", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("send succeeds but ApplyRemind fails — reported as unknown, batch continues", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		row1 := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		row2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status: domain.CampaignCreatorStatusSigning, CreatedAt: now, UpdatedAt: now,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1", "cr-2"}).
			Return([]*repository.CampaignCreatorRow{row1, row2}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1", "cr-2"}).
			Return(map[string]int64{"cr-1": 1001, "cr-2": 1002}, nil)

		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1001), telegram.CampaignRemindSigningText()).Return(nil).Once()
		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1002), telegram.CampaignRemindSigningText()).Return(nil).Once()

		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Times(2)
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyRemind(mock.Anything, "cc-1").Return(nil, sql.ErrNoRows).Once()
		newRow2 := &repository.CampaignCreatorRow{
			ID: "cc-2", CampaignID: "camp-1", CreatorID: "cr-2",
			Status:        domain.CampaignCreatorStatusSigning,
			RemindedCount: 1, RemindedAt: &now,
			CreatedAt: now, UpdatedAt: now,
		}
		k.ccRepo.EXPECT().ApplyRemind(mock.Anything, "cc-2").Return(newRow2, nil).Once()

		auditSeen := map[string]bool{}
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCampaignCreatorRemindSigning, row.Action)
				require.NotNil(t, row.EntityID)
				auditSeen[*row.EntityID] = true
			}).Return(nil).Once()

		k.log.EXPECT().Error(mock.Anything, "campaign batch: telegram sent but persist failed", mock.Anything).Once()
		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1", "cr-2"})
		require.NoError(t, err)
		require.Equal(t, []domain.NotifyFailure{
			{CreatorID: "cr-1", Reason: domain.NotifyFailureReasonUnknown},
		}, undelivered)
		require.Equal(t, map[string]bool{"cc-2": true}, auditSeen)
	})

	t.Run("happy path bumps reminded counter, status stays signing, audit shape", func(t *testing.T) {
		t.Parallel()
		k := setupNotifyKit(t, notifyKitOpts{})
		now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
		earlier := now.Add(-time.Hour)

		k.factory.EXPECT().NewCampaignRepo(k.pool).Return(k.campaigns)
		k.campaigns.EXPECT().GetByID(mock.Anything, "camp-1").Return(k.liveCampaign("camp-1"), nil)

		signingRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusSigning,
			InvitedCount: 1, InvitedAt: &earlier,
			RemindedCount: 0, DecidedAt: &earlier,
			CreatedAt: earlier, UpdatedAt: earlier,
		}
		k.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(k.ccRepo)
		k.ccRepo.EXPECT().ListByCampaignAndCreators(mock.Anything, "camp-1", []string{"cr-1"}).
			Return([]*repository.CampaignCreatorRow{signingRow}, nil)

		k.factory.EXPECT().NewCreatorRepo(k.pool).Return(k.creator)
		k.creator.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).
			Return(map[string]int64{"cr-1": 1001}, nil)

		// Captured-input: pin the exact text passed to the notifier. Catches
		// a future copy-paste where the batchOpSpec accidentally references
		// CampaignRemindInvitationText (or any other constant) instead of
		// CampaignRemindSigningText. mock.Anything on text would silently
		// pass the regression. The 3-arg signature also enforces that
		// remind-signing uses SendCampaignReminder (no WebApp button) and
		// not SendCampaignInvite.
		var capturedText string
		k.notifier.EXPECT().SendCampaignReminder(mock.Anything, int64(1001), mock.Anything).
			Run(func(_ context.Context, _ int64, text string) {
				capturedText = text
			}).
			Return(nil).Once()

		newRow := &repository.CampaignCreatorRow{
			ID: "cc-1", CampaignID: "camp-1", CreatorID: "cr-1",
			Status:       domain.CampaignCreatorStatusSigning,
			InvitedCount: 1, InvitedAt: &earlier,
			RemindedCount: 1, RemindedAt: &now,
			DecidedAt: &earlier,
			CreatedAt: earlier, UpdatedAt: now,
		}
		k.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Once()
		k.factory.EXPECT().NewAuditRepo(mock.Anything).Return(k.audit)
		k.ccRepo.EXPECT().ApplyRemind(mock.Anything, "cc-1").Return(newRow, nil).Once()

		var auditRow repository.AuditLogRow
		k.audit.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				auditRow = row
			}).Return(nil).Once()

		k.log.EXPECT().Info(mock.Anything, "campaign batch dispatched", mock.Anything).Once()

		undelivered, err := k.svc.RemindSigning(adminCtx(), "camp-1", []string{"cr-1"})
		require.NoError(t, err)
		require.Empty(t, undelivered)

		require.Equal(t, telegram.CampaignRemindSigningText(), capturedText)

		require.Equal(t, AuditActionCampaignCreatorRemindSigning, auditRow.Action)
		require.Equal(t, AuditEntityTypeCampaignCreator, auditRow.EntityType)
		require.NotNil(t, auditRow.EntityID)
		require.Equal(t, "cc-1", *auditRow.EntityID)
		require.NotNil(t, auditRow.ActorID)
		require.Equal(t, adminCtxUserID, *auditRow.ActorID)
		require.Equal(t, string(api.Admin), auditRow.ActorRole)
		oldSnap := domain.CampaignCreator{}
		require.NoError(t, json.Unmarshal(auditRow.OldValue, &oldSnap))
		require.Equal(t, domain.CampaignCreatorStatusSigning, oldSnap.Status)
		require.Equal(t, 0, oldSnap.RemindedCount)
		require.Nil(t, oldSnap.RemindedAt)
		newSnap := domain.CampaignCreator{}
		require.NoError(t, json.Unmarshal(auditRow.NewValue, &newSnap))
		require.Equal(t, domain.CampaignCreatorStatusSigning, newSnap.Status)
		require.Equal(t, 1, newSnap.RemindedCount)
		require.NotNil(t, newSnap.RemindedAt)
		require.WithinDuration(t, now, *newSnap.RemindedAt, time.Minute)
	})
}

func TestCampaignCreatorService_PatchParticipation(t *testing.T) {
	t.Parallel()

	signedRow := func(id string, ticketSentAt *time.Time, created time.Time) *repository.CampaignCreatorRow {
		return &repository.CampaignCreatorRow{
			ID:           id,
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusSigned,
			TicketSentAt: ticketSentAt,
			CreatedAt:    created,
			UpdatedAt:    created,
		}
	}

	t.Run("campaign not found surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "missing").
			Return((*repository.CampaignRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "missing", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("soft-deleted campaign surfaces ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(&repository.CampaignRow{
				ID: "camp-1", IsDeleted: true, CreatedAt: created, UpdatedAt: created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("missing campaign_creator pair surfaces ErrCampaignCreatorNotFound", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-missing").
			Return((*repository.CampaignCreatorRow)(nil), sql.ErrNoRows)

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-missing", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorIs(t, err, domain.ErrCampaignCreatorNotFound)
	})

	t.Run("status != signed surfaces ErrCampaignCreatorTicketSentBadStatus", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{
				ID:         "cc-1",
				CampaignID: "camp-1",
				CreatorID:  "cr-1",
				Status:     domain.CampaignCreatorStatusSigning,
				CreatedAt:  created,
				UpdatedAt:  created,
			}, nil)

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorIs(t, err, domain.ErrCampaignCreatorTicketSentBadStatus)
	})

	t.Run("no-op returns row unchanged without UPDATE or audit", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
		ticketSentAt := created.Add(-time.Hour)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepo)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(signedRow("cc-1", &ticketSentAt, created), nil)
		// No Begin, no UpdateTicketSentAt, no audit — strict mocks would fail
		// if anything inside the WithTx scope fired.

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		got, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotNil(t, got.TicketSentAt)
		require.Equal(t, ticketSentAt, *got.TicketSentAt)
	})

	t.Run("set flips nil → timestamp with audit + success log", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepoPool := repomocks.NewMockCampaignCreatorRepo(t)
		ccRepoTx := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
		now := time.Date(2026, 5, 11, 13, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepoPool)
		ccRepoPool.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(signedRow("cc-1", nil, created), nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepoTx)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		var capturedSentAt *time.Time
		ccRepoTx.EXPECT().UpdateTicketSentAt(mock.Anything, "cc-1", mock.AnythingOfType("*time.Time")).
			Run(func(_ context.Context, _ string, sentAt *time.Time) {
				capturedSentAt = sentAt
			}).
			Return(signedRow("cc-1", &now, now), nil)

		var auditRow repository.AuditLogRow
		audit.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				auditRow = row
			}).
			Return(nil)

		log.EXPECT().Info(mock.Anything, "campaign creator ticket_sent toggled",
			[]any{"campaign_id", "camp-1", "creator_id", "cr-1", "ticket_sent", true}).Once()

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), log)
		got, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.NoError(t, err)
		require.NotNil(t, got.TicketSentAt)
		require.Equal(t, now, *got.TicketSentAt)

		// UPDATE received a non-nil timestamp roughly equal to now (allow ±1m
		// because the service uses time.Now() internally).
		require.NotNil(t, capturedSentAt)
		require.WithinDuration(t, time.Now().UTC(), *capturedSentAt, time.Minute)

		// Audit row encodes the diff with actor + entity matches.
		require.Equal(t, AuditActionCampaignCreatorTicketSent, auditRow.Action)
		require.Equal(t, AuditEntityTypeCampaignCreator, auditRow.EntityType)
		require.NotNil(t, auditRow.EntityID)
		require.Equal(t, "cc-1", *auditRow.EntityID)
		require.NotNil(t, auditRow.ActorID)
		require.Equal(t, adminCtxUserID, *auditRow.ActorID)
		require.Equal(t, string(api.Admin), auditRow.ActorRole)

		// Full-struct diff so a regression in mapping (e.g. swapping fields,
		// forgetting to copy CampaignID into newCC) is caught here. Dynamic
		// timestamps (UpdatedAt) get a WithinDuration tolerance — see
		// backend-testing-unit.md § Assertions.
		var oldSnap domain.CampaignCreator
		require.NoError(t, json.Unmarshal(auditRow.OldValue, &oldSnap))
		require.WithinDuration(t, created, oldSnap.UpdatedAt, time.Minute)
		oldSnap.UpdatedAt = created
		require.Equal(t, domain.CampaignCreator{
			ID:           "cc-1",
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusSigned,
			TicketSentAt: nil,
			CreatedAt:    created,
			UpdatedAt:    created,
		}, oldSnap)

		var newSnap domain.CampaignCreator
		require.NoError(t, json.Unmarshal(auditRow.NewValue, &newSnap))
		require.WithinDuration(t, now, newSnap.UpdatedAt, time.Minute)
		newSnap.UpdatedAt = now
		require.NotNil(t, newSnap.TicketSentAt)
		require.Equal(t, now, *newSnap.TicketSentAt)
		require.Equal(t, domain.CampaignCreator{
			ID:           "cc-1",
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusSigned,
			TicketSentAt: &now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}, newSnap)
	})

	t.Run("unset flips timestamp → nil with audit", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepoPool := repomocks.NewMockCampaignCreatorRepo(t)
		ccRepoTx := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		log := logmocks.NewMockLogger(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
		ticketSentAt := created.Add(-time.Hour)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepoPool)
		ccRepoPool.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(signedRow("cc-1", &ticketSentAt, created), nil)

		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepoTx)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)

		ccRepoTx.EXPECT().UpdateTicketSentAt(mock.Anything, "cc-1", (*time.Time)(nil)).
			Return(signedRow("cc-1", nil, created), nil)

		var auditRow repository.AuditLogRow
		audit.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				auditRow = row
			}).
			Return(nil)

		log.EXPECT().Info(mock.Anything, "campaign creator ticket_sent toggled",
			[]any{"campaign_id", "camp-1", "creator_id", "cr-1", "ticket_sent", false}).Once()

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), log)
		got, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(false)})
		require.NoError(t, err)
		require.Nil(t, got.TicketSentAt)

		require.Equal(t, AuditActionCampaignCreatorTicketSent, auditRow.Action)
		var oldSnap domain.CampaignCreator
		require.NoError(t, json.Unmarshal(auditRow.OldValue, &oldSnap))
		require.NotNil(t, oldSnap.TicketSentAt)
		require.Equal(t, ticketSentAt, *oldSnap.TicketSentAt)
		oldSnap.TicketSentAt = &ticketSentAt
		require.Equal(t, domain.CampaignCreator{
			ID:           "cc-1",
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusSigned,
			TicketSentAt: &ticketSentAt,
			CreatedAt:    created,
			UpdatedAt:    created,
		}, oldSnap)

		var newSnap domain.CampaignCreator
		require.NoError(t, json.Unmarshal(auditRow.NewValue, &newSnap))
		require.Nil(t, newSnap.TicketSentAt)
		require.Equal(t, domain.CampaignCreator{
			ID:           "cc-1",
			CampaignID:   "camp-1",
			CreatorID:    "cr-1",
			Status:       domain.CampaignCreatorStatusSigned,
			TicketSentAt: nil,
			CreatedAt:    created,
			UpdatedAt:    created,
		}, newSnap)
	})

	t.Run("repo UpdateTicketSentAt error propagates wrapped", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepoPool := repomocks.NewMockCampaignCreatorRepo(t)
		ccRepoTx := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepoPool)
		ccRepoPool.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(signedRow("cc-1", nil, created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepoTx)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepoTx.EXPECT().UpdateTicketSentAt(mock.Anything, "cc-1", mock.Anything).
			Return((*repository.CampaignCreatorRow)(nil), errors.New("db down"))

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorContains(t, err, "db down")
	})

	t.Run("audit Create error rolls back the tx", func(t *testing.T) {
		t.Parallel()
		pool := dbmocks.NewMockPool(t)
		factory := svcmocks.NewMockCampaignCreatorRepoFactory(t)
		campaigns := repomocks.NewMockCampaignRepo(t)
		ccRepoPool := repomocks.NewMockCampaignCreatorRepo(t)
		ccRepoTx := repomocks.NewMockCampaignCreatorRepo(t)
		audit := repomocks.NewMockAuditRepo(t)
		created := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
		now := time.Date(2026, 5, 11, 13, 0, 0, 0, time.UTC)

		factory.EXPECT().NewCampaignRepo(pool).Return(campaigns)
		campaigns.EXPECT().GetByID(mock.Anything, "camp-1").
			Return(liveCampaignRow("camp-1", created), nil)
		factory.EXPECT().NewCampaignCreatorRepo(pool).Return(ccRepoPool)
		ccRepoPool.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(signedRow("cc-1", nil, created), nil)
		pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
		factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepoTx)
		factory.EXPECT().NewAuditRepo(mock.Anything).Return(audit)
		ccRepoTx.EXPECT().UpdateTicketSentAt(mock.Anything, "cc-1", mock.Anything).
			Return(signedRow("cc-1", &now, now), nil)
		audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit failed"))

		svc := NewCampaignCreatorService(pool, factory, svcmocks.NewMockCampaignInviteNotifier(t), logmocks.NewMockLogger(t))
		_, err := svc.PatchParticipation(adminCtx(), "camp-1", "cr-1", domain.PatchCampaignCreatorInput{TicketSent: pointer.ToBool(true)})
		require.ErrorContains(t, err, "audit failed")
	})
}
