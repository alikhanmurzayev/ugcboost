package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TrustMeWebhookEnvVar — env var carrying the staging/prod static webhook
// token. Локально и в isolated CI env не выставлен — fallback берётся из
// `TrustMeWebhookE2EToken` ниже. Same env var name backend reads at startup,
// чтобы test runner и backend всегда показывали одинаковое значение.
const TrustMeWebhookEnvVar = "TRUSTME_WEBHOOK_TOKEN"

// TrustMeWebhookE2EToken — хардкод-токен для isolated/local E2E (одно и
// то же значение, что и backend читает из `backend/.env`/`backend/.env.ci`).
// Не использовать на staging/prod — там через `TrustMeWebhookEnvVar`
// прокидывается реальный секрет.
const TrustMeWebhookE2EToken = "local-dev-trustme-webhook-token"

// TrustMeWebhookToken возвращает токен для подписи webhook-payload'ов в
// тестах. Сперва смотрит env (staging/prod CI задаёт его через
// GitHub-secret), при пустом env — fallback на локальный хардкод. Backend
// в любой среде сравнивает через subtle.ConstantTimeCompare — главное,
// чтобы test runner и backend читали одно и то же.
func TrustMeWebhookToken(t *testing.T) string {
	t.Helper()
	if tok := os.Getenv(TrustMeWebhookEnvVar); tok != "" {
		return tok
	}
	return TrustMeWebhookE2EToken
}

// TrustMeWebhookPayload mirrors backend/api openapi.yaml TrustMeWebhookRequest
// в test-side формате — повторяем wire-format wholesale, чтобы тесты не
// тянули сгенерированный apiclient (webhook public, нет helpers).
type TrustMeWebhookPayload struct {
	ContractID  string  `json:"contract_id"`
	Status      int     `json:"status"`
	Client      *string `json:"client,omitempty"`
	ContractURL *string `json:"contract_url,omitempty"`
}

// TrustMeWebhookSignedPayload собирает payload happy-signed (status=3) с
// PII-полями (`client`, `contract_url`), которые backend по контракту
// игнорирует.
func TrustMeWebhookSignedPayload(contractID string) TrustMeWebhookPayload {
	phone := "+77071234567"
	url := "https://tct.kz/uploader/" + contractID
	return TrustMeWebhookPayload{
		ContractID:  contractID,
		Status:      3,
		Client:      &phone,
		ContractURL: &url,
	}
}

// TrustMeWebhookDeclinedPayload — happy-declined (status=9).
func TrustMeWebhookDeclinedPayload(contractID string) TrustMeWebhookPayload {
	phone := "+77071234567"
	url := "https://tct.kz/uploader/" + contractID
	return TrustMeWebhookPayload{
		ContractID:  contractID,
		Status:      9,
		Client:      &phone,
		ContractURL: &url,
	}
}

// TrustMeWebhookStatusPayload — generic helper for arbitrary status.
func TrustMeWebhookStatusPayload(contractID string, status int) TrustMeWebhookPayload {
	return TrustMeWebhookPayload{ContractID: contractID, Status: status}
}

// PostTrustMeWebhook отправляет payload на POST /trustme/webhook с
// заголовком `Authorization: Bearer <token>` — формат, в котором реально
// шлёт TrustMe (blueprint неточен про raw token). Empty token → header
// не выставляется, чтоб тестировать missing-token сценарий. Возвращает
// HTTP статус и тело ответа. Caller сам решает, как трактовать ответ —
// webhook эндпоинт public, без apiclient-обёртки.
func PostTrustMeWebhook(t *testing.T, payload TrustMeWebhookPayload, token string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		BaseURL+"/trustme/webhook", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := HTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, respBody
}
