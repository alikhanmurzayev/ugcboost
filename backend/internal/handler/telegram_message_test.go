package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

func TestServer_ListTelegramMessages(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("limit too small returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=0", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("limit too large returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=101", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("invalid cursor returns 422", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5&cursor=not-a-base64", nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeValidation, resp.Error.Code)
	})

	t.Run("happy empty page", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)
		svc := mocks.NewMockTelegramMessageService(t)
		svc.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 5).
			Return(nil, nil, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, svc, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.TelegramMessagesResult](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, resp.Data.Items)
		require.Nil(t, resp.Data.NextCursor)
	})

	t.Run("happy page with rows and nextCursor", func(t *testing.T) {
		t.Parallel()
		t1 := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
		rowID, err := uuid.NewRandom()
		require.NoError(t, err)
		nextCursor := &domain.TelegramMessagesCursor{CreatedAt: t1, ID: rowID.String()}

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)
		svc := mocks.NewMockTelegramMessageService(t)
		svc.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 5).
			Return([]*repository.TelegramMessageRow{
				{
					ID:                rowID.String(),
					ChatID:            42,
					Direction:         domain.TelegramMessageDirectionInbound,
					Text:              "hi",
					TelegramMessageID: pointer.ToInt64(7),
					TelegramUsername:  pointer.ToString("aidana"),
					CreatedAt:         t1,
				},
			}, nextCursor, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, svc, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.TelegramMessagesResult](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Items, 1)
		require.Equal(t, rowID, resp.Data.Items[0].Id)
		require.Equal(t, int64(42), resp.Data.Items[0].ChatId)
		require.Equal(t, api.TelegramMessageDirection("inbound"), resp.Data.Items[0].Direction)
		require.Equal(t, "hi", resp.Data.Items[0].Text)
		require.Equal(t, pointer.ToInt64(7), resp.Data.Items[0].TelegramMessageId)
		require.Equal(t, pointer.ToString("aidana"), resp.Data.Items[0].TelegramUsername)
		require.Nil(t, resp.Data.Items[0].Status)
		require.Nil(t, resp.Data.Items[0].Error)
		require.NotNil(t, resp.Data.NextCursor)

		// Round-trip: encoded nextCursor decodes back to the original cursor.
		decoded, err := domain.DecodeTelegramMessagesCursor(*resp.Data.NextCursor)
		require.NoError(t, err)
		require.NotNil(t, decoded)
		require.Equal(t, rowID.String(), decoded.ID)
		require.True(t, t1.Equal(decoded.CreatedAt))
	})

	t.Run("valid cursor forwarded to service", func(t *testing.T) {
		t.Parallel()
		t1 := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		cur := domain.TelegramMessagesCursor{CreatedAt: t1, ID: "id-prev"}
		encoded, err := domain.EncodeTelegramMessagesCursor(cur)
		require.NoError(t, err)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)
		svc := mocks.NewMockTelegramMessageService(t)
		svc.EXPECT().ListByChat(mock.Anything, int64(42), mock.MatchedBy(func(c *domain.TelegramMessagesCursor) bool {
			return c != nil && c.ID == "id-prev" && c.CreatedAt.Equal(t1)
		}), 5).Return(nil, nil, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, svc, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, _ := doJSON[api.TelegramMessagesResult](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5&cursor="+encoded, nil)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("outbound failed row maps status and error", func(t *testing.T) {
		t.Parallel()
		t1 := time.Date(2026, 5, 13, 11, 0, 0, 0, time.UTC)
		rowID, err := uuid.NewRandom()
		require.NoError(t, err)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanReadTelegramMessages(mock.Anything).Return(nil)
		svc := mocks.NewMockTelegramMessageService(t)
		svc.EXPECT().ListByChat(mock.Anything, int64(42), (*domain.TelegramMessagesCursor)(nil), 5).
			Return([]*repository.TelegramMessageRow{
				{
					ID:        rowID.String(),
					ChatID:    42,
					Direction: domain.TelegramMessageDirectionOutbound,
					Text:      "hi",
					Status:    pointer.ToString(domain.TelegramMessageStatusFailed),
					Error:     pointer.ToString("bot blocked"),
					CreatedAt: t1,
				},
			}, nil, nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, nil, nil, nil, nil, nil, nil, svc, ServerConfig{Version: "test-version"}, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.TelegramMessagesResult](t, router, http.MethodGet, "/telegram-messages?chatId=42&limit=5", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Data.Items, 1)
		require.NotNil(t, resp.Data.Items[0].Status)
		require.Equal(t, api.TelegramMessageStatus("failed"), *resp.Data.Items[0].Status)
		require.Equal(t, pointer.ToString("bot blocked"), resp.Data.Items[0].Error)
	})
}
