package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

const trustMeWebhookPath = "/trustme/webhook"

func trustMeWebhookRouter(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	api.HandlerWithOptions(NewStrictAPIHandler(s), api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
	})
	return r
}

func serverWithTrustMeWebhook(t *testing.T, svc TrustMeWebhookService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, svc, nil, ServerConfig{Version: "test-version"}, log)
}

func TestServer_TrustMeWebhook(t *testing.T) {
	t.Parallel()

	t.Run("happy path: forwards parsed event to service and returns 200 {}", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)
		var captured domain.TrustMeWebhookEvent
		svc.EXPECT().HandleEvent(mock.Anything, mock.Anything).
			Run(func(_ context.Context, ev domain.TrustMeWebhookEvent) {
				captured = ev
			}).Return(nil)

		log := logmocks.NewMockLogger(t)
		w, _ := doJSON[map[string]any](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{
				ContractId:  "doc-xyz",
				Status:      3,
				Client:      pointer.ToString("+77071234567"),
				ContractUrl: pointer.ToString("https://tct.kz/uploader/doc-xyz"),
			},
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
		// Captured-input — handler должен прокинуть contract_id и status
		// в domain DTO. PII-поля client/contract_url игнорируются (не
		// доходят до domain).
		require.Equal(t, "doc-xyz", captured.ContractID)
		require.Equal(t, 3, captured.Status)
	})

	t.Run("unknown document returns 404 with CONTRACT_WEBHOOK_UNKNOWN_DOCUMENT", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)
		svc.EXPECT().HandleEvent(mock.Anything, mock.Anything).Return(domain.ErrContractWebhookUnknownDocument)

		log := logmocks.NewMockLogger(t)
		w, body := doJSON[api.ErrorResponse](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{ContractId: "doc-missing", Status: 3},
		)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeContractWebhookUnknownDocument, body.Error.Code)
	})

	t.Run("unknown subject returns 422 with CONTRACT_WEBHOOK_UNKNOWN_SUBJECT", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)
		svc.EXPECT().HandleEvent(mock.Anything, mock.Anything).Return(domain.ErrContractWebhookUnknownSubject)

		log := logmocks.NewMockLogger(t)
		w, body := doJSON[api.ErrorResponse](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{ContractId: "doc-1", Status: 3},
		)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeContractWebhookUnknownSubject, body.Error.Code)
	})

	t.Run("status out of 0..9 rejected before service call", func(t *testing.T) {
		t.Parallel()
		// OpenAPI validation не активна в strict-server без параметра — domain
		// валидатор ловит status вне 0..9 раньше service.
		svc := mocks.NewMockTrustMeWebhookService(t)

		log := logmocks.NewMockLogger(t)
		w, body := doJSON[api.ErrorResponse](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{ContractId: "doc-1", Status: 15},
		)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeContractWebhookInvalidStatus, body.Error.Code)
	})

	t.Run("empty contract_id rejected as 404 (anti-fingerprint with unknown document)", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)

		log := logmocks.NewMockLogger(t)
		w, body := doJSON[api.ErrorResponse](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{ContractId: "", Status: 3},
		)

		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, domain.CodeContractWebhookUnknownDocument, body.Error.Code)
	})

	t.Run("invalid json body returns 422 via strict-server request error handler", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)
		log := logmocks.NewMockLogger(t)

		req := httptest.NewRequest(http.MethodPost, trustMeWebhookPath, bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)).ServeHTTP(w, req)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("downstream service error mapped via respondError default branch", func(t *testing.T) {
		t.Parallel()
		svc := mocks.NewMockTrustMeWebhookService(t)
		svc.EXPECT().HandleEvent(mock.Anything, mock.Anything).Return(errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, trustMeWebhookPath)

		w, body := doJSON[api.ErrorResponse](t, trustMeWebhookRouter(t, serverWithTrustMeWebhook(t, svc, log)),
			http.MethodPost, trustMeWebhookPath,
			api.TrustMeWebhookRequest{ContractId: "doc-1", Status: 3},
		)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, domain.CodeInternal, body.Error.Code)
	})
}
