package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }
func newGetRequest(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }

func TestServer_ListAuditLogs(t *testing.T) {
	t.Parallel()

	t.Run("forbidden for manager", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListAuditLogs(mock.Anything).Return(domain.ErrForbidden)

		router := newTestRouter(t, NewServer(nil, nil, authz, nil, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/audit-logs", nil)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, domain.CodeForbidden, resp.Error.Code)
	})

	t.Run("admin success no filters full equality", func(t *testing.T) {
		t.Parallel()
		created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListAuditLogs(mock.Anything).Return(nil)
		audit := mocks.NewMockAuditLogService(t)
		audit.EXPECT().List(mock.Anything, domain.AuditFilter{}, 1, 20).
			Return([]*domain.AuditLog{
				{
					ID: "al-1", ActorID: strptr("u-1"), ActorRole: "admin", Action: "login",
					EntityType: "user", EntityID: strptr("u-1"),
					IPAddress: "127.0.0.1", CreatedAt: created,
				},
			}, int64(1), nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, audit, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.AuditLogsResult](t, router, http.MethodGet, "/audit-logs", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.AuditLogsResult{
			Data: api.ListAuditLogsData{
				Logs: []api.AuditLogEntry{
					{
						Id: "al-1", ActorId: "u-1", ActorRole: "admin", Action: "login",
						EntityType: "user", EntityId: strptr("u-1"),
						OldValue: nil, NewValue: nil,
						IpAddress: "127.0.0.1", CreatedAt: created,
					},
				},
				Page: 1, PerPage: 20, Total: 1,
			},
		}, resp)
	})

	t.Run("admin success all filters with JSON payloads", func(t *testing.T) {
		t.Parallel()
		dateFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		dateTo := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
		created := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
		newPayload := json.RawMessage(`{"name":"Acme"}`)
		oldPayload := json.RawMessage(`{"name":"Old"}`)

		expectedFilter := domain.AuditFilter{
			ActorID:    "u-1",
			EntityType: "brand",
			EntityID:   "e-1",
			Action:     "brand_update",
			DateFrom:   &dateFrom,
			DateTo:     &dateTo,
		}

		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListAuditLogs(mock.Anything).Return(nil)
		audit := mocks.NewMockAuditLogService(t)
		audit.EXPECT().List(mock.Anything, expectedFilter, 2, 50).
			Return([]*domain.AuditLog{
				{
					ID: "al-2", ActorID: strptr("u-1"), ActorRole: "admin", Action: "brand_update",
					EntityType: "brand", EntityID: strptr("e-1"),
					OldValue: oldPayload, NewValue: newPayload,
					IPAddress: "127.0.0.1", CreatedAt: created,
				},
			}, int64(1), nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, audit, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))
		url := "/audit-logs?actor_id=u-1&entity_type=brand&entity_id=e-1&action=brand_update" +
			"&date_from=2026-01-01T00:00:00Z&date_to=2026-12-31T23:59:59Z&page=2&per_page=50"
		w, resp := doJSON[api.AuditLogsResult](t, router, http.MethodGet, url, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.AuditLogsResult{
			Data: api.ListAuditLogsData{
				Logs: []api.AuditLogEntry{
					{
						Id: "al-2", ActorId: "u-1", ActorRole: "admin", Action: "brand_update",
						EntityType: "brand", EntityId: strptr("e-1"),
						OldValue: map[string]any{"name": "Old"},
						NewValue: map[string]any{"name": "Acme"},
						IpAddress: "127.0.0.1", CreatedAt: created,
					},
				},
				Page: 2, PerPage: 50, Total: 1,
			},
		}, resp)
	})

	t.Run("empty result", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListAuditLogs(mock.Anything).Return(nil)
		audit := mocks.NewMockAuditLogService(t)
		audit.EXPECT().List(mock.Anything, domain.AuditFilter{}, 1, 20).
			Return([]*domain.AuditLog{}, int64(0), nil)

		router := newTestRouter(t, NewServer(nil, nil, authz, audit, nil, ServerConfig{Version: "test-version"},logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.AuditLogsResult](t, router, http.MethodGet, "/audit-logs", nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, api.AuditLogsResult{
			Data: api.ListAuditLogsData{
				Logs:    []api.AuditLogEntry{},
				Page:    1,
				PerPage: 20,
				Total:   0,
			},
		}, resp)
	})

	t.Run("service error returns 500", func(t *testing.T) {
		t.Parallel()
		authz := mocks.NewMockAuthzService(t)
		authz.EXPECT().CanListAuditLogs(mock.Anything).Return(nil)
		audit := mocks.NewMockAuditLogService(t)
		audit.EXPECT().List(mock.Anything, domain.AuditFilter{}, 1, 20).
			Return(nil, int64(0), errors.New("db error"))
		log := logmocks.NewMockLogger(t)
		expectHandlerUnexpectedErrorLog(log, "/audit-logs")

		router := newTestRouter(t, NewServer(nil, nil, authz, audit, nil, ServerConfig{Version: "test-version"},log))
		w, _ := doJSON[api.ErrorResponse](t, router, http.MethodGet, "/audit-logs", nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleParamError(t *testing.T) {
	t.Parallel()

	w := newRecorder()
	r := newGetRequest("/x")
	HandleParamError(logmocks.NewMockLogger(t))(w, r, errors.New("invalid date"))

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp api.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, domain.CodeValidation, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "invalid date")
}

func newServerForRawJSON(log *logmocks.MockLogger) *Server {
	return NewServer(nil, nil, nil, nil, nil, ServerConfig{Version: "test-version"},log)
}

func TestRawJSONToAny(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		s := newServerForRawJSON(logmocks.NewMockLogger(t))
		require.Nil(t, s.rawJSONToAny(context.Background(), "al-1", nil))
	})

	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		s := newServerForRawJSON(logmocks.NewMockLogger(t))
		require.Nil(t, s.rawJSONToAny(context.Background(), "al-1", []byte{}))
	})

	t.Run("valid JSON object is decoded", func(t *testing.T) {
		t.Parallel()
		s := newServerForRawJSON(logmocks.NewMockLogger(t))
		got := s.rawJSONToAny(context.Background(), "al-1", []byte(`{"name":"Acme","count":3}`))
		require.Equal(t, map[string]any{"name": "Acme", "count": float64(3)}, got)
	})

	t.Run("valid JSON array is decoded", func(t *testing.T) {
		t.Parallel()
		s := newServerForRawJSON(logmocks.NewMockLogger(t))
		got := s.rawJSONToAny(context.Background(), "al-1", []byte(`["a","b"]`))
		require.Equal(t, []any{"a", "b"}, got)
	})

	t.Run("invalid JSON logs error and returns nil", func(t *testing.T) {
		t.Parallel()
		log := logmocks.NewMockLogger(t)
		log.EXPECT().Error(mock.Anything, "failed to unmarshal audit log value", mock.MatchedBy(func(args []any) bool {
			return len(args) == 4 && args[0] == "error" && args[2] == "auditLogID" && args[3] == "al-1"
		})).Once()

		s := newServerForRawJSON(log)
		require.Nil(t, s.rawJSONToAny(context.Background(), "al-1", []byte(`not json`)))
	})
}

func strptr(s string) *string { return &s }
