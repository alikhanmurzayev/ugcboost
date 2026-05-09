package contract_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/contract"
	contractmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/contract/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
	trustmemocks "github.com/alikhanmurzayev/ugcboost/backend/internal/trustme/mocks"
)

// Batch sizes — приватные константы пакета contract зафиксированы здесь как
// числовые литералы (per intent Decision #10: claimBatchSize=4 синхронизирован
// с TrustMe rate-limit 4 RPS, recoveryBatchSize=8 шире ради экономии тиков).
// Если изменятся — тест упадёт, форсируя пересмотр (что и есть желаемое
// поведение для contract'а between worker'а и rate-limit).
const (
	claimBatchSize    = 4
	recoveryBatchSize = 8
)

// Audit constants — продублированы локально, потому что внутри contract
// пакета они приватные. Стандарт `naming.md` допускает два источника правды
// для test-time, главное — sync через тесты при изменении production кода.
const (
	auditEntityTypeCampaignCreator = "campaign_creator"
	auditActorRoleSystem           = "system"
)

func newPgxmockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	return mock
}

// senderRig — все mockery-моки + factory-обвязка для одного scenario'а.
// Каждый t.Run собирает свой rig — изоляция per backend-testing-unit.
type senderRig struct {
	pool      pgxmock.PgxPoolIface
	contracts *repomocks.MockContractRepo
	cc        *repomocks.MockCampaignCreatorRepo
	audit     *repomocks.MockAuditRepo
	factory   *contractmocks.MockContractSenderRepoFactory
	tm        *trustmemocks.MockClient
	renderer  *contractmocks.MockRenderer
	notifier  *contractmocks.MockCreatorNotifier
	resolver  *contractmocks.MockCreatorTelegramResolver
	logger    *logmocks.MockLogger
}

func newSenderRig(t *testing.T) *senderRig {
	t.Helper()
	pool := newPgxmockPool(t)
	rig := &senderRig{
		pool:      pool,
		contracts: repomocks.NewMockContractRepo(t),
		cc:        repomocks.NewMockCampaignCreatorRepo(t),
		audit:     repomocks.NewMockAuditRepo(t),
		factory:   contractmocks.NewMockContractSenderRepoFactory(t),
		tm:        trustmemocks.NewMockClient(t),
		renderer:  contractmocks.NewMockRenderer(t),
		notifier:  contractmocks.NewMockCreatorNotifier(t),
		resolver:  contractmocks.NewMockCreatorTelegramResolver(t),
		logger:    logmocks.NewMockLogger(t),
	}
	rig.factory.EXPECT().NewContractsRepo(mock.Anything).Return(rig.contracts).Maybe()
	rig.factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(rig.cc).Maybe()
	rig.factory.EXPECT().NewAuditRepo(mock.Anything).Return(rig.audit).Maybe()
	return rig
}

func (r *senderRig) build(t *testing.T) *contract.ContractSenderService {
	t.Helper()
	return contract.NewContractSenderService(r.pool, r.factory, r.tm,
		r.renderer, r.resolver, r.notifier, r.logger, time.Hour)
}

// expectRecordFailedAttempt задаёт mockery-expect для RecordFailedAttempt с
// проверкой что nextRetryAt = now() + retryBackoff (time.Hour в тестах) с
// допуском ±1 секунда. Точные args заменяют слабый mock.AnythingOfType.
func expectRecordFailedAttempt(t *testing.T, rig *senderRig, contractID, code, message string) {
	t.Helper()
	rig.contracts.EXPECT().RecordFailedAttempt(mock.Anything, contractID, code, message,
		mock.AnythingOfType("time.Time")).
		Run(func(_ context.Context, _, _, _ string, nextRetryAt time.Time) {
			expected := time.Now().UTC().Add(time.Hour)
			require.WithinDuration(t, expected, nextRetryAt, time.Second,
				"nextRetryAt must equal now()+retryBackoff (time.Hour ±1s)")
		}).Return(nil)
}

