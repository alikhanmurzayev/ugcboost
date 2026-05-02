package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
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
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			map[string]any{"type": "campaign", "id": "c-1"})
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

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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

		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/test/cleanup-entity",
			testapi.CleanupEntityRequest{Type: testapi.Brand, Id: "b-missing"})
		require.Equal(t, http.StatusNotFound, w.Code)
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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, resp := doJSON[testapi.ResetTokenResult](t, router, http.MethodGet,
			"/test/reset-tokens?email=alice@example.com", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, testapi.ResetTokenResult{
			Data: testapi.ResetTokenData{Token: "raw-token-123"},
		}, resp)
	})
}

func TestTestAPIHandler_GetCreatorApplicationVerificationCode(t *testing.T) {
	t.Parallel()

	appUUID := openapi_types.UUID{}
	require.NoError(t, appUUID.UnmarshalText([]byte("11111111-2222-3333-4444-555555555555")))

	t.Run("success returns the persisted code", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		creatorRepo := repomocks.NewMockCreatorApplicationRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		repos.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(creatorRepo)
		creatorRepo.EXPECT().GetByID(mock.Anything, appUUID.String()).
			Return(&repository.CreatorApplicationRow{ID: appUUID.String(), VerificationCode: "UGC-123456"}, nil)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, resp := doJSON[testapi.CreatorApplicationVerificationCodeResult](t, router, http.MethodGet,
			"/test/creator-applications/"+appUUID.String()+"/verification-code", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "UGC-123456", resp.Data.VerificationCode)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		creatorRepo := repomocks.NewMockCreatorApplicationRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		repos.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(creatorRepo)
		creatorRepo.EXPECT().GetByID(mock.Anything, appUUID.String()).
			Return(nil, sql.ErrNoRows)
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet,
			"/test/creator-applications/"+appUUID.String()+"/verification-code", nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("repo error surfaces as 500", func(t *testing.T) {
		t.Parallel()
		auth := mocks.NewMockTestAPIAuthService(t)
		repos := mocks.NewMockTestAPICleanupRepoFactory(t)
		pool := dbutilmocks.NewMockPool(t)
		creatorRepo := repomocks.NewMockCreatorApplicationRepo(t)
		store := mocks.NewMockTokenStore(t)
		log := logmocks.NewMockLogger(t)
		repos.EXPECT().NewCreatorApplicationRepo(mock.Anything).Return(creatorRepo)
		creatorRepo.EXPECT().GetByID(mock.Anything, appUUID.String()).
			Return(nil, errors.New("db down"))
		expectUnexpectedErrorLog(log, "/test/creator-applications/"+appUUID.String()+"/verification-code")
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet,
			"/test/creator-applications/"+appUUID.String()+"/verification-code", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

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
		router := newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store, telegram.NewHandler(nil, log), log))

		w, resp := doJSON[testapi.SendTelegramMessageResult](t, router, http.MethodPost,
			"/test/telegram/message",
			testapi.SendTelegramMessageRequest{ChatId: 999, Text: "/start"})
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Replies, 1)
		require.Equal(t, int64(999), resp.Replies[0].ChatId)
		require.Equal(t, telegram.MessageFallback, resp.Replies[0].Text)
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
