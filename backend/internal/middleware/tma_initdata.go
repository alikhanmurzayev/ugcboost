package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// TMA initData header contract — identical to admin Bearer scheme but with
// a different scheme word so the two middlewares cannot accidentally
// authenticate each other's traffic.
const (
	authSchemeTma         = "tma"
	tmaInitDataHashKey    = "hash"
	tmaInitDataAuthDate   = "auth_date"
	tmaInitDataUserKey    = "user"
	telegramSecretKeyData = "WebAppData"
)

// ContextKey values for TMA-resolved identity. Role re-uses
// middleware.ContextKeyRole so AuthzService can branch admin / creator on
// the same RoleFromContext helper.
const (
	ContextKeyTelegramUserID contextKey = "telegramUserID"
	ContextKeyCreatorID      contextKey = "creatorID"
)

// TMACreatorRepo is the subset of repository.CreatorRepo the middleware
// needs. Accept-interface convention — keeps the middleware testable
// without dragging the full creator repo surface in.
type TMACreatorRepo interface {
	GetByTelegramUserID(ctx context.Context, tgID int64) (*repository.CreatorRow, error)
}

// TMAInitDataConfig bundles the dependencies of TMAInitData. Keeping them
// in a struct keeps the middleware constructor short and lets future config
// (e.g. allowed bot ids when we run more than one bot) grow without a
// long positional signature.
type TMAInitDataConfig struct {
	BotToken    string
	TTL         time.Duration
	CreatorRepo TMACreatorRepo
	Logger      logger.Logger
}

// telegramUser is the JSON payload encoded into the `user` query parameter
// of Telegram WebApp initData. We only need the id; other fields stay out
// of stdout (security.md § PII в логах).
type telegramUser struct {
	ID int64 `json:"id"`
}

