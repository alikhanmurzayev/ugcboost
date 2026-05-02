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

const sendPulseWebhookPath = "/webhooks/sendpulse/instagram"

func sendPulseRouter(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	api.HandlerWithOptions(NewStrictAPIHandler(s), api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
	})
	return r
}

func TestServer_SendPulseInstagramWebhook(t *testing.T) {
	t.Parallel()

	t.Run("forwards parsed code and username to service on happy path", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		var (
			capturedCode   string
			capturedHandle string
		)
		creator.EXPECT().VerifyInstagramByCode(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, code, handle string) {
				capturedCode = code
				capturedHandle = handle
			}).
			Return(domain.VerifyInstagramStatusVerified, nil)

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Info(mock.Anything, "sendpulse webhook: instagram verified", mock.Anything).Maybe()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{
				Username:    "AIDANA",
				LastMessage: "Hi! Code UGC-123456",
				ContactId:   pointer.ToString("contact-42"),
			},
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
		require.Equal(t, "UGC-123456", capturedCode)
		require.Equal(t, "AIDANA", capturedHandle, "handler must forward username verbatim — service normalises")
	})

	t.Run("missing code in payload short-circuits without service call", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Debug(mock.Anything, "sendpulse webhook: code not found in message", mock.Anything).Once()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{Username: "aidana", LastMessage: "no code here"},
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
	})

	t.Run("noop status returns 200 with debug log", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyInstagramByCode(mock.Anything, "UGC-123456", "aidana").
			Return(domain.VerifyInstagramStatusNoop, nil)

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Debug(mock.Anything, "sendpulse webhook: already verified", mock.Anything).Once()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{Username: "aidana", LastMessage: "UGC-123456"},
		)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("not_found status returns 200 with warn log", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyInstagramByCode(mock.Anything, "UGC-999999", "aidana").
			Return(domain.VerifyInstagramStatusNotFound, nil)

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Warn(mock.Anything, "sendpulse webhook: no active application for code", mock.Anything).Once()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{Username: "aidana", LastMessage: "UGC-999999"},
		)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("no_ig_social status returns 200 with warn log", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyInstagramByCode(mock.Anything, "UGC-555555", "aidana").
			Return(domain.VerifyInstagramStatusNoIGSocial, nil)

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Warn(mock.Anything, "sendpulse webhook: application has no instagram social", mock.Anything).Once()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{Username: "aidana", LastMessage: "UGC-555555"},
		)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid request body is suppressed to 200 (anti-fingerprinting)", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		log := logmocks.NewMockLogger(t)

		req := httptest.NewRequest(http.MethodPost, sendPulseWebhookPath, bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		sendPulseRouter(t, serverWithCreator(t, creator, log)).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "spec freezes 200/401 only — request errors must not leak past auth")
		require.Equal(t, "{}\n", w.Body.String())
	})

	t.Run("service infrastructure error is suppressed to 200 (anti-fingerprinting)", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().VerifyInstagramByCode(mock.Anything, "UGC-123456", "aidana").
			Return(domain.VerifyInstagramStatus(""), errors.New("db down"))

		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "sendpulse webhook: suppressed downstream error", mock.Anything).Once()

		w, _ := doJSON[map[string]any](t, sendPulseRouter(t, serverWithCreator(t, creator, log)),
			http.MethodPost, sendPulseWebhookPath,
			api.SendPulseInstagramWebhookRequest{Username: "aidana", LastMessage: "UGC-123456"},
		)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "{}\n", w.Body.String())
	})
}
