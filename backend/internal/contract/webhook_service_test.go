package contract_test

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/contract"
	contractmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/contract/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
)

// webhookRig — все mockery-моки + factory-обвязка для одного сценария.
type webhookRig struct {
	pool      pgxmock.PgxPoolIface
	contracts *repomocks.MockContractRepo
	cc        *repomocks.MockCampaignCreatorRepo
	audit     *repomocks.MockAuditRepo
	factory   *contractmocks.MockWebhookRepoFactory
	notifier  *contractmocks.MockWebhookNotifier
	logger    *logmocks.MockLogger
}

func newWebhookRig(t *testing.T) *webhookRig {
	t.Helper()
	rig := &webhookRig{
		pool:      newPgxmockPool(t),
		contracts: repomocks.NewMockContractRepo(t),
		cc:        repomocks.NewMockCampaignCreatorRepo(t),
		audit:     repomocks.NewMockAuditRepo(t),
		factory:   contractmocks.NewMockWebhookRepoFactory(t),
		notifier:  contractmocks.NewMockWebhookNotifier(t),
		logger:    logmocks.NewMockLogger(t),
	}
	rig.factory.EXPECT().NewContractsRepo(mock.Anything).Return(rig.contracts).Maybe()
	rig.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(rig.cc).Maybe()
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.audit).Maybe()
	rig.logger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Maybe()
	rig.logger.EXPECT().Warn(mock.Anything, mock.Anything, mock.Anything).Maybe()
	rig.logger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything).Maybe()
	return rig
}

func (r *webhookRig) build(t *testing.T) *contract.WebhookService {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) }
	return contract.NewWebhookService(r.pool, r.factory, r.notifier, r.logger, now)
}

// expectWebhookAudit задаёт expect на audit.Create + JSONEq на NewValue.
func expectWebhookAudit(t *testing.T, rig *webhookRig, action, ccID, expectedPayloadJSON string) {
	t.Helper()
	rig.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, row repository.AuditLogRow) {
			require.Equal(t, action, row.Action)
			require.Nil(t, row.ActorID)
			require.Equal(t, "system", row.ActorRole)
			require.Equal(t, "campaign_creator", row.EntityType)
			require.NotNil(t, row.EntityID)
			require.Equal(t, ccID, *row.EntityID)
			require.JSONEq(t, expectedPayloadJSON, string(row.NewValue))
		}).Return(nil)
}

func contractRow(id string, statusCode int) *repository.ContractRow {
	docID := "doc-" + id
	return &repository.ContractRow{
		ID:                id,
		SubjectKind:       repository.ContractSubjectKindCampaignCreator,
		TrustMeDocumentID: &docID,
		TrustMeStatusCode: statusCode,
	}
}

func ccView(ccID string, isDeleted bool, tgID int64) *repository.CampaignCreatorWebhookView {
	return &repository.CampaignCreatorWebhookView{
		CampaignCreatorID:     ccID,
		CampaignCreatorStatus: domain.CampaignCreatorStatusSigning,
		CampaignIsDeleted:     isDeleted,
		CreatorTelegramUserID: tgID,
	}
}

func TestWebhookService_HandleEvent_Signed(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	row := contractRow("ct-1", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-1").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-1", 3).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-1").Return(ccView("cc-1", false, 5001), nil)
	rig.cc.EXPECT().UpdateStatus(mock.Anything, "cc-1", domain.CampaignCreatorStatusSigned).Return(nil)
	expectWebhookAudit(t, rig, "campaign_creator.contract_signed", "cc-1",
		`{"contract_id":"ct-1","trustme_status_code_old":0,"trustme_status_code_new":3}`)
	rig.notifier.EXPECT().NotifyCampaignContractSigned(mock.Anything, int64(5001))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-1", 3)
	require.NoError(t, err)
	require.NoError(t, svc.HandleEvent(context.Background(), ev))
}

func TestWebhookService_HandleEvent_Declined(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	row := contractRow("ct-2", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-2").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-2", 9).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-2").Return(ccView("cc-2", false, 5002), nil)
	rig.cc.EXPECT().UpdateStatus(mock.Anything, "cc-2", domain.CampaignCreatorStatusSigningDeclined).Return(nil)
	expectWebhookAudit(t, rig, "campaign_creator.contract_signing_declined", "cc-2",
		`{"contract_id":"ct-2","trustme_status_code_old":0,"trustme_status_code_new":9}`)
	rig.notifier.EXPECT().NotifyCampaignContractDeclined(mock.Anything, int64(5002))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-2", 9)
	require.NoError(t, err)
	require.NoError(t, svc.HandleEvent(context.Background(), ev))
}

func TestWebhookService_HandleEvent_IntermediateStatuses(t *testing.T) {
	t.Parallel()

	statuses := []int{0, 1, 2, 4, 5, 6, 7, 8}
	for _, status := range statuses {
		t.Run("status="+strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			rig := newWebhookRig(t)
			rig.pool.ExpectBegin()
			rig.pool.ExpectCommit()

			row := contractRow("ct-int", 0)
			rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-int").Return(row, nil)
			rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-int", status).Return(1, nil)
			rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-int").Return(ccView("cc-int", false, 9999), nil)
			expectWebhookAudit(t, rig, "campaign_creator.contract_unexpected_status", "cc-int",
				`{"contract_id":"ct-int","trustme_status_code_old":0,"trustme_status_code_new":`+strconv.Itoa(status)+`}`)
			// notifier НЕ зовётся для intermediate.

			svc := rig.build(t)
			ev, err := domain.NewTrustMeWebhookEvent("doc-ct-int", status)
			require.NoError(t, err)
			require.NoError(t, svc.HandleEvent(context.Background(), ev))
		})
	}
}

