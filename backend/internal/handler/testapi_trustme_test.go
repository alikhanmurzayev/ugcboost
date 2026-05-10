package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	hmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport"
	trustmeportmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// trustMeRig — handler-router + все mockery-моки. trustMeRunner / trustMeSpy
// можно передавать nil, чтобы воспроизвести prod-конфигурацию (test endpoints
// disabled).
type trustMeRig struct {
	router    http.Handler
	runner    *trustmeportmocks.MockOutboxRunner
	spy       *trustmeportmocks.MockSpyStore
	logger    *logmocks.MockLogger
	hasRunner bool
	hasSpy    bool
}

func newTrustMeRig(t *testing.T, withRunner, withSpy bool) *trustMeRig {
	t.Helper()
	auth := hmocks.NewMockTestAPIAuthService(t)
	pool := mocks.NewMockPool(t)
	repos := hmocks.NewMockTestAPICleanupRepoFactory(t)
	store := hmocks.NewMockTokenStore(t)
	log := logmocks.NewMockLogger(t)

	rig := &trustMeRig{
		logger:    log,
		hasRunner: withRunner,
		hasSpy:    withSpy,
	}
	var runner trustmeport.OutboxRunner
	if withRunner {
		rig.runner = trustmeportmocks.NewMockOutboxRunner(t)
		runner = rig.runner
	}
	var spy trustmeport.SpyStore
	if withSpy {
		rig.spy = trustmeportmocks.NewMockSpyStore(t)
		spy = rig.spy
	}
	rig.router = newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store,
		telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "",
		runner, spy, log))
	return rig
}

func TestTestAPIHandler_TrustMeRunOutboxOnce(t *testing.T) {
	t.Parallel()

	t.Run("runs and returns 204", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, true)
		var captured context.Context
		rig.runner.EXPECT().RunOnce(mock.Anything).Run(func(ctx context.Context) {
			captured = ctx
		}).Once()

		w, _ := doJSON[any](t, rig.router, http.MethodPost, "/test/trustme/run-outbox-once", nil)

		require.Equal(t, http.StatusNoContent, w.Code)
		require.NotNil(t, captured, "RunOnce must receive non-nil ctx propagated from request")
	})

	t.Run("nil runner returns 404", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, false, false)

		w, resp := doJSON[api.ErrorResponse](t, rig.router, http.MethodPost, "/test/trustme/run-outbox-once", nil)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})
}

func TestTestAPIHandler_TrustMeSpyList(t *testing.T) {
	t.Parallel()

	t.Run("returns records as typed schema", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
		rig := newTrustMeRig(t, true, true)
		rig.spy.EXPECT().List().Return([]trustmeport.SentRecord{{
			DocumentID:     "doc-1",
			ShortURL:       "https://t.tct.kz/uploader/doc-1",
			AdditionalInfo: "ct-1",
			ContractName:   "Договор UGC",
			NumberDial:     "UGC-42",
			FIO:            "Иванов Иван Иванович",
			IIN:            "880101300123",
			Phone:          "+77071234567",
			PDFSha256:      "deadbeef00000000000000000000000000000000000000000000000000000000",
			SentAt:         now,
		}}).Once()

		w, resp := doJSON[testapi.TrustMeSpyListResult](t, rig.router, http.MethodGet, "/test/trustme/spy-list", nil)

		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Items, 1)
		got := resp.Data.Items[0]
		require.NotNil(t, got.DocumentId)
		require.Equal(t, "doc-1", *got.DocumentId)
		require.NotNil(t, got.ShortUrl)
		require.Equal(t, "https://t.tct.kz/uploader/doc-1", *got.ShortUrl)
		require.Equal(t, "ct-1", got.AdditionalInfo)
		require.Equal(t, "Договор UGC", got.ContractName)
		require.Equal(t, "UGC-42", got.NumberDial)
		require.Equal(t, "Иванов Иван Иванович", got.Fio)
		require.Equal(t, "880101300123", got.Iin)
		require.Equal(t, "+77071234567", got.Phone)
		require.Equal(t, "deadbeef00000000000000000000000000000000000000000000000000000000", got.PdfSha256)
		require.True(t, got.SentAt.Equal(now))
		require.Nil(t, got.Err)
	})

	t.Run("nil spy returns 404", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, false)

		w, resp := doJSON[api.ErrorResponse](t, rig.router, http.MethodGet, "/test/trustme/spy-list", nil)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})
}

func TestTestAPIHandler_TrustMeSpyClear(t *testing.T) {
	t.Parallel()

	t.Run("clears and returns 204", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, true)
		rig.spy.EXPECT().Clear().Once()

		w, _ := doJSON[any](t, rig.router, http.MethodPost, "/test/trustme/spy-clear", nil)

		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("nil spy returns 404", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, false)

		w, resp := doJSON[api.ErrorResponse](t, rig.router, http.MethodPost, "/test/trustme/spy-clear", nil)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})
}

func TestTestAPIHandler_TrustMeSpyFailNext(t *testing.T) {
	t.Parallel()
	rig := newTrustMeRig(t, true, true)
	rig.spy.EXPECT().RegisterFailNext("880101300123", "boom", 2).Once()

	w, _ := doJSON[any](t, rig.router, http.MethodPost, "/test/trustme/spy-fail-next",
		testapi.TrustMeSpyFailNextRequest{
			Iin:    "880101300123",
			Reason: pointerStr("boom"),
			Count:  pointerInt(2),
		})

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestTestAPIHandler_TrustMeSpyRegisterDocument(t *testing.T) {
	t.Parallel()

	t.Run("default short URL synthesized from documentId", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, true)
		rig.spy.EXPECT().RegisterDocument("ct-1", "doc-X",
			"https://test.trustme.kz/uploader/doc-X", 2).Once()

		w, _ := doJSON[any](t, rig.router, http.MethodPost, "/test/trustme/spy-register-document",
			testapi.TrustMeSpyRegisterDocumentRequest{
				AdditionalInfo: "ct-1",
				DocumentId:     "doc-X",
				ContractStatus: pointerInt(2),
			})

		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("custom short URL passed through", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, true)
		rig.spy.EXPECT().RegisterDocument("ct-1", "doc-X",
			"https://custom/doc-X", 0).Once()

		w, _ := doJSON[any](t, rig.router, http.MethodPost, "/test/trustme/spy-register-document",
			testapi.TrustMeSpyRegisterDocumentRequest{
				AdditionalInfo: "ct-1",
				DocumentId:     "doc-X",
				ShortUrl:       pointerStr("https://custom/doc-X"),
			})

		require.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("nil spy returns 404", func(t *testing.T) {
		t.Parallel()
		rig := newTrustMeRig(t, true, false)

		w, resp := doJSON[api.ErrorResponse](t, rig.router, http.MethodPost, "/test/trustme/spy-register-document",
			testapi.TrustMeSpyRegisterDocumentRequest{
				AdditionalInfo: "ct-1",
				DocumentId:     "doc-X",
			})

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeNotFound, resp.Error.Code)
	})
}

func pointerStr(s string) *string { return &s }
func pointerInt(i int) *int       { return &i }
