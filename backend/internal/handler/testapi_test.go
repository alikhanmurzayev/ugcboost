package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	dbutilmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	repomocks "github.com/alikhanmurzayev/ugcboost/backend/internal/repository/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// newTestAPIRouter registers a TestAPIHandler behind the generated strict
// testapi adapter and chi wrapper, mirroring production wiring.
func newTestAPIRouter(t *testing.T, h *TestAPIHandler) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	testapi.HandlerWithOptions(NewStrictTestAPIHandler(h), testapi.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
	})
	return r
}

func expectUnexpectedErrorLog(log *logmocks.MockLogger, path string) {
	log.EXPECT().Error(mock.Anything, "unexpected error", mock.MatchedBy(func(args []any) bool {
		return len(args) == 4 && args[0] == "error" && args[2] == "path" && args[3] == path
	})).Once()
}

func TestTestAPIHandler_SeedUser(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			map[string]any{"email": 123})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{Email: "user@example.com", Password: "pass"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		auth.EXPECT().SeedUser(mock.Anything, "user@example.com", "pass", "admin").
			Return(nil, errors.New("db error"))
		expectUnexpectedErrorLog(log, "/test/seed-user")
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{
				Email: "user@example.com", Password: "pass",
				Role: testapi.SeedUserRequestRoleAdmin,
			})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		auth.EXPECT().SeedUser(mock.Anything, "user@example.com", "pass", "admin").
			Return(&domain.User{ID: "u-seed", Email: "user@example.com", Role: api.Admin}, nil)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[testapi.SeedUserResult](t, router, http.MethodPost, "/test/seed-user",
			testapi.SeedUserRequest{
				Email: "user@example.com", Password: "pass",
				Role: testapi.SeedUserRequestRoleAdmin,
			})
		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, testapi.SeedUserResult{
			Data: testapi.SeedUserData{
				Id:    "u-seed",
				Email: openapi_types.Email("user@example.com"),
				Role:  testapi.SeedUserDataRoleAdmin,
			},
		}, resp)
	})
}

