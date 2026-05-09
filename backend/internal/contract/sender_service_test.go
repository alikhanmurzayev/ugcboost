package contract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
)

// stubRenderer возвращает заданный PDF без реального overlay'а.
type stubRenderer struct {
	pdf []byte
	err error
}

func (s *stubRenderer) Render(_ []byte, _ ContractData) ([]byte, error) {
	return s.pdf, s.err
}

// stubTrustMe — конфигурируемый client для unit-тестов RunOnce.
type stubTrustMe struct {
	sendErr      error
	sendResult   *trustme.SendToSignResult
	searchResult *trustme.SearchContractResult
	searchErr    error
	sendCalls    int
	searchCalls  int
	lastSend     trustme.SendToSignInput
}

func (s *stubTrustMe) SendToSign(_ context.Context, in trustme.SendToSignInput) (*trustme.SendToSignResult, error) {
	s.sendCalls++
	s.lastSend = in
	return s.sendResult, s.sendErr
}

func (s *stubTrustMe) SearchContractByAdditionalInfo(_ context.Context, _ string) (*trustme.SearchContractResult, error) {
	s.searchCalls++
	return s.searchResult, s.searchErr
}

func (s *stubTrustMe) DownloadContractFile(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// stubNotifier — записывает вызовы NotifyContractSent.
type stubNotifier struct {
	calls []notifyCall
}

type notifyCall struct {
	chatID   int64
	shortURL string
}

func (s *stubNotifier) NotifyContractSent(_ context.Context, chatID int64, shortURL string) {
	s.calls = append(s.calls, notifyCall{chatID, shortURL})
}

// stubResolver — креатор → telegramID без реального DB.
type stubResolver struct {
	mapping map[string]int64
}

func (s *stubResolver) GetTelegramUserIDsByIDs(_ context.Context, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, id := range ids {
		if v, ok := s.mapping[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// repoStubFactory — все три repo в одном месте.
type repoStubFactory struct {
	contracts repository.ContractRepo
	cc        repository.CampaignCreatorRepo
	audit     repository.AuditRepo
}

func (r repoStubFactory) NewContractsRepo(_ dbutil.DB) repository.ContractRepo {
	return r.contracts
}

func (r repoStubFactory) NewCampaignCreatorRepo(_ dbutil.DB) repository.CampaignCreatorRepo {
	return r.cc
}

func (r repoStubFactory) NewAuditRepo(_ dbutil.DB) repository.AuditRepo {
	return r.audit
}

func newPgxmockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(func() { mock.Close() })
	return mock
}

func TestContractSenderService_RunOnce_HappyPath(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	pool.ExpectBegin()
	pool.ExpectCommit()
	pool.ExpectBegin()
	pool.ExpectCommit()

	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory := repoStubFactory{contracts: contractsRepo, cc: ccRepo, audit: auditRepo}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{
			CampaignCreatorID:   "cc-1",
			CampaignID:          "camp-1",
			CreatorID:           "cr-1",
			CreatorIIN:          "880101300123",
			CreatorLastName:     "Иванов",
			CreatorFirstName:    "Иван",
			CreatorMiddleName:   pointer.ToString("Иванович"),
			CreatorPhone:        "+7 707 123 45 67",
			ContractTemplatePDF: []byte("fake-template"),
		},
	}, nil)
	contractsRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{
		ID:          "ct-1",
		SubjectKind: repository.ContractSubjectKindCampaignCreator,
	}, nil)
	ccRepo.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	contractsRepo.EXPECT().UpdateUnsignedPDF(mock.Anything, "ct-1", []byte("rendered-pdf")).Return(nil)
	contractsRepo.EXPECT().UpdateAfterSend(mock.Anything, "ct-1", "doc-A", "https://short", 0).Return(nil)
	auditRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
		return row.Action == "campaign_creator.contract_initiated" &&
			row.ActorID == nil &&
			strings.Contains(string(row.NewValue), "ct-1")
	})).Return(nil)

	tm := &stubTrustMe{
		sendResult: &trustme.SendToSignResult{
			DocumentID: "doc-A",
			ShortURL:   "https://short",
		},
	}
	notifier := &stubNotifier{}
	resolver := &stubResolver{mapping: map[string]int64{"cr-1": 1001}}

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{pdf: []byte("rendered-pdf")},
		resolver, notifier, logmocks.NewMockLogger(t), time.Hour)

	svc.RunOnce(context.Background())

	require.Equal(t, 1, tm.sendCalls)
	require.Equal(t, "ct-1", tm.lastSend.AdditionalInfo)
	require.Equal(t, "+77071234567", tm.lastSend.Requisites[0].PhoneNumber)
	require.Equal(t, "Иванов Иван Иванович", tm.lastSend.Requisites[0].FIO)
	require.Equal(t, "880101300123", tm.lastSend.Requisites[0].IINBIN)
	require.Len(t, notifier.calls, 1)
	require.Equal(t, int64(1001), notifier.calls[0].chatID)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestContractSenderService_RunOnce_RenderFails(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	pool.ExpectBegin()
	pool.ExpectCommit()

	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	factory := repoStubFactory{
		contracts: contractsRepo,
		cc:        ccRepo,
		audit:     repomocks.NewMockAuditRepo(t),
	}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{CampaignCreatorID: "cc-1", CreatorID: "cr-1", ContractTemplatePDF: []byte("x")},
	}, nil)
	contractsRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{ID: "ct-1"}, nil)
	ccRepo.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)

	tm := &stubTrustMe{}
	logger := logmocks.NewMockLogger(t)
	logger.EXPECT().Error(mock.Anything, "contract: phase 2a render", mock.Anything).Return()

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{err: errors.New("bad PDF")},
		&stubResolver{}, &stubNotifier{}, logger, time.Hour)
	svc.RunOnce(context.Background())

	require.Zero(t, tm.sendCalls)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestContractSenderService_RunOnce_SendFailsLeavesOrphan(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	pool.ExpectBegin()
	pool.ExpectCommit()

	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory := repoStubFactory{contracts: contractsRepo, cc: ccRepo, audit: auditRepo}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return(nil, nil)
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return([]*repository.AgreedClaimRow{
		{CampaignCreatorID: "cc-1", CreatorID: "cr-1", ContractTemplatePDF: []byte("x")},
	}, nil)
	contractsRepo.EXPECT().Insert(mock.Anything, mock.Anything).Return(&repository.ContractRow{ID: "ct-1"}, nil)
	ccRepo.EXPECT().UpdateContractIDAndStatus(mock.Anything, "cc-1", "ct-1", "signing").Return(nil)
	contractsRepo.EXPECT().UpdateUnsignedPDF(mock.Anything, "ct-1", mock.Anything).Return(nil)
	// Phase 2c send fail → recordFailedAttempt: untyped error → code пустой,
	// message — текст ошибки, next_retry_at = now() + retryBackoff.
	contractsRepo.EXPECT().RecordFailedAttempt(mock.Anything, "ct-1", "", "502", mock.AnythingOfType("time.Time")).Return(nil)

	tm := &stubTrustMe{sendErr: errors.New("502")}
	logger := logmocks.NewMockLogger(t)
	logger.EXPECT().Error(mock.Anything, "contract: phase 2c send", mock.Anything).Return()

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{pdf: []byte("p")}, &stubResolver{}, &stubNotifier{}, logger, time.Hour)
	svc.RunOnce(context.Background())

	require.Equal(t, 1, tm.sendCalls)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_Resend_WithRealRequisites(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	// Phase 3 finalize Tx (resend ветвь успешна) + Phase 1 claim Tx (пустой).
	pool.ExpectBegin()
	pool.ExpectCommit()
	pool.ExpectBegin()
	pool.ExpectCommit()

	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory := repoStubFactory{contracts: contractsRepo, cc: ccRepo, audit: auditRepo}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan", UnsignedPDFContent: []byte("persisted-pdf")},
	}, nil)
	contractsRepo.EXPECT().GetOrphanRequisites(mock.Anything, "ct-orphan").Return(&repository.OrphanRequisites{
		CampaignCreatorID: "cc-orphan",
		CreatorID:         "cr-orphan",
		CreatorIIN:        "880101300123",
		CreatorLastName:   "Иванов",
		CreatorFirstName:  "Иван",
		CreatorMiddleName: pointer.ToString("Иванович"),
		CreatorPhone:      "8 707 123 45 67",
	}, nil)
	contractsRepo.EXPECT().UpdateAfterSend(mock.Anything, "ct-orphan", "doc-resent", "https://resent", 0).Return(nil)
	auditRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
		return row.Action == "campaign_creator.contract_orphan_recovered"
	})).Return(nil)
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)

	tm := &stubTrustMe{
		searchErr: trustme.ErrTrustMeNotFound,
		sendResult: &trustme.SendToSignResult{
			DocumentID: "doc-resent",
			ShortURL:   "https://resent",
		},
	}
	notifier := &stubNotifier{}
	resolver := &stubResolver{mapping: map[string]int64{"cr-orphan": 555}}

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{}, resolver, notifier, logmocks.NewMockLogger(t), time.Hour)
	svc.RunOnce(context.Background())

	require.Equal(t, 1, tm.searchCalls)
	require.Equal(t, 1, tm.sendCalls)
	require.Equal(t, "ct-orphan", tm.lastSend.AdditionalInfo)
	require.Equal(t, "Иванов Иван Иванович", tm.lastSend.Requisites[0].FIO)
	require.Equal(t, "880101300123", tm.lastSend.Requisites[0].IINBIN)
	require.Equal(t, "+77071234567", tm.lastSend.Requisites[0].PhoneNumber)
	// PDF в resend — base64 от persisted bytes (тот же sha256, не re-render).
	require.Contains(t, tm.lastSend.PDFBase64, "cGVyc2lzdGVkLXBkZg==")
	require.Len(t, notifier.calls, 1)
	require.Equal(t, int64(555), notifier.calls[0].chatID)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_OrphanWithoutPDF_LogsAndSkips(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory := repoStubFactory{contracts: contractsRepo, cc: ccRepo, audit: auditRepo}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-pre-persist", UnsignedPDFContent: nil},
	}, nil)
	pool.ExpectBegin()
	pool.ExpectCommit()
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)

	tm := &stubTrustMe{searchErr: trustme.ErrTrustMeNotFound}
	logger := logmocks.NewMockLogger(t)
	logger.EXPECT().Error(mock.Anything,
		"contract: phase 0 orphan without unsigned pdf — manual intervention needed",
		mock.Anything).Return()

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{}, &stubResolver{}, &stubNotifier{}, logger, time.Hour)
	svc.RunOnce(context.Background())

	require.Zero(t, tm.sendCalls)
	require.NoError(t, pool.ExpectationsWereMet())
}

