package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	svcmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/service/mocks"
)

const (
	tmaTestCampaignID = "camp-1"
	tmaTestCreatorID  = "cr-1"
	tmaTestCCID       = "cc-1"
)

func tmaTestAuth() TmaDecisionAuth {
	return TmaDecisionAuth{
		CampaignID:        tmaTestCampaignID,
		CreatorID:         tmaTestCreatorID,
		CampaignCreatorID: tmaTestCCID,
	}
}

func newTmaCampaignCreatorTestRig(t *testing.T) (*TmaCampaignCreatorService, *svcmocks.MockTmaCampaignCreatorRepoFactory, *repomocks.MockCampaignCreatorRepo, *repomocks.MockAuditRepo) {
	t.Helper()
	pool := dbmocks.NewMockPool(t)
	pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil).Maybe()

	factory := svcmocks.NewMockTmaCampaignCreatorRepoFactory(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	auditRepo := repomocks.NewMockAuditRepo(t)
	factory.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo).Maybe()
	factory.EXPECT().NewAuditRepo(mock.Anything).Return(auditRepo).Maybe()

	log := logmocks.NewMockLogger(t)
	log.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Maybe()

	svc := NewTmaCampaignCreatorService(pool, factory, log)
	return svc, factory, ccRepo, auditRepo
}

func TestTmaCampaignCreatorService_ApplyDecision(t *testing.T) {
	t.Parallel()

	t.Run("happy agree from invited writes UPDATE + audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil)
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil)
		auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusAgreed,
			AlreadyDecided: false,
		}, got)
	})

	t.Run("happy decline from invited writes UPDATE + audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil)
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusDeclined).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil)
		auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusDeclined,
			AlreadyDecided: false,
		}, got)
	})

	t.Run("idempotent agree from agreed → AlreadyDecided=true, no UPDATE/audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil)

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusAgreed,
			AlreadyDecided: true,
		}, got)
	})

	t.Run("idempotent decline from declined → AlreadyDecided=true", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil)

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusDeclined,
			AlreadyDecided: true,
		}, got)
	})

	t.Run("decline from agreed → ErrCampaignCreatorAlreadyAgreed", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil)

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorAlreadyAgreed)
	})

	t.Run("agree from declined → ErrCampaignCreatorDeclinedNeedReinvite", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil)

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorDeclinedNeedReinvite)
	})

	t.Run("any decision from planned → ErrCampaignCreatorNotInvited", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusPlanned}, nil)

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorNotInvited)
	})

	t.Run("GetByIDForUpdate db error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(nil, errors.New("db down"))

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "lock campaign_creator")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("ApplyDecision repo error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil)
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(nil, errors.New("update failed"))

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "apply decision")
		require.ErrorContains(t, err, "update failed")
	})

	t.Run("audit error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil)
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil)
		auditRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("audit down")).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "audit decision")
		require.ErrorContains(t, err, "audit down")
	})
}