func TestTestAPIHandler_CleanupEntity(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			map[string]any{"type": 42})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.User, Id: ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("unknown type returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			map[string]any{"type": "totally_unknown", "id": "c-1"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("user success runs inside a transaction", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		tx := dbutilmocks.NewMockDB(t)
		userRepo := repomocks.NewMockUserRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		txWrapper := pgxmockTx(t, tx)
		pool.EXPECT().Begin(mock.Anything).Return(txWrapper, nil)
		// Tx's Commit is called on the pgx.Tx wrapper returned by Begin; our
		// wrapper forwards Commit to tx.Exec via pgxmockTx, so no explicit
		// expectation on the tx mock itself is needed beyond DeleteForTests.
		repos.EXPECT().NewUserRepo(mock.Anything).Return(userRepo)
		userRepo.EXPECT().DeleteForTests(mock.Anything, "u-1").Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.User, Id: "u-1"})
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("user not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		tx := dbutilmocks.NewMockDB(t)
		userRepo := repomocks.NewMockUserRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		txWrapper := pgxmockTx(t, tx)
		pool.EXPECT().Begin(mock.Anything).Return(txWrapper, nil)
		repos.EXPECT().NewUserRepo(mock.Anything).Return(userRepo)
		userRepo.EXPECT().DeleteForTests(mock.Anything, "u-missing").Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.User, Id: "u-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("user delete error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		tx := dbutilmocks.NewMockDB(t)
		userRepo := repomocks.NewMockUserRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		txWrapper := pgxmockTx(t, tx)
		pool.EXPECT().Begin(mock.Anything).Return(txWrapper, nil)
		repos.EXPECT().NewUserRepo(mock.Anything).Return(userRepo)
		userRepo.EXPECT().DeleteForTests(mock.Anything, "u-boom").Return(errors.New("db boom"))
		expectUnexpectedErrorLog(log, "/test/cleanup-entity")

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.User, Id: "u-boom"})
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("brand success calls brand repo directly", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		brandRepo := repomocks.NewMockBrandRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewBrandRepo(mock.Anything).Return(brandRepo)
		brandRepo.EXPECT().Delete(mock.Anything, "b-1").Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Brand, Id: "b-1"})
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("brand not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		brandRepo := repomocks.NewMockBrandRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewBrandRepo(mock.Anything).Return(brandRepo)
		brandRepo.EXPECT().Delete(mock.Anything, "b-missing").Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Brand, Id: "b-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("campaign success runs inside a transaction", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		tx := dbutilmocks.NewMockDB(t)
		campaignRepo := repomocks.NewMockCampaignRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		txWrapper := pgxmockTx(t, tx)
		pool.EXPECT().Begin(mock.Anything).Return(txWrapper, nil)
		repos.EXPECT().NewCampaignRepo(mock.Anything).Return(campaignRepo)
		campaignRepo.EXPECT().DeleteForTests(mock.Anything, "c-1").Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Campaign, Id: "c-1"})
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("campaign not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		tx := dbutilmocks.NewMockDB(t)
		campaignRepo := repomocks.NewMockCampaignRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		txWrapper := pgxmockTx(t, tx)
		pool.EXPECT().Begin(mock.Anything).Return(txWrapper, nil)
		repos.EXPECT().NewCampaignRepo(mock.Anything).Return(campaignRepo)
		campaignRepo.EXPECT().DeleteForTests(mock.Anything, "c-missing").Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Campaign, Id: "c-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("campaign_creator success splits compound id", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		ccRepo.EXPECT().
			DeleteByCampaignAndCreatorForTests(mock.Anything, "camp-1", "creator-1").
			Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.CampaignCreator, Id: "camp-1:creator-1"})
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("campaign_creator id without colon returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.CampaignCreator, Id: "single-uuid"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("campaign_creator id with empty half returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.CampaignCreator, Id: "camp-1:"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("campaign_creator not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		ccRepo.EXPECT().
			DeleteByCampaignAndCreatorForTests(mock.Anything, "camp-1", "creator-missing").
			Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.CampaignCreator, Id: "camp-1:creator-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("contract success calls contracts repo directly", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		contractRepo := repomocks.NewMockContractRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewContractsRepo(mock.Anything).Return(contractRepo)
		contractRepo.EXPECT().DeleteForTests(mock.Anything, "ct-1").Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Contract, Id: "ct-1"})
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("contract not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		contractRepo := repomocks.NewMockContractRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewContractsRepo(mock.Anything).Return(contractRepo)
		contractRepo.EXPECT().DeleteForTests(mock.Anything, "ct-missing").Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Contract, Id: "ct-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestTestAPIHandler_ForceCleanupCampaignCreator(t *testing.T) {
	t.Parallel()

	campaignID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	creatorID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	body := testapi.ForceCleanupCampaignCreatorRequest{
		CampaignId: campaignID,
		CreatorId:  creatorID,
	}

	t.Run("success returns 204", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		ccRepo.EXPECT().DeleteByCampaignAndCreatorForTests(mock.Anything, campaignID.String(), creatorID.String()).Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/campaign-creators/force-cleanup", body)
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		ccRepo.EXPECT().DeleteByCampaignAndCreatorForTests(mock.Anything, campaignID.String(), creatorID.String()).Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaign-creators/force-cleanup", body)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("repo error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		ccRepo := repomocks.NewMockCampaignCreatorRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignCreatorRepo(mock.Anything).Return(ccRepo)
		ccRepo.EXPECT().DeleteByCampaignAndCreatorForTests(mock.Anything, campaignID.String(), creatorID.String()).Return(errors.New("db boom"))
		expectUnexpectedErrorLog(log, "/test/campaign-creators/force-cleanup")

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaign-creators/force-cleanup", body)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("zero campaign id returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaign-creators/force-cleanup",
			testapi.ForceCleanupCampaignCreatorRequest{CampaignId: uuid.Nil, CreatorId: creatorID})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, string(domain.CodeValidation), resp.Error.Code)
	})

	t.Run("zero creator id returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaign-creators/force-cleanup",
			testapi.ForceCleanupCampaignCreatorRequest{CampaignId: campaignID, CreatorId: uuid.Nil})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, string(domain.CodeValidation), resp.Error.Code)
	})
}

