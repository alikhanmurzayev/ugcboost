package domain

import (
	"errors"
	"strings"
)

// TrustMe webhook user-facing codes returned by POST /trustme/webhook when
// the request reaches the handler past middleware-auth. 401 на отсутствие/
// неверный токен пишется middleware'ом напрямую (anti-fingerprint), мимо
// respondError — поэтому отдельного кода для него нет.
const (
	// 404 — payload references a contract_id that we do not know.
	CodeContractWebhookUnknownDocument = "CONTRACT_WEBHOOK_UNKNOWN_DOCUMENT"
	// 422 — found contracts row but its subject_kind is not yet supported by
	// the dispatcher (defensive — only `campaign_creator` is wired today).
	CodeContractWebhookUnknownSubject = "CONTRACT_WEBHOOK_UNKNOWN_SUBJECT"
	// 422 — payload status outside the documented 0..9 range. Bypasses
	// OpenAPI validation only on raw HTTP probes.
	CodeContractWebhookInvalidStatus = "CONTRACT_WEBHOOK_INVALID_STATUS"
)

// Sentinel errors raised by the TrustMe webhook flow. Handler maps each via
// respondError into the codes above.
var (
	ErrContractWebhookUnknownDocument = errors.New("trustme webhook: unknown document")
	ErrContractWebhookUnknownSubject  = errors.New("trustme webhook: unknown subject_kind")
	ErrContractWebhookInvalidStatus   = errors.New("trustme webhook: invalid status code")
)

// TrustMeWebhookEvent — domain DTO для приёма webhook'а от TrustMe. Имена
// полей в payload TrustMe непоследовательны (`contract_id` здесь, при том
// что в /SendToSign и /search контракт идентифицируется как `document_id`/
// `id`); в нашей БД это всегда `contracts.trustme_document_id`. Service-
// слой принимает domain DTO, не api-type — handler конвертит до вызова.
type TrustMeWebhookEvent struct {
	// ContractID — идентификатор документа на стороне TrustMe (наш
	// `contracts.trustme_document_id`). Webhook payload-поле `contract_id`.
	ContractID string
	// Status — код статуса документа per blueprint (0..9).
	Status int
}

// NewTrustMeWebhookEvent валидирует payload-поля и собирает domain DTO.
// Возвращает ErrContractWebhookInvalidStatus, если status вне 0..9, или
// ValidationError с CodeContractWebhookUnknownDocument, если contract_id
// пустой после trim. PII-поля (`client`, `contract_url`) handler сюда не
// передаёт — они игнорируются на api-уровне.
func NewTrustMeWebhookEvent(contractID string, status int) (TrustMeWebhookEvent, error) {
	contractID = strings.TrimSpace(contractID)
	if contractID == "" {
		return TrustMeWebhookEvent{}, ErrContractWebhookUnknownDocument
	}
	if status < 0 || status > 9 {
		return TrustMeWebhookEvent{}, ErrContractWebhookInvalidStatus
	}
	return TrustMeWebhookEvent{
		ContractID: contractID,
		Status:     status,
	}, nil
}