func TestContractSenderService_Phase0_KnownDoc_Finalize(t *testing.T) {
	t.Parallel()

	pool := newPgxmockPool(t)
	pool.ExpectBegin()
	pool.ExpectCommit()
	pool.ExpectBegin()
	pool.ExpectCommit()

	contractsRepo := repomocks.NewMockContractRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory := repoStubFactory{contracts: contractsRepo, cc: ccRepo, audit: auditRepo}

	contractsRepo.EXPECT().SelectOrphansForRecovery(mock.Anything, recoveryBatchSize).Return([]*repository.OrphanRow{
		{ContractID: "ct-orphan", UnsignedPDFContent: []byte("pdf")},
	}, nil)
	contractsRepo.EXPECT().GetOrphanRequisites(mock.Anything, "ct-orphan").Return(&repository.OrphanRequisites{
		CampaignCreatorID: "cc-orphan",
		CreatorID:         "cr-orphan",
		CreatorIIN:        "880101300123",
		CreatorLastName:   "Иванов",
		CreatorFirstName:  "Иван",
		CreatorPhone:      "+77071234567",
	}, nil)
	contractsRepo.EXPECT().UpdateAfterSend(mock.Anything, "ct-orphan", "doc-known", "url", 2).Return(nil)
	auditRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row repository.AuditLogRow) bool {
		return row.Action == "campaign_creator.contract_orphan_recovered" &&
			strings.Contains(string(row.NewValue), "cc-orphan")
	})).Return(nil)
	contractsRepo.EXPECT().SelectAgreedForClaim(mock.Anything, claimBatchSize).Return(nil, nil)

	tm := &stubTrustMe{
		searchResult: &trustme.SearchContractResult{
			DocumentID: "doc-known", ShortURL: "url", ContractStatus: 2,
		},
	}
	logger := logmocks.NewMockLogger(t)
	logger.EXPECT().Info(mock.Anything, "contract: phase 0 recovered known document", mock.Anything).Return()

	svc := NewContractSenderService(pool, factory, tm,
		&stubRenderer{}, &stubResolver{}, &stubNotifier{}, logger, time.Hour)
	svc.RunOnce(context.Background())

	require.Equal(t, 1, tm.searchCalls)
	require.Zero(t, tm.sendCalls)
	require.NoError(t, pool.ExpectationsWereMet())
}