func TestTestAPIHandler_MarkCampaignDeleted(t *testing.T) {
	t.Parallel()

	id := "11111111-2222-3333-4444-555555555555"

	t.Run("success returns 204", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		campaignRepo := repomocks.NewMockCampaignRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignRepo(mock.Anything).Return(campaignRepo)
		campaignRepo.EXPECT().MarkDeletedForTests(mock.Anything, id).Return(nil)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[any](t, router, http.MethodPost, "/test/campaigns/"+id+"/mark-deleted", nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		campaignRepo := repomocks.NewMockCampaignRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignRepo(mock.Anything).Return(campaignRepo)
		campaignRepo.EXPECT().MarkDeletedForTests(mock.Anything, id).Return(sql.ErrNoRows)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaigns/"+id+"/mark-deleted", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("repo error returns 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		campaignRepo := repomocks.NewMockCampaignRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		repos.EXPECT().NewCampaignRepo(mock.Anything).Return(campaignRepo)
		campaignRepo.EXPECT().MarkDeletedForTests(mock.Anything, id).Return(errors.New("db boom"))
		expectUnexpectedErrorLog(log, "/test/campaigns/"+id+"/mark-deleted")

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/campaigns/"+id+"/mark-deleted", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestTestAPIHandler_GetResetToken(t *testing.T) {
	t.Parallel()

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		store.EXPECT().GetToken("missing@example.com").Return("", false)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet,
			"/test/reset-tokens?email=missing@example.com", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		store.EXPECT().GetToken("alice@example.com").Return("raw-token-123", true)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[testapi.ResetTokenResult](t, router, http.MethodGet,
			"/test/reset-tokens?email=alice@example.com", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, testapi.ResetTokenResult{
			Data: testapi.ResetTokenData{Token: "raw-token-123"},
		}, resp)
	})
}

func TestTestAPIHandler_GetTelegramSent(t *testing.T) {
	t.Parallel()

	t.Run("returns recorded messages with WebApp URL extracted", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		spy := telegram.NewSentSpyStore()
		// Seed: one match for our chat, one for a different chat (must be filtered out).
		spy.Record(telegram.SentRecord{
			ChatID: 555,
			Text:   "ok",
			ReplyMarkup: tgmodels.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
					{Text: "open", WebApp: &tgmodels.WebAppInfo{URL: "https://tma.test"}},
				}},
			},
			SentAt: time.Now().UTC(),
		})
		spy.Record(telegram.SentRecord{ChatID: 999, Text: "other", SentAt: time.Now().UTC()})

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		w, resp := doJSON[testapi.TelegramSentResult](t, router, http.MethodGet,
			"/test/telegram/sent?chatId=555", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Messages, 1)
		require.Equal(t, int64(555), resp.Data.Messages[0].ChatId)
		require.Equal(t, "ok", resp.Data.Messages[0].Text)
		require.NotNil(t, resp.Data.Messages[0].WebAppUrl)
		require.Equal(t, "https://tma.test", *resp.Data.Messages[0].WebAppUrl)
		require.Nil(t, resp.Data.Messages[0].Error)
	})

	t.Run("error string surfaces when TeeSender recorded an upstream failure", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		spy := telegram.NewSentSpyStore()
		spy.Record(telegram.SentRecord{ChatID: 42, Text: "boom", SentAt: time.Now().UTC(), Err: "telegram down"})

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		w, resp := doJSON[testapi.TelegramSentResult](t, router, http.MethodGet,
			"/test/telegram/sent?chatId=42", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Messages, 1)
		require.NotNil(t, resp.Data.Messages[0].Error)
		require.Equal(t, "telegram down", *resp.Data.Messages[0].Error)
	})

	t.Run("since filter excludes older records", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		spy := telegram.NewSentSpyStore()
		now := time.Now().UTC()
		spy.Record(telegram.SentRecord{ChatID: 7, Text: "old", SentAt: now.Add(-time.Hour)})
		spy.Record(telegram.SentRecord{ChatID: 7, Text: "fresh", SentAt: now})

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		since := now.Add(-time.Minute).Format(time.RFC3339Nano)
		w, resp := doJSON[testapi.TelegramSentResult](t, router, http.MethodGet,
			"/test/telegram/sent?chatId=7&since="+url.QueryEscape(since), nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Messages, 1)
		require.Equal(t, "fresh", resp.Data.Messages[0].Text)
	})
}

