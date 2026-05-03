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
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

type telegramRig struct {
	pool       *dbmocks.MockPool
	factory    *svcmocks.MockCreatorApplicationTelegramRepoFactory
	appRepo    *repomocks.MockCreatorApplicationRepo
	socialRepo *repomocks.MockCreatorApplicationSocialRepo
	linkRepo   *repomocks.MockCreatorApplicationTelegramLinkRepo
	auditRepo  *repomocks.MockAuditRepo
	notifier   *svcmocks.MockcreatorAppTelegramNotifier
	logger     *logmocks.MockLogger
}

func newTelegramRig(t *testing.T) telegramRig {
	t.Helper()
	return telegramRig{
		pool:       dbmocks.NewMockPool(t),
		factory:    svcmocks.NewMockCreatorApplicationTelegramRepoFactory(t),
		appRepo:    repomocks.NewMockCreatorApplicationRepo(t),
		socialRepo: repomocks.NewMockCreatorApplicationSocialRepo(t),
		linkRepo:   repomocks.NewMockCreatorApplicationTelegramLinkRepo(t),
		auditRepo:  repomocks.NewMockAuditRepo(t),
		notifier:   svcmocks.NewMockcreatorAppTelegramNotifier(t),
		logger:     logmocks.NewMockLogger(t),
	}
}