// expectAuditRow задаёт expect для audit.Create с .Run() capture, проверяющим
// каждое поле (Action, ActorID=nil, ActorRole, EntityType, EntityID) и
// JSONEq на NewValue payload.
func expectAuditRow(t *testing.T, rig *senderRig, action, ccID, expectedPayloadJSON string) {
	t.Helper()
	rig.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, row repository.AuditLogRow) {
			require.Equal(t, action, row.Action)
			require.Nil(t, row.ActorID)
			require.Equal(t, auditActorRoleSystem, row.ActorRole)
			require.Equal(t, auditEntityTypeCampaignCreator, row.EntityType)
			require.NotNil(t, row.EntityID)
			require.Equal(t, ccID, *row.EntityID)
			require.JSONEq(t, expectedPayloadJSON, string(row.NewValue))
		}).Return(nil)
}

// happyClaim — фикстура заявки, которая попадает в Phase 1 SELECT.
func happyClaim() *repository.AgreedClaimRow {
	return &repository.AgreedClaimRow{
		CampaignCreatorID:   "cc-1",
		CampaignID:          "camp-1",
		CreatorID:           "cr-1",
		CreatorIIN:          "880101300123",
		CreatorLastName:     "Иванов",
		CreatorFirstName:    "Иван",
		CreatorMiddleName:   pointer.ToString("Иванович"),
		CreatorPhone:        "+7 707 123 45 67",
		ContractTemplatePDF: []byte("fake-template"),
	}
}

func TestContractSenderService_RunOnce_HappyPath(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{happyClaim()}, nil)
	rig.contracts.EXPECT().Insert(mock.Anything, repository.ContractRow{
		SubjectKind:       repository.ContractSubjectKindCampaignCreator,
		TrustMeStatusCode: 0,
	}).Return(&repository.ContractRow{
		ID:          "ct-1",
		SubjectKind: repository.ContractSubjectKindCampaignCreator,
	}, nil)
	rig.cc.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	rig.renderer.EXPECT().Render(mock.Anything, mock.MatchedBy(func(d contract.ContractData) bool {
		return d.CreatorFIO == "Иванов Иван Иванович" &&
			d.CreatorIIN == "880101300123" &&
			d.IssuedDate != ""
	})).Return([]byte("rendered-pdf"), nil)
	rig.contracts.EXPECT().UpdateUnsignedPDF(mock.Anything, "ct-1", []byte("rendered-pdf")).Return(nil)
	rig.tm.EXPECT().SendToSign(mock.Anything, mock.MatchedBy(func(in trustme.SendToSignInput) bool {
		return in.AdditionalInfo == "ct-1" &&
			len(in.Requisites) == 1 &&
			in.Requisites[0].FIO == "Иванов Иван Иванович" &&
			in.Requisites[0].IINBIN == "880101300123" &&
			in.Requisites[0].PhoneNumber == "+77071234567"
	})).Return(&trustme.SendToSignResult{
		DocumentID: "doc-A",
		ShortURL:   "https://short",
	}, nil)
	rig.contracts.EXPECT().UpdateAfterSend(mock.Anything, "ct-1", "doc-A", "https://short", 0).Return(nil)
	expectAuditRow(t, rig, "campaign_creator.contract_initiated", "cc-1",
		`{"contract_id":"ct-1","campaign_creator_id":"cc-1","trustme_document_id":"doc-A"}`)
	rig.resolver.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-1"}).Return(map[string]int64{"cr-1": 1001}, nil)
	rig.notifier.EXPECT().NotifyContractSent(mock.Anything, int64(1001))

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