// TMAInitDataFromScopes returns middleware that authenticates the request
// only when the generated wrapper marked it with `api.TmaInitDataScopes`
// (i.e. the route was declared with `tmaInitData` security in openapi.yaml).
// Other routes flow through unchanged. On success it sets:
//
//   - ContextKeyTelegramUserID — the verified telegram user id (int64).
//   - ContextKeyCreatorID + ContextKeyRole — only when the creator exists.
//
// Identity validation is purely cryptographic: HMAC-SHA256 with key derived
// from the bot token plus an auth_date freshness window. Any failure
// surfaces as a generic 401 anti-fingerprint — the body never reveals
// which sub-check failed. Stdout debug logs may carry an abstract reason
// (`hmac_mismatch`, `auth_date_invalid`, `header_missing`,
// `creator_lookup_db_error`) but never the raw initData / telegram_user_id
// / secret_token.
func TMAInitDataFromScopes(cfg TMAInitDataConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Context().Value(api.TmaInitDataScopes) == nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx, ok := authenticateTmaInitData(r, cfg)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, api.ErrorResponse{
					Error: api.APIError{Code: domain.CodeUnauthorized, Message: "Не удалось подтвердить доступ"},
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// authenticateTmaInitData runs the full HMAC + freshness + creator-lookup
// pipeline and returns the enriched ctx. ok=false on any failure —
// caller emits a single generic 401, no leak of which step failed.
func authenticateTmaInitData(r *http.Request, cfg TMAInitDataConfig) (context.Context, bool) {
	ctx := r.Context()
	header := r.Header.Get(headerAuthorization)
	if header == "" {
		cfg.Logger.Debug(ctx, "tma_initdata: header_missing")
		return nil, false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], authSchemeTma) {
		cfg.Logger.Debug(ctx, "tma_initdata: header_scheme_invalid")
		return nil, false
	}

	values, err := url.ParseQuery(parts[1])
	if err != nil {
		cfg.Logger.Debug(ctx, "tma_initdata: header_parse_error")
		return nil, false
	}
	gotHash := values.Get(tmaInitDataHashKey)
	rawAuthDate := values.Get(tmaInitDataAuthDate)
	rawUser := values.Get(tmaInitDataUserKey)
	if gotHash == "" || rawAuthDate == "" || rawUser == "" {
		cfg.Logger.Debug(ctx, "tma_initdata: header_field_missing")
		return nil, false
	}

	authDateUnix, err := strconv.ParseInt(rawAuthDate, 10, 64)
	if err != nil {
		cfg.Logger.Debug(ctx, "tma_initdata: auth_date_parse_error")
		return nil, false
	}
	authDate := time.Unix(authDateUnix, 0)
	now := time.Now()
	if authDate.After(now) || now.Sub(authDate) > cfg.TTL {
		cfg.Logger.Debug(ctx, "tma_initdata: auth_date_invalid")
		return nil, false
	}

	if !verifyTmaHash(values, gotHash, cfg.BotToken) {
		cfg.Logger.Debug(ctx, "tma_initdata: hmac_mismatch")
		return nil, false
	}

	var user telegramUser
	if err := json.Unmarshal([]byte(rawUser), &user); err != nil || user.ID <= 0 {
		cfg.Logger.Debug(ctx, "tma_initdata: user_parse_error")
		return nil, false
	}

	ctx = context.WithValue(ctx, ContextKeyTelegramUserID, user.ID)
	creator, err := cfg.CreatorRepo.GetByTelegramUserID(ctx, user.ID)
	switch {
	case err == nil:
		ctx = context.WithValue(ctx, ContextKeyCreatorID, creator.ID)
		ctx = context.WithValue(ctx, ContextKeyRole, api.Creator)
	case errors.Is(err, sql.ErrNoRows):
		// Creator not registered in our DB — leave ctx with telegram_user_id
		// only. AuthzService surfaces 403 anti-fingerprint between this and
		// "creator not in campaign" downstream.
		cfg.Logger.Debug(ctx, "tma_initdata: creator_not_registered")
	default:
		cfg.Logger.Debug(ctx, "tma_initdata: creator_lookup_db_error")
		return nil, false
	}
	return ctx, true
}

// verifyTmaHash recomputes the HMAC over the data_check_string and
// constant-time compares it against the supplied `hash`. data_check_string
// is `key=value` pairs (excluding `hash`), sorted alphabetically by key,
// joined by `\n`. Secret key is `HMAC_SHA256(bot_token, "WebAppData")`.
func verifyTmaHash(values url.Values, gotHash, botToken string) bool {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == tmaInitDataHashKey {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(values.Get(k))
	}

	secretMac := hmac.New(sha256.New, []byte(telegramSecretKeyData))
	secretMac.Write([]byte(botToken))
	secretKey := secretMac.Sum(nil)

	checkMac := hmac.New(sha256.New, secretKey)
	checkMac.Write([]byte(buf.String()))
	expectedHex := hex.EncodeToString(checkMac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(expectedHex), []byte(gotHash)) == 1
}

// TelegramUserIDFromContext returns the verified Telegram user id stored by
// TMAInitData on success. Returns 0 when the ctx is not from a /tma/*
// request — caller treats 0 as "no TMA auth attempted".
func TelegramUserIDFromContext(ctx context.Context) int64 {
	v, _ := ctx.Value(ContextKeyTelegramUserID).(int64)
	return v
}

// CreatorIDFromContext returns the resolved creator id stored by
// TMAInitData on success. Returns "" when the telegram_user_id has no
// matching creator row — AuthzService surfaces 403 in that case.
func CreatorIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyCreatorID).(string)
	return v
}

// SignTMAInitDataForTests builds a valid initData query string for the
// supplied telegram user id and auth_date. Used by the test endpoint
// `/test/tma/sign-init-data` so e2e suites can mint legitimate initData
// without driving a real Telegram WebApp client. botToken is the same
// secret production HMAC validation uses; in tests it comes from
// cfg.TelegramBotToken just like in production.
func SignTMAInitDataForTests(botToken string, telegramUserID int64, authDate time.Time) string {
	user, _ := json.Marshal(telegramUser{ID: telegramUserID})
	values := url.Values{}
	values.Set(tmaInitDataAuthDate, strconv.FormatInt(authDate.Unix(), 10))
	values.Set(tmaInitDataUserKey, string(user))

	keys := []string{tmaInitDataAuthDate, tmaInitDataUserKey}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(values.Get(k))
	}

	secretMac := hmac.New(sha256.New, []byte(telegramSecretKeyData))
	secretMac.Write([]byte(botToken))
	secretKey := secretMac.Sum(nil)

	checkMac := hmac.New(sha256.New, secretKey)
	checkMac.Write([]byte(buf.String()))
	values.Set(tmaInitDataHashKey, hex.EncodeToString(checkMac.Sum(nil)))
	return values.Encode()
}