func expectTelegramTxBegin(rig telegramRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectTelegramFactoryWiringHead wires the repos every TX touches before any
// branching (app, social). The link/audit repos are wired by the helper that
// follows because failure paths short-circuit before reaching them.
func expectTelegramFactoryWiringHead(rig telegramRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.linkRepo)
	rig.factory.EXPECT().NewCreatorApplicationSocialRepo(mock.Anything).Return(rig.socialRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

// igSocials returns a one-element social slice that contains an Instagram row.
// The link service only inspects the platform field, so the rest is omitted.
func igSocials(appID string) []*repository.CreatorApplicationSocialRow {
	return []*repository.CreatorApplicationSocialRow{
		{ID: "social-ig", ApplicationID: appID, Platform: domain.SocialPlatformInstagram, Handle: "aidana"},
	}
}

// noIGSocials returns a slice without an IG entry — TikTok-only.
func noIGSocials(appID string) []*repository.CreatorApplicationSocialRow {
	return []*repository.CreatorApplicationSocialRow{
		{ID: "social-tt", ApplicationID: appID, Platform: domain.SocialPlatformTikTok, Handle: "aidana_tt"},
	}
}

func TestCreatorApplicationTelegramService_LinkTelegram(t *testing.T) {
	t.Parallel()

	const (
		appID = "11111111-2222-3333-4444-555555555555"
		code  = "UGC-123456"
	)
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	baseInput := domain.TelegramLinkInput{
		ApplicationID:     appID,
		TelegramUserID:    int64(123),
		TelegramUsername:  pointer.ToString("aidana"),
		TelegramFirstName: pointer.ToString("Aidana"),
		TelegramLastName:  pointer.ToString("M."),
	}

	// appRowWithCode returns a fresh row preserving the verification code so
	// service-level capture matches what the welcome notify needs.
	appRowWithCode := func() *repository.CreatorApplicationRow {
		return &repository.CreatorApplicationRow{
			ID:               appID,
			Status:           domain.CreatorApplicationStatusVerification,
			VerificationCode: code,
		}
	}

	// Подтесты упорядочены по `docs/standards/backend-testing-unit.md` § Нейминг —
	// сначала ранние выходы (ошибки), затем нестандартный success-branch
	// (idempotent), потом основные happy paths и edge happy.

	t.Run("application not found surfaces domain.ErrNotFound; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("application repo error wrapped; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "get application")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("social list error wrapped; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(nil, errors.New("social boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "list socials")
		require.ErrorContains(t, err, "social boom")
	})

	t.Run("preflight error wrapped; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, errors.New("preflight boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "preflight telegram link")
		require.ErrorContains(t, err, "preflight boom")
	})

	t.Run("preflight finds different TG → ApplicationAlreadyLinked; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{ApplicationID: appID, TelegramUserID: int64(999)}, nil)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("insert error wrapped; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil, errors.New("insert boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "insert telegram link")
		require.ErrorContains(t, err, "insert boom")
	})

	t.Run("PK race after preflight (rare) → ApplicationAlreadyLinked; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)

		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("FK violation on insert (parent gone) surfaces domain.ErrNotFound; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(nil, domain.ErrNotFound)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("audit error wrapped; no notify", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(123), LinkedAt: now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.ErrorContains(t, err, "write audit")
		require.ErrorContains(t, err, "audit boom")
	})

	t.Run("preflight finds same TG → idempotent: no audit, debug-log, with-IG welcome fires", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)
		linkedAt := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID: appID, TelegramUserID: int64(123), LinkedAt: linkedAt,
			}, nil)
		rig.logger.EXPECT().Debug(mock.Anything, "telegram link idempotent",
			[]any{"application_id", appID, "telegram_user_id", int64(123)}).Once()

		var capturedPayload telegram.ApplicationLinkedPayload
		rig.notifier.EXPECT().NotifyApplicationLinked(mock.Anything, int64(123), mock.AnythingOfType("telegram.ApplicationLinkedPayload")).
			Run(func(_ context.Context, _ int64, p telegram.ApplicationLinkedPayload) {
				capturedPayload = p
			}).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.Equal(t, code, capturedPayload.VerificationCode)
		require.True(t, capturedPayload.HasInstagram)
	})

	t.Run("insert success writes audit, logs identifiers, fires with-IG welcome", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
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

		var capturedPayload telegram.ApplicationLinkedPayload
		rig.notifier.EXPECT().NotifyApplicationLinked(mock.Anything, int64(123), mock.AnythingOfType("telegram.ApplicationLinkedPayload")).
			Run(func(_ context.Context, _ int64, p telegram.ApplicationLinkedPayload) {
				capturedPayload = p
			}).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.Equal(t, code, capturedPayload.VerificationCode)
		require.True(t, capturedPayload.HasInstagram)
	})

	t.Run("insert success on no-IG application fires no-IG welcome", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(noIGSocials(appID), nil)
		rig.linkRepo.EXPECT().GetByApplicationID(mock.Anything, appID).Return(nil, sql.ErrNoRows)
		rig.linkRepo.EXPECT().Insert(mock.Anything, mock.Anything).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  appID,
				TelegramUserID: int64(123),
				LinkedAt:       now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "telegram linked to creator application", mock.Anything).Once()

		var capturedPayload telegram.ApplicationLinkedPayload
		rig.notifier.EXPECT().NotifyApplicationLinked(mock.Anything, int64(123), mock.AnythingOfType("telegram.ApplicationLinkedPayload")).
			Run(func(_ context.Context, _ int64, p telegram.ApplicationLinkedPayload) {
				capturedPayload = p
			}).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), baseInput, now)
		require.NoError(t, err)
		require.False(t, capturedPayload.HasInstagram)
		require.Equal(t, code, capturedPayload.VerificationCode, "code is always present even when no-IG variant ignores it")
	})

	t.Run("trims and caps username/first/last", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiringHead(rig)

		over := make([]rune, domain.TelegramNameMaxLen+50)
		for i := range over {
			over[i] = 'я'
		}
		input := domain.TelegramLinkInput{
			ApplicationID:     appID,
			TelegramUserID:    int64(7),
			TelegramUsername:  pointer.ToString("   "), // whitespace-only → nil
			TelegramFirstName: pointer.ToString(string(over)),
			TelegramLastName:  pointer.ToString("Surname"),
		}

		rig.appRepo.EXPECT().GetByID(mock.Anything, appID).Return(appRowWithCode(), nil)
		rig.socialRepo.EXPECT().ListByApplicationID(mock.Anything, appID).Return(igSocials(appID), nil)
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
		rig.notifier.EXPECT().NotifyApplicationLinked(mock.Anything, int64(7), mock.Anything).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.notifier, rig.logger)
		err := svc.LinkTelegram(context.Background(), input, now)
		require.NoError(t, err)
	})
}
