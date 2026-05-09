package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	mwmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/middleware/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

const tmaTestBotToken = "test-bot-token-for-hmac"

// genericUnauthorizedBody is the canonical body the middleware writes on any
// 401. Anti-fingerprint requires every negative branch to be byte-equal —
// asserting the body in each scenario locks the contract.
const genericUnauthorizedBody = `{"error":{"code":"UNAUTHORIZED","message":"Не удалось подтвердить доступ"}}`

func newTMAInitDataConfig(t *testing.T, repo TMACreatorRepo) TMAInitDataConfig {
	t.Helper()
	log := logmocks.NewMockLogger(t)
	log.EXPECT().Debug(mock.Anything, mock.Anything).Maybe()
	return TMAInitDataConfig{
		BotToken:    tmaTestBotToken,
		TTL:         24 * time.Hour,
		CreatorRepo: repo,
		Logger:      log,
	}
}

// scopedRequest builds a request with the api.TmaInitDataScopes context value
// set — mimics what the generated wrapper does for /tma/* endpoints.
func scopedRequest(t *testing.T, header string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/tma/campaigns/abc/agree", nil)
	//nolint:staticcheck // SA1029: api.TmaInitDataScopes is the exact key the generated server wrapper sets; tests must use the same.
	r = r.WithContext(context.WithValue(r.Context(), api.TmaInitDataScopes, []string{}))
	if header != "" {
		r.Header.Set(headerAuthorization, header)
	}
	return r
}

func runMiddleware(t *testing.T, cfg TMAInitDataConfig, r *http.Request) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	rec := httptest.NewRecorder()
	var captured *http.Request
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.WriteHeader(http.StatusOK)
	})
	TMAInitDataFromScopes(cfg)(next).ServeHTTP(rec, r)
	return rec, captured
}

func TestTMAInitDataFromScopes(t *testing.T) {
	t.Parallel()

	t.Run("not scoped passes through without lookup", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		rec := httptest.NewRecorder()
		captured := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			captured = true
			w.WriteHeader(http.StatusOK)
		})
		r := httptest.NewRequest(http.MethodGet, "/some-other-endpoint", nil)
		TMAInitDataFromScopes(cfg)(next).ServeHTTP(rec, r)

		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, captured)
	})

	t.Run("happy path populates context with telegram_user_id, creator_id, role", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		repo.EXPECT().GetByTelegramUserID(mock.Anything, int64(9000000001)).
			Return(&repository.CreatorRow{ID: "creator-1", TelegramUserID: 9000000001}, nil)

		cfg := newTMAInitDataConfig(t, repo)
		initData := SignTMAInitDataForTests(tmaTestBotToken, 9000000001, time.Now())
		rec, captured := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))

		require.Equal(t, http.StatusOK, rec.Code)
		require.NotNil(t, captured)
		require.Equal(t, int64(9000000001), TelegramUserIDFromContext(captured.Context()))
		require.Equal(t, "creator-1", CreatorIDFromContext(captured.Context()))
		require.Equal(t, api.Creator, RoleFromContext(captured.Context()))
	})

	t.Run("creator not registered passes through with telegram_user_id only", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		repo.EXPECT().GetByTelegramUserID(mock.Anything, int64(9000000002)).
			Return(nil, sql.ErrNoRows)

		cfg := newTMAInitDataConfig(t, repo)
		initData := SignTMAInitDataForTests(tmaTestBotToken, 9000000002, time.Now())
		rec, captured := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, int64(9000000002), TelegramUserIDFromContext(captured.Context()))
		require.Empty(t, CreatorIDFromContext(captured.Context()))
		require.Empty(t, RoleFromContext(captured.Context()))
	})

	t.Run("database error surfaces as 401 generic (anti-fingerprint)", func(t *testing.T) {
		// We can't tell the caller "DB is sad" without leaking that the
		// initData itself was valid — a side-channel that lets an attacker
		// distinguish a forged signature from a real one.
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		repo.EXPECT().GetByTelegramUserID(mock.Anything, int64(9000000003)).
			Return(nil, errors.New("db down"))

		cfg := newTMAInitDataConfig(t, repo)
		initData := SignTMAInitDataForTests(tmaTestBotToken, 9000000003, time.Now())
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("header missing → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		rec, _ := runMiddleware(t, cfg, scopedRequest(t, ""))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("wrong scheme → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "Bearer something"))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("hmac mismatch → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		// Sign with a different bot token — middleware must reject.
		initData := SignTMAInitDataForTests("WRONG-BOT-TOKEN", 9000000001, time.Now())
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("auth_date expired → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)
		cfg.TTL = 60 * time.Second

		old := time.Now().Add(-time.Hour)
		initData := SignTMAInitDataForTests(tmaTestBotToken, 9000000001, old)
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("auth_date in future → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		future := time.Now().Add(10 * time.Minute)
		initData := SignTMAInitDataForTests(tmaTestBotToken, 9000000001, future)
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+initData))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("bad auth_date format → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		// Manually craft initData with non-numeric auth_date.
		header := "auth_date=not-a-number&user=" + `{"id":1}` + "&hash=deadbeef"
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+header))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("invalid user JSON → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		authDate := strconv.FormatInt(time.Now().Unix(), 10)
		header := "auth_date=" + authDate + "&user=not-json&hash=deadbeef"
		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma "+header))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})

	t.Run("required field missing → 401 generic", func(t *testing.T) {
		t.Parallel()
		repo := mwmocks.NewMockTMACreatorRepo(t)
		cfg := newTMAInitDataConfig(t, repo)

		rec, _ := runMiddleware(t, cfg, scopedRequest(t, "tma auth_date=123"))
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.JSONEq(t, genericUnauthorizedBody, rec.Body.String())
	})
}

func TestTelegramUserIDFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns zero when context is empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, int64(0), TelegramUserIDFromContext(context.Background()))
	})
}

func TestCreatorIDFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns empty string when context is empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", CreatorIDFromContext(context.Background()))
	})
}
