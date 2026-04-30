package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

type telegramRig struct {
	pool        *dbmocks.MockPool
	factory     *svcmocks.MockCreatorApplicationTelegramRepoFactory
	appRepo     *repomocks.MockCreatorApplicationRepo
	linkRepo    *repomocks.MockCreatorApplicationTelegramLinkRepo
	auditRepo   *repomocks.MockAuditRepo
	logger      *logmocks.MockLogger
}

func newTelegramRig(t *testing.T) telegramRig {
	t.Helper()
	return telegramRig{
		pool:      dbmocks.NewMockPool(t),
		factory:   svcmocks.NewMockCreatorApplicationTelegramRepoFactory(t),
		appRepo:   repomocks.NewMockCreatorApplicationRepo(t),
		linkRepo:  repomocks.NewMockCreatorApplicationTelegramLinkRepo(t),
		auditRepo: repomocks.NewMockAuditRepo(t),
		logger:    logmocks.NewMockLogger(t),
	}
}

func expectTelegramTxBegin(rig telegramRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

func expectTelegramFactoryWiring(rig telegramRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.linkRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

func TestCreatorApplicationTelegramService_LinkTelegram(t *testing.T) {
	t.Parallel()

	const appID = "11111111-2222-3333-4444-555555555555"
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	baseInput := domain.TelegramLinkInput{
		ApplicationID: appID,
		TgUserID:      int64(123),
		TgUsername:    pointer.ToString("aidana"),
		TgFirstName:   pointer.ToString("Aidana"),
		TgLastName:    pointer.ToString("M."),
	}

	t.Run("application not found surfaces domain.ErrNotFound", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("existing link to same Telegram is idempotent and silent", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		linkedAt := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(123), LinkedAt: linkedAt,
			}, nil)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.Equal(t, &domain.TelegramLinkResult{
			ApplicationID:  appID,
			Status:         domain.CreatorApplicationStatusPending,
			TelegramUserID: int64(123),
			LinkedAt:       linkedAt,
			Idempotent:     true,
		}, got)
	})

	t.Run("existing link to different Telegram returns ApplicationAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{ApplicationID: appID, TelegramUserID: int64(999)}, nil)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("insert success writes audit and logs identifiers", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Run(func(ctx context.Context, row repository.CreatorApplicationTelegramLinkRow) {
				require.Equal(t, appID, row.ApplicationID)
				require.Equal(t, int64(123), row.TelegramUserID)
				require.Equal(t, pointer.ToString("aidana"), row.TelegramUsername)
				require.Equal(t, now, row.LinkedAt)
				require.Equal(t, "telegram-bot", middleware.ClientIPFromContext(ctx))
			}).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  appID,
				TelegramUserID: int64(123),
				LinkedAt:       now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(func(_ context.Context, row repository.AuditLogRow) {
				require.Equal(t, AuditActionCreatorApplicationLinkTelegram, row.Action)
				require.Equal(t, AuditEntityTypeCreatorApplication, row.EntityType)
				require.Equal(t, pointer.ToString(appID), row.EntityID)
				require.Equal(t, "telegram-bot", row.IPAddress)
				require.NotEmpty(t, row.NewValue)
			}).
			Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "telegram linked to creator application",
			[]any{"application_id", appID, "telegram_user_id", int64(123)}).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.False(t, got.Idempotent)
		require.Equal(t, appID, got.ApplicationID)
		require.Equal(t, int64(123), got.TelegramUserID)
	})

	t.Run("PK race: re-read finds same TG then idempotent", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		linkedAt := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(nil, sql.ErrNoRows).Once()
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(123), LinkedAt: linkedAt,
			}, nil).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.True(t, got.Idempotent)
		require.Equal(t, linkedAt, got.LinkedAt)
	})

	t.Run("PK race: re-read finds different TG then ApplicationAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(nil, sql.ErrNoRows).Once()
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{ApplicationID: appID, TelegramUserID: int64(999)}, nil).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("application repo error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "get application")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("preflight link lookup error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, errors.New("link boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "preflight telegram link")
		require.ErrorContains(t, err, "link boom")
	})

	t.Run("insert error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil, errors.New("insert boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "insert telegram link")
		require.ErrorContains(t, err, "insert boom")
	})

	t.Run("audit error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(123), LinkedAt: now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "write audit")
		require.ErrorContains(t, err, "audit boom")
	})

	t.Run("trims and caps username/first/last", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)

		over := make([]rune, domain.TelegramNameMaxLen+50)
		for i := range over {
			over[i] = 'я'
		}
		input := domain.TelegramLinkInput{
			ApplicationID: appID,
			TgUserID:      int64(7),
			TgUsername:    pointer.ToString("   "), // whitespace-only → nil
			TgFirstName:   pointer.ToString(string(over)),
			TgLastName:    pointer.ToString("Surname"),
		}

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).
			Return(&repository.CreatorApplicationRow{ID: appID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Run(func(_ context.Context, row repository.CreatorApplicationTelegramLinkRow) {
				require.Nil(t, row.TelegramUsername, "whitespace-only username must collapse to nil")
				require.NotNil(t, row.TelegramFirstName)
				require.Equal(t, domain.TelegramNameMaxLen, len([]rune(*row.TelegramFirstName)))
				require.Equal(t, pointer.ToString("Surname"), row.TelegramLastName)
			}).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(7), LinkedAt: now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), input, now)
		require.NoError(t, err)
	})
}
