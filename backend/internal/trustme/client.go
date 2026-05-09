// Package trustme integrates the TrustMe e-signature service. The package
// exposes a small Client interface — outbox-worker (chunk 16) calls SendToSign
// + SearchContractByAdditionalInfo, webhook handler (chunk 17) will call
// DownloadContractFile. Two implementations land here: RealClient (HTTP) for
// production, SpyOnlyClient (in-memory) for local + staging + tests. Per
// Decision #17 of intent-trustme-contract-v2: TrustMe has no sandbox, so a
// Tee implementation that records-while-forwarding-real makes no sense.
package trustme

import (
	"context"
	"errors"
)

// Requisite — каждая сторона договора (кроме инициатора, реквизиты которого
// зашиты в токен). FIO/IIN_BIN/PhoneNumber обязательны per blueprint;
// CompanyName опциональный.
type Requisite struct {
	CompanyName string
	FIO         string
	IINBIN      string
	PhoneNumber string
}

// SendToSignInput — параметры одного вызова SendToSignBase64FileExt/pdf.
// PDFBase64 — clean base64 без data URI префикса (per blueprint).
// AdditionalInfo несём contracts.id — ключ для поиска через
// SearchContractByAdditionalInfo при Phase 0 recovery.
type SendToSignInput struct {
	PDFBase64      string
	AdditionalInfo string
	ContractName   string
	Requisites     []Requisite
}

// SendToSignResult — выжимка из ответа TrustMe. document_id попадает в
// contracts.trustme_document_id, short_url — в contracts.trustme_short_url
// (и в бот-уведомление как ссылка на договор).
type SendToSignResult struct {
	DocumentID string
	ShortURL   string
	FileName   string
}

// SearchContractResult — наш orphan, обнаруженный в TrustMe при Phase 0
// recovery. Если результат пустой (nil) — TrustMe не знает наш AdditionalInfo,
// надо перепосылать. ContractStatus — целое из таблицы статусов TrustMe (0..9).
type SearchContractResult struct {
	DocumentID     string
	ShortURL       string
	ContractStatus int
}

// ErrTrustMeNotFound возвращается клиентом, когда search/Contracts не нашёл
// ни одного документа с заданным additionalInfo. Используется в Phase 0
// recovery вместо nil-результата для явного branch'а в worker'е.
var ErrTrustMeNotFound = errors.New("trustme: contract not found")

// Error — типизированная ошибка TrustMe API: API вернул status=Error с
// каким-то errorText кодом. Code — это сам код («1219», «1208»), Message
// — formatted string с расшифровкой через FormatErrorText. Используется
// outbox-worker'ом, чтобы записать last_error_code в БД при retry-failures
// без парсинга текста ошибки.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Client — minimal subset поверхности TrustMe API, нужный outbox-worker'у
// и webhook-handler'у. Реализаций две: RealClient (HTTP) и SpyOnlyClient
// (детерминированный in-memory). Клиент сам делает rate-limit (4 RPS per
// blueprint), обёртку и вычитку ответов.
type Client interface {
	// SendToSign отправляет PDF в TrustMe и возвращает выданный документу
	// trustme_document_id и short_url. Per blueprint: token в Authorization
	// header без Bearer, тело — multipart/form-data с FileBase64 + details
	// (JSON) + contract_name.
	SendToSign(ctx context.Context, in SendToSignInput) (*SendToSignResult, error)

	// SearchContractByAdditionalInfo выполняет POST /search/Contracts с
	// фильтром по additionalInfo. Возвращает первый совпавший документ или
	// ErrTrustMeNotFound, если TrustMe не знает наш additionalInfo.
	SearchContractByAdditionalInfo(ctx context.Context, additionalInfo string) (*SearchContractResult, error)

	// DownloadContractFile возвращает signed PDF — для chunk 17 webhook
	// status=3. Тут метод закреплён в интерфейсе на этапе chunk 16, чтобы
	// chunk 17 не правил Client.
	DownloadContractFile(ctx context.Context, documentID string) ([]byte, error)
}
