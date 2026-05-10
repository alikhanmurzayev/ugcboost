package service

import (
	"context"
	"encoding/json"
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

// newTmaCampaignCreatorTestRig wires up the service with mocks. Infrastructure
// expectations (`pool.Begin`, factory constructors, success log) are set with
// `.Maybe()` because the number of invocations differs by scenario (idempotent
// paths skip the audit repo entirely). Business expectations (cc-repo /
// audit-repo) are set inline in each t.Run with `.Once()` / `AssertNotCalled`
// so a regression cannot slip through.
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

// captureAuditEntry asserts the captured audit row carries the expected
// action / entity / payload, including JSON-equality on the marshalled
// payload (map iteration order is not deterministic). Required for TMA
// flow because actor_id is NULL — the only signal that distinguishes an
// agree from a decline (or a wrong entity_id) is the row contents.
func captureAuditEntry(t *testing.T, expectedAction, expectedEntityType, expectedEntityID, expectedPayloadJSON string) func(ctx context.Context, row repository.AuditLogRow) {
	t.Helper()
	return func(_ context.Context, row repository.AuditLogRow) {
		require.Equal(t, expectedAction, row.Action)
		require.Equal(t, expectedEntityType, row.EntityType)
		require.NotNil(t, row.EntityID, "TMA audit row must carry entity_id")
		require.Equal(t, expectedEntityID, *row.EntityID)
		require.Nil(t, row.ActorID, "TMA audit row must have NULL actor_id (public endpoint)")
		require.Empty(t, row.ActorRole, "TMA audit row must have empty actor_role (public endpoint)")
		require.Nil(t, row.OldValue, "TMA audit row must omit old_value")
		require.NotNil(t, row.NewValue)
		require.JSONEq(t, expectedPayloadJSON, string(row.NewValue))
	}
}

func TestTmaCampaignCreatorService_ApplyDecision(t *testing.T) {
	t.Parallel()

	t.Run("happy agree from invited writes UPDATE + audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil).Once()
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil).Once()
		expectedPayload, err := json.Marshal(map[string]string{
			"campaign_id": tmaTestCampaignID,
			"creator_id":  tmaTestCreatorID,
		})
		require.NoError(t, err)
		auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(captureAuditEntry(t,
				AuditActionCampaignCreatorAgree,
				AuditEntityTypeCampaignCreator,
				tmaTestCCID,
				string(expectedPayload),
			)).
			Return(nil).Once()

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
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil).Once()
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusDeclined).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil).Once()
		expectedPayload, err := json.Marshal(map[string]string{
			"campaign_id": tmaTestCampaignID,
			"creator_id":  tmaTestCreatorID,
		})
		require.NoError(t, err)
		auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(captureAuditEntry(t,
				AuditActionCampaignCreatorDecline,
				AuditEntityTypeCampaignCreator,
				tmaTestCCID,
				string(expectedPayload),
			)).
			Return(nil).Once()

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusDeclined,
			AlreadyDecided: false,
		}, got)
	})

	t.Run("idempotent agree from agreed → AlreadyDecided=true, no UPDATE/audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil).Once()

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusAgreed,
			AlreadyDecided: true,
		}, got)
		ccRepo.AssertNotCalled(t, "ApplyDecision", mock.Anything, mock.Anything, mock.Anything)
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("idempotent decline from declined → AlreadyDecided=true, no UPDATE/audit", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil).Once()

		got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.NoError(t, err)
		require.Equal(t, domain.CampaignCreatorDecisionResult{
			Status:         domain.CampaignCreatorStatusDeclined,
			AlreadyDecided: true,
		}, got)
		ccRepo.AssertNotCalled(t, "ApplyDecision", mock.Anything, mock.Anything, mock.Anything)
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("decline from agreed → ErrCampaignCreatorAlreadyAgreed", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorAlreadyAgreed)
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	postAgreed := []string{
		domain.CampaignCreatorStatusSigning,
		domain.CampaignCreatorStatusSigned,
		domain.CampaignCreatorStatusSigningDeclined,
	}

	for _, status := range postAgreed {
		t.Run("idempotent agree from "+status+" → AlreadyDecided=true preserves current status", func(t *testing.T) {
			t.Parallel()
			svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
			ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
				Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: status}, nil).Once()

			got, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
			require.NoError(t, err)
			require.Equal(t, domain.CampaignCreatorDecisionResult{
				Status:         status,
				AlreadyDecided: true,
			}, got)
			ccRepo.AssertNotCalled(t, "ApplyDecision", mock.Anything, mock.Anything, mock.Anything)
			auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})

		t.Run("decline from "+status+" → ErrCampaignCreatorAlreadyAgreed", func(t *testing.T) {
			t.Parallel()
			svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
			ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
				Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: status}, nil).Once()

			_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionDecline)
			require.ErrorIs(t, err, domain.ErrCampaignCreatorAlreadyAgreed)
			ccRepo.AssertNotCalled(t, "ApplyDecision", mock.Anything, mock.Anything, mock.Anything)
			auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}

	t.Run("agree from declined → ErrCampaignCreatorDeclinedNeedReinvite", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusDeclined}, nil).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorDeclinedNeedReinvite)
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("any decision from planned → ErrCampaignCreatorNotInvited", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusPlanned}, nil).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorIs(t, err, domain.ErrCampaignCreatorNotInvited)
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("unexpected campaign_creator status surfaces internal error", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: "bogus-status"}, nil).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "unexpected campaign_creator status")
		require.ErrorContains(t, err, "bogus-status")
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("unexpected decision from invited surfaces internal error", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecision("bogus-decision"))
		require.ErrorContains(t, err, "unexpected campaign_creator decision")
		require.ErrorContains(t, err, "bogus-decision")
		auditRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("GetByIDForUpdate db error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(nil, errors.New("db down")).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "lock campaign_creator")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("ApplyDecision repo error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, _ := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil).Once()
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(nil, errors.New("update failed")).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "apply decision")
		require.ErrorContains(t, err, "update failed")
	})

	t.Run("audit error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, ccRepo, auditRepo := newTmaCampaignCreatorTestRig(t)
		ccRepo.EXPECT().GetByIDForUpdate(mock.Anything, tmaTestCCID).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusInvited}, nil).Once()
		ccRepo.EXPECT().ApplyDecision(mock.Anything, tmaTestCCID, domain.CampaignCreatorStatusAgreed).
			Return(&repository.CampaignCreatorRow{ID: tmaTestCCID, Status: domain.CampaignCreatorStatusAgreed}, nil).Once()
		expectedPayload, marshalErr := json.Marshal(map[string]string{
			"campaign_id": tmaTestCampaignID,
			"creator_id":  tmaTestCreatorID,
		})
		require.NoError(t, marshalErr)
		auditRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("repository.AuditLogRow")).
			Run(captureAuditEntry(t,
				AuditActionCampaignCreatorAgree,
				AuditEntityTypeCampaignCreator,
				tmaTestCCID,
				string(expectedPayload),
			)).
			Return(errors.New("audit down")).Once()

		_, err := svc.ApplyDecision(context.Background(), tmaTestAuth(), domain.CampaignCreatorDecisionAgree)
		require.ErrorContains(t, err, "audit decision")
		require.ErrorContains(t, err, "audit down")
	})
}
