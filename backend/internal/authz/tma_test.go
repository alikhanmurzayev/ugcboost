package authz

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz/mocks"
	dbmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
)

// ctxWithCreator returns a context with role=creator and creator_id=cr-1, as
// the TMA initData middleware would stamp on success.
func ctxWithCreator() context.Context {
	ctx := context.WithValue(context.Background(), middleware.ContextKeyRole, api.Creator)
	return context.WithValue(ctx, middleware.ContextKeyCreatorID, "cr-1")
}

func newAuthzWithFactory(t *testing.T) (*AuthzService, *mocks.MockRepoFactory, *repomocks.MockCampaignRepo, *repomocks.MockCampaignCreatorRepo) {
	t.Helper()
	rf := mocks.NewMockRepoFactory(t)
	pool := dbmocks.NewMockPool(t)
	campRepo := repomocks.NewMockCampaignRepo(t)
	ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
	rf.EXPECT().NewCampaignRepo(mock.Anything).Return(campRepo).Maybe()
	rf.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo).Maybe()
	svc := NewAuthzService(mocks.NewMockBrandService(t), pool, rf)
	return svc, rf, campRepo, ccRepo
}

func TestAuthzService_AuthorizeTMACampaignDecision(t *testing.T) {
	t.Parallel()

	t.Run("role missing → ErrTMAForbidden", func(t *testing.T) {
		t.Parallel()
		svc, _, _, _ := newAuthzWithFactory(t)
		_, err := svc.AuthorizeTMACampaignDecision(context.Background(), "abc_padding_secrettokenxx")
		require.ErrorIs(t, err, domain.ErrTMAForbidden)
	})

	t.Run("creator_id missing → ErrTMAForbidden", func(t *testing.T) {
		t.Parallel()
		svc, _, _, _ := newAuthzWithFactory(t)
		ctx := context.WithValue(context.Background(), middleware.ContextKeyRole, api.Creator)
		_, err := svc.AuthorizeTMACampaignDecision(ctx, "abc_padding_secrettokenxx")
		require.ErrorIs(t, err, domain.ErrTMAForbidden)
	})

	t.Run("campaign not found → ErrCampaignNotFound", func(t *testing.T) {
		t.Parallel()
		svc, _, campRepo, _ := newAuthzWithFactory(t)
		campRepo.EXPECT().GetBySecretToken(mock.Anything, "abc_padding_secrettokenxx").
			Return(nil, sql.ErrNoRows)

		_, err := svc.AuthorizeTMACampaignDecision(ctxWithCreator(), "abc_padding_secrettokenxx")
		require.ErrorIs(t, err, domain.ErrCampaignNotFound)
	})

	t.Run("campaign lookup db error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, campRepo, _ := newAuthzWithFactory(t)
		campRepo.EXPECT().GetBySecretToken(mock.Anything, "abc_padding_secrettokenxx").
			Return(nil, errors.New("db down"))

		_, err := svc.AuthorizeTMACampaignDecision(ctxWithCreator(), "abc_padding_secrettokenxx")
		require.ErrorContains(t, err, "authz lookup campaign")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("creator not in campaign → ErrTMAForbidden", func(t *testing.T) {
		t.Parallel()
		svc, _, campRepo, ccRepo := newAuthzWithFactory(t)
		campRepo.EXPECT().GetBySecretToken(mock.Anything, "abc_padding_secrettokenxx").
			Return(&repository.CampaignRow{ID: "camp-1"}, nil)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(nil, sql.ErrNoRows)

		_, err := svc.AuthorizeTMACampaignDecision(ctxWithCreator(), "abc_padding_secrettokenxx")
		require.ErrorIs(t, err, domain.ErrTMAForbidden)
	})

	t.Run("campaign_creator lookup db error wrapped", func(t *testing.T) {
		t.Parallel()
		svc, _, campRepo, ccRepo := newAuthzWithFactory(t)
		campRepo.EXPECT().GetBySecretToken(mock.Anything, "abc_padding_secrettokenxx").
			Return(&repository.CampaignRow{ID: "camp-1"}, nil)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(nil, errors.New("db down"))

		_, err := svc.AuthorizeTMACampaignDecision(ctxWithCreator(), "abc_padding_secrettokenxx")
		require.ErrorContains(t, err, "authz lookup campaign_creator")
		require.ErrorContains(t, err, "db down")
	})

	t.Run("happy path returns auth tuple", func(t *testing.T) {
		t.Parallel()
		svc, _, campRepo, ccRepo := newAuthzWithFactory(t)
		campRepo.EXPECT().GetBySecretToken(mock.Anything, "abc_padding_secrettokenxx").
			Return(&repository.CampaignRow{ID: "camp-1"}, nil)
		ccRepo.EXPECT().GetByCampaignAndCreator(mock.Anything, "camp-1", "cr-1").
			Return(&repository.CampaignCreatorRow{ID: "cc-1", Status: domain.CampaignCreatorStatusInvited}, nil)

		got, err := svc.AuthorizeTMACampaignDecision(ctxWithCreator(), "abc_padding_secrettokenxx")
		require.NoError(t, err)
		require.Equal(t, TMACampaignDecisionAuth{
			CreatorID:         "cr-1",
			CampaignID:        "camp-1",
			CampaignCreatorID: "cc-1",
			CurrentStatus:     domain.CampaignCreatorStatusInvited,
		}, got)
	})
}