func TestWebhookService_HandleEvent_IdempotentRepeat(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	row := contractRow("ct-3", 3) // уже signed
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-3").Return(row, nil)
	// UPDATE 0 affected (idempotent — тот же status уже).
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-3", 3).Return(0, nil)
	// cc lookup НЕ зовётся, audit НЕ пишется, notify НЕ отправляется.

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-3", 3)
	require.NoError(t, err)
	require.NoError(t, svc.HandleEvent(context.Background(), ev))
}

func TestWebhookService_HandleEvent_TerminalGuard(t *testing.T) {
	t.Parallel()

	// БД уже terminal (signed=3), прилетает stale status=2 → 0 affected,
	// info-лог `stale_webhook_after_terminal`, no audit, no notify.
	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	row := contractRow("ct-4", 3)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-4").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-4", 2).Return(0, nil)

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-4", 2)
	require.NoError(t, err)
	require.NoError(t, svc.HandleEvent(context.Background(), ev))
}

func TestWebhookService_HandleEvent_SoftDeletedCampaign(t *testing.T) {
	t.Parallel()

	// soft-deleted campaign + status=3 → state-transition + audit пишутся
	// (factual record), но notify пропускается + warn-лог.
	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	row := contractRow("ct-5", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-5").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-5", 3).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-5").Return(ccView("cc-5", true, 7777), nil)
	rig.cc.EXPECT().UpdateStatus(mock.Anything, "cc-5", domain.CampaignCreatorStatusSigned).Return(nil)
	expectWebhookAudit(t, rig, "campaign_creator.contract_signed", "cc-5",
		`{"contract_id":"ct-5","trustme_status_code_old":0,"trustme_status_code_new":3}`)
	// notifier НЕ зовётся.

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-5", 3)
	require.NoError(t, err)
	require.NoError(t, svc.HandleEvent(context.Background(), ev))
}

func TestWebhookService_HandleEvent_UnknownDocument(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-missing").Return(nil, sql.ErrNoRows)

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-missing", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorIs(t, err, domain.ErrContractWebhookUnknownDocument)
}

func TestWebhookService_HandleEvent_UnknownSubjectKind(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	row := &repository.ContractRow{
		ID:                "ct-6",
		SubjectKind:       "brand_agreement", // future kind, not yet wired
		TrustMeStatusCode: 0,
	}
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-6").Return(row, nil)

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-6", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorIs(t, err, domain.ErrContractWebhookUnknownSubject)
}

func TestWebhookService_HandleEvent_CampaignCreatorMissing(t *testing.T) {
	t.Parallel()

	// Defensive — FK contracts ↔ campaign_creators существует, но JOIN view
	// вернул 0 рядов. Возвращаем UnknownSubject + warn-лог.
	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	row := contractRow("ct-7", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-7").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-7", 3).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-7").Return(nil, sql.ErrNoRows)

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-7", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorIs(t, err, domain.ErrContractWebhookUnknownSubject)
}

func TestWebhookService_HandleEvent_LockError(t *testing.T) {
	t.Parallel()

	// Любая не-ErrNoRows ошибка из repo пробрасывается с обёрткой.
	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-x").Return(nil, errors.New("db down"))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-x", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.Error(t, err)
	require.NotErrorIs(t, err, domain.ErrContractWebhookUnknownDocument)
	require.Contains(t, err.Error(), "lock contract")
}

func TestWebhookService_HandleEvent_UpdateError(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	row := contractRow("ct-8", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-8").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-8", 3).Return(0, errors.New("db down"))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-8", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorContains(t, err, "update contract")
}

func TestWebhookService_HandleEvent_CCUpdateStatusError(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	row := contractRow("ct-9", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-9").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-9", 9).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-9").Return(ccView("cc-9", false, 11), nil)
	rig.cc.EXPECT().UpdateStatus(mock.Anything, "cc-9", domain.CampaignCreatorStatusSigningDeclined).Return(errors.New("db down"))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-9", 9)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorContains(t, err, "update cc status")
}

func TestWebhookService_HandleEvent_AuditError(t *testing.T) {
	t.Parallel()

	rig := newWebhookRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectRollback()

	row := contractRow("ct-10", 0)
	rig.contracts.EXPECT().LockByTrustMeDocumentID(mock.Anything, "doc-ct-10").Return(row, nil)
	rig.contracts.EXPECT().UpdateAfterWebhook(mock.Anything, "ct-10", 3).Return(1, nil)
	rig.cc.EXPECT().GetWithCampaignAndCreatorByContractID(mock.Anything, "ct-10").Return(ccView("cc-10", false, 12), nil)
	rig.cc.EXPECT().UpdateStatus(mock.Anything, "cc-10", domain.CampaignCreatorStatusSigned).Return(nil)
	rig.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit fail"))

	svc := rig.build(t)
	ev, err := domain.NewTrustMeWebhookEvent("doc-ct-10", 3)
	require.NoError(t, err)
	err = svc.HandleEvent(context.Background(), ev)
	require.ErrorContains(t, err, "audit")
}