func TestContractSenderService_RunOnce_RenderFails(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{CampaignCreatorID: "cc-1", CreatorID: "cr-1", ContractTemplatePDF: []byte("x")},
	}, nil)
	rig.contracts.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{ID: "ct-1"}, nil)
	rig.cc.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	rig.renderer.EXPECT().Render(mock.Anything, mock.Anything).Return(nil, errors.New("bad PDF"))
	// Cluster A: render fail → recordFailedAttempt (раньше log+return без backoff).
	expectRecordFailedAttempt(t, rig, "ct-1", "", "bad PDF")
	rig.logger.EXPECT().Error(mock.Anything, "contract: phase 2a render", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

func TestContractSenderService_RunOnce_SendFailsLeavesOrphan(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{CampaignCreatorID: "cc-1", CreatorID: "cr-1", ContractTemplatePDF: []byte("x")},
	}, nil)
	rig.contracts.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{ID: "ct-1"}, nil)
	rig.cc.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	rig.renderer.EXPECT().Render(mock.Anything, mock.Anything).Return([]byte("p"), nil)
	rig.contracts.EXPECT().UpdateUnsignedPDF(mock.Anything, "ct-1", mock.Anything).Return(nil)
	rig.tm.EXPECT().SendToSign(mock.Anything, mock.Anything).Return(nil, errors.New("502"))
	// Phase 2c send fail → recordFailedAttempt: untyped error → code пустой,
	// message — текст ошибки, next_retry_at = now() + retryBackoff.
	expectRecordFailedAttempt(t, rig, "ct-1", "", "502")
	rig.logger.EXPECT().Error(mock.Anything, "contract: phase 2c send", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_Resend_WithRealRequisites(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	// Phase 3 finalize Tx (resend ветвь успешна) + Phase 1 claim Tx (пустой).
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan", UnsignedPDFContent: []byte("persisted-pdf")},
	}, nil)
	rig.tm.EXPECT().SearchContractByAdditionalInfo(mock.Anything, "ct-orphan").Return(nil, trustme.ErrTrustMeNotFound)
	rig.contracts.EXPECT().GetOrphanRequisites(mock.Anything, "ct-orphan").Return(&repository.OrphanRequisites{
		CampaignCreatorID: "cc-orphan",
		CreatorID:         "cr-orphan",
		CreatorIIN:        "880101300123",
		CreatorLastName:   "Иванов",
		CreatorFirstName:  "Иван",
		CreatorMiddleName: pointer.ToString("Иванович"),
		CreatorPhone:      "8 707 123 45 67",
	}, nil)
	rig.tm.EXPECT().SendToSign(mock.Anything, mock.MatchedBy(func(in trustme.SendToSignInput) bool {
		return in.AdditionalInfo == "ct-orphan" &&
			in.Requisites[0].FIO == "Иванов Иван Иванович" &&
			in.Requisites[0].IINBIN == "880101300123" &&
			in.Requisites[0].PhoneNumber == "+77071234567" &&
			// PDF в resend — base64 от persisted bytes (тот же sha256, не re-render).
			in.PDFBase64 == "cGVyc2lzdGVkLXBkZg=="
	})).Return(&trustme.SendToSignResult{
		DocumentID: "doc-resent",
		ShortURL:   "https://resent",
	}, nil)
	rig.contracts.EXPECT().UpdateAfterSend(mock.Anything, "ct-orphan", "doc-resent", "https://resent", 0).Return(nil)
	expectAuditRow(t, rig, "campaign_creator.contract_orphan_recovered", "cc-orphan",
		`{"contract_id":"ct-orphan","campaign_creator_id":"cc-orphan","trustme_document_id":"doc-resent"}`)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)
	rig.resolver.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-orphan"}).Return(map[string]int64{"cr-orphan": 555}, nil)
	rig.notifier.EXPECT().NotifyContractSent(mock.Anything, int64(555))

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_OrphanWithoutPDF_LogsAndSkips(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-pre-persist", UnsignedPDFContent: nil},
	}, nil)
	rig.tm.EXPECT().SearchContractByAdditionalInfo(mock.Anything, "ct-pre-persist").Return(nil, trustme.ErrTrustMeNotFound)
	// Cluster A: orphan без unsigned_pdf → log + recordFailedAttempt.
	expectRecordFailedAttempt(t, rig, "ct-pre-persist", "",
		"manual intervention: orphan without unsigned pdf")
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)
	rig.logger.EXPECT().Error(mock.Anything,
		"contract: phase 0 orphan without unsigned pdf — manual intervention needed",
		mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_KnownDoc_Finalize(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan", UnsignedPDFContent: []byte("pdf")},
	}, nil)
	rig.tm.EXPECT().SearchContractByAdditionalInfo(mock.Anything, "ct-orphan").Return(&trustme.SearchContractResult{
		DocumentID: "doc-known", ShortURL: "url", ContractStatus: 2,
	}, nil)
	rig.contracts.EXPECT().GetOrphanRequisites(mock.Anything, "ct-orphan").Return(&repository.OrphanRequisites{
		CampaignCreatorID: "cc-orphan",
		CreatorID:         "cr-orphan",
		CreatorIIN:        "880101300123",
		CreatorLastName:   "Иванов",
		CreatorFirstName:  "Иван",
		CreatorPhone:      "+77071234567",
	}, nil)
	rig.contracts.EXPECT().UpdateAfterSend(mock.Anything, "ct-orphan", "doc-known", "url", 2).Return(nil)
	expectAuditRow(t, rig, "campaign_creator.contract_orphan_recovered", "cc-orphan",
		`{"contract_id":"ct-orphan","campaign_creator_id":"cc-orphan","trustme_document_id":"doc-known"}`)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)
	rig.resolver.EXPECT().GetTelegramUserIDsByIDs(mock.Anything, []string{"cr-orphan"}).Return(map[string]int64{}, nil)
	rig.logger.EXPECT().Info(mock.Anything, "contract: phase 0 recovered known document", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

// Cluster A new branch: Phase 0 search transient error → recordFailedAttempt +
// log + skip (раньше log+return без backoff = log spam каждые 10s).
func TestContractSenderService_Phase0_SearchTransientError_RecordsBackoff(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan", UnsignedPDFContent: []byte("pdf")},
	}, nil)
	rig.tm.EXPECT().SearchContractByAdditionalInfo(mock.Anything, "ct-orphan").Return(nil, errors.New("trustme: search http 503"))
	expectRecordFailedAttempt(t, rig, "ct-orphan", "", "trustme: search http 503")
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)
	rig.logger.EXPECT().Error(mock.Anything, "contract: phase 0 search", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

// Cluster A new branch: GetOrphanRequisites fails (FK SET NULL race) →
// recordFailedAttempt instead of vanilla log+return.
func TestContractSenderService_Phase0_FinalizeKnown_RequisitesLookupFails(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan"},
	}, nil)
	rig.tm.EXPECT().SearchContractByAdditionalInfo(mock.Anything, "ct-orphan").Return(&trustme.SearchContractResult{
		DocumentID: "doc-known", ShortURL: "url", ContractStatus: 2,
	}, nil)
	rig.contracts.EXPECT().GetOrphanRequisites(mock.Anything, "ct-orphan").Return(nil, errors.New("sql: no rows in result set"))
	expectRecordFailedAttempt(t, rig, "ct-orphan", "", "sql: no rows in result set")
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)
	rig.logger.EXPECT().Error(mock.Anything, "contract: phase 0 finalize lookup", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}

// Cluster A: Phase 2b persist fail → recordFailedAttempt.
func TestContractSenderService_Phase2b_PersistFails_RecordsBackoff(t *testing.T) {
	t.Parallel()

	rig := newSenderRig(t)
	rig.pool.ExpectBegin()
	rig.pool.ExpectCommit()

	rig.contracts.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	rig.contracts.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{CampaignCreatorID: "cc-1", CreatorID: "cr-1", ContractTemplatePDF: []byte("x")},
	}, nil)
	rig.contracts.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{ID: "ct-1"}, nil)
	rig.cc.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	rig.renderer.EXPECT().Render(mock.Anything, mock.Anything).Return([]byte("p"), nil)
	rig.contracts.EXPECT().UpdateUnsignedPDF(mock.Anything, "ct-1", []byte("p")).Return(errors.New("db down"))
	expectRecordFailedAttempt(t, rig, "ct-1", "", "db down")
	rig.logger.EXPECT().Error(mock.Anything, "contract: phase 2b persist", mock.Anything).Return()

	svc := rig.build(t)
	svc.RunOnce(context.Background())

	require.NoError(t, rig.pool.ExpectationsWereMet())
}
