// Package trustme — клиент TrustMe e-signature API. RealClient (HTTP) для
// прода + SpyOnlyClient для local/staging/тестов; Tee нет, у TrustMe не
// существует sandbox (intent-v2 Decision #17).
package trustme

import (
	"context"
	"errors"
)

// Requisite — сторона договора (кроме инициатора, его реквизиты зашиты в
// токен). FIO/IIN_BIN/PhoneNumber обязательны per blueprint.
type Requisite struct {
	CompanyName string
	FIO         string
	IINBIN      string
	PhoneNumber string
}

// SendToSignInput — параметры одного вызова SendToSignBase64FileExt/pdf.
// PDFBase64 — clean base64 без data URI префикса. AdditionalInfo несёт
// contracts.id (ключ Phase 0 search). NumberDial — отображаемый номер
// договора (UGC-{serial}), попадает в TrustMe details.NumberDial.
type SendToSignInput struct {
	PDFBase64      string
	AdditionalInfo string
	ContractName   string
	NumberDial     string
	Requisites     []Requisite
}

// SendToSignResult — выжимка из ответа TrustMe.
type SendToSignResult struct {
	DocumentID string
	ShortURL   string
	FileName   string
}

// SearchContractResult — Phase 0 recovery snapshot. ContractStatus — 0..9
// per TrustMe.
type SearchContractResult struct {
	DocumentID     string
	ShortURL       string
	ContractStatus int
}

// ErrTrustMeNotFound — search/Contracts не нашёл document с заданным
// additionalInfo. Worker трактует как «надо перепосылать».
var ErrTrustMeNotFound = errors.New("trustme: contract not found")

// Error — типизированная ошибка TrustMe API (status=Error). Code — «1219»,
// «1208» из errorText; outbox пишет его в contracts.last_error_code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Client — минимальная поверхность TrustMe API. Rate-limit (4 RPS),
// обёртка и парсинг ответов — внутри реализаций.
type Client interface {
	SendToSign(ctx context.Context, in SendToSignInput) (*SendToSignResult, error)
	SearchContractByAdditionalInfo(ctx context.Context, additionalInfo string) (*SearchContractResult, error)
	DownloadContractFile(ctx context.Context, documentID string) ([]byte, error)
}