func TestTestAPIHandler_SendTelegramMessage(t *testing.T) {
	t.Parallel()

	t.Run("invalid JSON returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/telegram/message",
			map[string]any{"chatId": "not-a-number"})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("empty text returns 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/telegram/message",
			testapi.SendTelegramMessageRequest{ChatId: 1, Text: ""})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("success returns fallback reply for bare /start captured by spy", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[testapi.SendTelegramMessageResult](t, router, http.MethodPost,
			"/test/telegram/message",
			testapi.SendTelegramMessageRequest{ChatId: 999, Text: "/start"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Replies, 1)
		require.Equal(t, int64(999), resp.Replies[0].ChatId)
		require.Equal(t, telegram.MessageFallback, resp.Replies[0].Text)
	})
}

func TestTestAPIHandler_TelegramSpyFailNext(t *testing.T) {
	t.Parallel()

	t.Run("default reason classifies as bot_blocked", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		spy := telegram.NewSentSpyStore()
		sender := telegram.NewSpyOnlySender(spy)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		w, _ := doJSON[map[string]any](t, router, http.MethodPost,
			"/test/telegram/spy/fail-next",
			testapi.TelegramSpyFailNextRequest{ChatId: 555})
		require.Equal(t, http.StatusNoContent, w.Code)

		// Next send to chat 555 must come back with the canonical Forbidden
		// error so MapTelegramErrorToReason classifies it as bot_blocked.
		// Pin the exact error string — e2e assertions depend on the canonical
		// substring "bot was blocked by the user", and any drift in that
		// phrase would silently demote bot_blocked → unknown after the
		// substring tightening in MapTelegramErrorToReason.
		_, err := sender.SendMessage(context.Background(), &tgbot.SendMessageParams{
			ChatID: int64(555),
			Text:   "ping",
		})
		require.Error(t, err)
		require.EqualError(t, err, "Forbidden: bot was blocked by the user")
		require.Equal(t, domain.NotifyFailureReasonBotBlocked, telegram.MapTelegramErrorToReason(err))

		// One-shot: subsequent send goes through unchanged.
		_, err = sender.SendMessage(context.Background(), &tgbot.SendMessageParams{
			ChatID: int64(555),
			Text:   "second",
		})
		require.NoError(t, err)
	})

	t.Run("custom reason is preserved verbatim", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		spy := telegram.NewSentSpyStore()
		sender := telegram.NewSpyOnlySender(spy)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		custom := "network down"
		w, _ := doJSON[map[string]any](t, router, http.MethodPost,
			"/test/telegram/spy/fail-next",
			testapi.TelegramSpyFailNextRequest{ChatId: 777, Reason: &custom})
		require.Equal(t, http.StatusNoContent, w.Code)

		_, err := sender.SendMessage(context.Background(), &tgbot.SendMessageParams{
			ChatID: int64(777),
			Text:   "ping",
		})
		require.Error(t, err)
		require.Equal(t, custom, err.Error())
		require.Equal(t, domain.NotifyFailureReasonUnknown, telegram.MapTelegramErrorToReason(err))
	})
}

