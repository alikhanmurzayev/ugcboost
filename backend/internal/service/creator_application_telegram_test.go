package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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

// captureRunFunc is a typed Run callback compatible with mockery v3-generated
// Insert(ctx, row) expectations. The mocks expose a typed Run signature so the
// test cannot pass a generic mock.Arguments closure.
type captureLinkInsert func(ctx context.Context, row repository.CreatorApplicationTelegramLinkRow)
type captureAuditCreate func(ctx context.Context, entry repository.AuditLogRow)

// telegramServiceRig assembles every mock the LinkTelegram test cases need.
// One rig per t.Run keeps each scenario isolated.
type telegramServiceRig struct {
	pool       *dbmocks.MockPool
	factory    *svcmocks.MockCreatorApplicationTelegramRepoFactory
	appRepo    *repomocks.MockCreatorApplicationRepo
	tgLinkRepo *repomocks.MockCreatorApplicationTelegramLinkRepo
	auditRepo  *repomocks.MockAuditRepo
	logger     *logmocks.MockLogger
}

func newTelegramServiceRig(t *testing.T) telegramServiceRig {
	t.Helper()
	return telegramServiceRig{
		pool:       dbmocks.NewMockPool(t),
		factory:    svcmocks.NewMockCreatorApplicationTelegramRepoFactory(t),
		appRepo:    repomocks.NewMockCreatorApplicationRepo(t),
		tgLinkRepo: repomocks.NewMockCreatorApplicationTelegramLinkRepo(t),
		auditRepo:  repomocks.NewMockAuditRepo(t),
		logger:     logmocks.NewMockLogger(t),
	}
}

// expectTelegramTxBegin opens a fake transaction so dbutil.WithTx can run the
// callback. Mock repos intercept all queries before they reach testTx{}.
func expectTelegramTxBegin(rig telegramServiceRig) {
	rig.pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
}

// expectTelegramFactoryWiring covers the three NewXRepo calls the service
// makes at the top of the transaction. They run unconditionally so each
// scenario expects them once.
func expectTelegramFactoryWiring(rig telegramServiceRig) {
	rig.factory.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(rig.appRepo)
	rig.factory.EXPECT().NewCreatorApplicationTelegramLinkRepo(mock.Anything).Return(rig.tgLinkRepo)
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.auditRepo)
}

func validTelegramInput() domain.TelegramLinkInput {
	return domain.TelegramLinkInput{
		ApplicationID: "11111111-2222-3333-4444-555555555555",
		TgUserID:      7000123,
		TgUsername:    pointer.ToString("test_42"),
		TgFirstName:   pointer.ToString("Айдана"),
		TgLastName:    pointer.ToString("Муратова"),
	}
}