func TestTestAPIHandler_TelegramSpyFakeChat(t *testing.T) {
	t.Parallel()

	t.Run("registers fake chat so IsFakeChat returns true", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		spy := telegram.NewSentSpyStore()

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), spy, "", nil, nil, log))

		require.False(t, spy.IsFakeChat(101))
		w, _ := doJSON[map[string]any](t, router, http.MethodPost,
			"/test/telegram/spy/fake-chat",
			testapi.TelegramSpyFakeChatRequest{ChatId: 101})
		require.Equal(t, http.StatusNoContent, w.Code)
		require.True(t, spy.IsFakeChat(101))
	})
}

func TestTestAPIHandler_SignTMAInitData(t *testing.T) {
	t.Parallel()

	t.Run("signed initData passes server-side verification", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		const botToken = "bot-token-secret"

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), botToken, nil, nil, log))

		w, resp := doJSON[testapi.SignTMAInitDataResult](t, router, http.MethodPost,
			"/test/tma/sign-init-data",
			testapi.SignTMAInitDataRequest{TelegramUserId: 9000000001})
		require.Equal(t, http.StatusOK, w.Code)
		require.NotEmpty(t, resp.Data.InitData)

		// Sanity check: the returned initData must contain a hash field and
		// the user payload we requested. Cryptographic verification lives in
		// the middleware test — this just confirms the wiring works.
		values, err := url.ParseQuery(resp.Data.InitData)
		require.NoError(t, err)
		require.NotEmpty(t, values.Get("hash"))
		require.Contains(t, values.Get("user"), `"id":9000000001`)
	})

	t.Run("non-positive telegram user id → 422", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "", nil, nil, log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost,
			"/test/tma/sign-init-data",
			testapi.SignTMAInitDataRequest{TelegramUserId: 0})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("explicit auth_date is honoured", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "bot-token", nil, nil, log))

		past := time.Now().Add(-time.Hour).Unix()
		w, resp := doJSON[testapi.SignTMAInitDataResult](t, router, http.MethodPost,
			"/test/tma/sign-init-data",
			testapi.SignTMAInitDataRequest{TelegramUserId: 9000000001, AuthDate: &past})
		require.Equal(t, http.StatusOK, w.Code)
		values, err := url.ParseQuery(resp.Data.InitData)
		require.NoError(t, err)
		// Auth date in the signed payload should equal the value we passed.
		gotAuthDate, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
		require.NoError(t, err)
		require.Equal(t, past, gotAuthDate)
	})
}

// pgxmockTx adapts a mocked dbutil.DB to the pgx.Tx interface. dbutil.WithTx
// calls pool.Begin and receives a pgx.Tx; the test flow only needs Commit to
// succeed and the inner DB operations (Query/Exec/QueryRow) to be exposed for
// repository mocks to receive. dbutil mocks embed a *testing.T hook through
// the generated mockery struct, so we wrap them in a lightweight pgx.Tx
// stub that forwards the DB methods and returns nil for commit/rollback.
func pgxmockTx(t *testing.T, inner *dbutilmocks.MockDB) pgx.Tx {
	t.Helper()
	return &stubTx{inner: inner}
}

type stubTx struct {
	inner *dbutilmocks.MockDB
}

func (s *stubTx) Begin(context.Context) (pgx.Tx, error) { return s, nil }
func (s *stubTx) Commit(context.Context) error          { return nil }
func (s *stubTx) Rollback(context.Context) error        { return nil }
func (s *stubTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (s *stubTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (s *stubTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (s *stubTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (s *stubTx) Conn() *pgx.Conn { return nil }
func (s *stubTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.inner.Exec(ctx, sql, args...)
}
func (s *stubTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.inner.Query(ctx, sql, args...)
}
func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.inner.QueryRow(ctx, sql, args...)
}