func TestCreatorApplicationTelegramService_LinkTelegram(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 29, 22, 0, 0, 0, time.UTC)

	t.Run("application not found surfaces ErrNotFound", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("application GetByID generic error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(nil, errors.New("db down"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorContains(t, err, "get application")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("rejected application returns CodeTelegramApplicationNotActive", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusRejected}, nil)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationNotActive, be.Code)
	})

	t.Run("idempotent: same Telegram user, link exists, no audit row", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()
		linkedAt := time.Date(2026, 4, 29, 21, 0, 0, 0, time.UTC)

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  in.ApplicationID,
				TelegramUserID: in.TgUserID,
				LinkedAt:       linkedAt,
			}, nil)

		// Idempotent path must NOT log success — the audit log is the source
		// of truth and we did not touch it.
		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), in, now)
		require.NoError(t, err)
		require.True(t, got.Idempotent)
		require.Equal(t, in.TgUserID, got.TelegramUserID)
		require.Equal(t, linkedAt, got.LinkedAt)
	})

	t.Run("conflict: link exists for different Telegram user", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  in.ApplicationID,
				TelegramUserID: 9999999, // another Telegram user
			}, nil)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("preflight GetByApplicationID generic error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, errors.New("link read boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorContains(t, err, "preflight telegram link")
		require.ErrorContains(t, err, "link read boom")
	})

	t.Run("happy path: link inserted, audit row written, after-tx log fires", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)

		var capturedRow repository.CreatorApplicationTelegramLinkRow
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Run(captureLinkInsert(func(_ context.Context, row repository.CreatorApplicationTelegramLinkRow) {
				capturedRow = row
			})).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:     in.ApplicationID,
				TelegramUserID:    in.TgUserID,
				TelegramUsername:  in.TgUsername,
				TelegramFirstName: in.TgFirstName,
				TelegramLastName:  in.TgLastName,
				LinkedAt:          now,
			}, nil)

		var capturedAudit repository.AuditLogRow
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(captureAuditCreate(func(_ context.Context, entry repository.AuditLogRow) {
				capturedAudit = entry
			})).
			Return(nil)

		// after-tx Info logs identifiers only; Telegram metadata never leaves
		// the DB / audit_logs.
		rig.logger.EXPECT().Info(
			mock.Anything,
			"telegram linked to creator application",
			[]any{"application_id", in.ApplicationID, "telegram_user_id", in.TgUserID},
		).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), in, now)
		require.NoError(t, err)
		require.False(t, got.Idempotent)
		require.Equal(t, in.ApplicationID, got.ApplicationID)
		require.Equal(t, in.TgUserID, got.TelegramUserID)
		require.Equal(t, now, got.LinkedAt)
		require.Equal(t, in.TgUsername, capturedRow.TelegramUsername)
		require.Equal(t, in.TgFirstName, capturedRow.TelegramFirstName)
		require.Equal(t, in.TgLastName, capturedRow.TelegramLastName)
		require.Equal(t, AuditActionCreatorApplicationLinkTelegram, capturedAudit.Action)
		require.Equal(t, AuditEntityTypeCreatorApplication, capturedAudit.EntityType)
		require.NotNil(t, capturedAudit.EntityID)
		require.Equal(t, in.ApplicationID, *capturedAudit.EntityID)
		require.JSONEq(t,
			`{"telegram_user_id":7000123,"telegram_username":"test_42","telegram_first_name":"Айдана","telegram_last_name":"Муратова"}`,
			string(capturedAudit.NewValue))
	})

	t.Run("trim and cap: oversized strings clipped, blank → nil, audit reflects sanitised values", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		// Service trims " hello " to "hello" and " " to nil. Username gets
		// capped to TelegramUsernameMaxLen runes.
		in := domain.TelegramLinkInput{
			ApplicationID: "app-trim",
			TgUserID:      7000124,
			TgUsername:    pointer.ToString(strings.Repeat("u", domain.TelegramUsernameMaxLen+5)),
			TgFirstName:   pointer.ToString("   "),
			TgLastName:    pointer.ToString(" Муратова "),
		}

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)

		var captured repository.CreatorApplicationTelegramLinkRow
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Run(captureLinkInsert(func(_ context.Context, row repository.CreatorApplicationTelegramLinkRow) {
				captured = row
			})).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:    in.ApplicationID,
				TelegramUserID:   in.TgUserID,
				TelegramUsername: pointer.ToString(strings.Repeat("u", domain.TelegramUsernameMaxLen)),
				TelegramLastName: pointer.ToString("Муратова"),
				LinkedAt:         now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Return(nil)
		rig.logger.EXPECT().Info(mock.Anything, "telegram linked to creator application",
			[]any{"application_id", in.ApplicationID, "telegram_user_id", in.TgUserID},
		).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.NoError(t, err)
		require.NotNil(t, captured.TelegramUsername)
		require.Equal(t, domain.TelegramUsernameMaxLen, len([]rune(*captured.TelegramUsername)))
		require.Nil(t, captured.TelegramFirstName) // whitespace-only → nil
		require.NotNil(t, captured.TelegramLastName)
		require.Equal(t, "Муратова", *captured.TelegramLastName)
	})

	t.Run("UNIQUE on telegram_user_id maps to CodeTelegramAccountAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(nil, domain.ErrTelegramAccountLinkConflict)

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramAccountAlreadyLinked, be.Code)
	})

	t.Run("PK race + same TG: re-read finds existing row → idempotent", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()
		raceLinked := time.Date(2026, 4, 29, 21, 30, 0, 0, time.UTC)

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		// Preflight: not found.
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows).Once()
		// Insert race: PK conflict.
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)
		// Re-read finds the row, same TG user → idempotent success.
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  in.ApplicationID,
				TelegramUserID: in.TgUserID,
				LinkedAt:       raceLinked,
			}, nil).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		got, err := svc.LinkTelegram(context.Background(), in, now)
		require.NoError(t, err)
		require.True(t, got.Idempotent)
		require.Equal(t, raceLinked, got.LinkedAt)
	})

	t.Run("PK race + different TG: re-read returns CodeTelegramApplicationAlreadyLinked", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows).Once()
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  in.ApplicationID,
				TelegramUserID: 9999999,
			}, nil).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		var be *domain.BusinessError
		require.ErrorAs(t, err, &be)
		require.Equal(t, domain.CodeTelegramApplicationAlreadyLinked, be.Code)
	})

	t.Run("PK race + re-read fails: error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows).Once()
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(nil, domain.ErrTelegramApplicationLinkConflict)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, errors.New("re-read boom")).Once()

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorContains(t, err, "re-read telegram link after PK race")
		require.ErrorContains(t, err, "re-read boom")
	})

	t.Run("Insert generic error wrapped", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(nil, errors.New("insert boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorContains(t, err, "insert telegram link")
		require.ErrorContains(t, err, "insert boom")
	})

	t.Run("audit Create error wrapped — link write rolled back too via tx", func(t *testing.T) {
		t.Parallel()
		rig := newTelegramServiceRig(t)
		expectTelegramTxBegin(rig)
		expectTelegramFactoryWiring(rig)
		in := validTelegramInput()

		rig.appRepo.EXPECT().GetByID(mock.Anything, in.ApplicationID).
			Return(&repository.CreatorApplicationRow{ID: in.ApplicationID, Status: domain.CreatorApplicationStatusPending}, nil)
		rig.tgLinkRepo.EXPECT().GetByApplicationID(mock.Anything, in.ApplicationID).
			Return(nil, sql.ErrNoRows)
		rig.tgLinkRepo.EXPECT().Insert(mock.Anything, mock.AnythingOfType("repository.CreatorApplicationTelegramLinkRow")).
			Return(&repository.CreatorApplicationTelegramLinkRow{
				ApplicationID:  in.ApplicationID,
				TelegramUserID: in.TgUserID,
				LinkedAt:       now,
			}, nil)
		rig.auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Return(errors.New("audit boom"))

		svc := NewCreatorApplicationTelegramService(rig.pool, rig.factory, rig.logger)
		_, err := svc.LinkTelegram(context.Background(), in, now)
		require.ErrorContains(t, err, "write audit")
		require.ErrorContains(t, err, "audit boom")
	})
}
