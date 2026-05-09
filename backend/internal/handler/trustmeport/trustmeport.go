// Package trustmeport объявляет интерфейсы и DTO, которыми handler.TestAPI
// общается с TrustMe-адаптерами. Вынесено в подпакет, чтобы mockery (который
// генерит моки в handler/mocks/) не создавал import cycle: моки на типы из
// handler' которые сами лежат в handler/.
package trustmeport

import (
	"context"
	"time"
)

// SentRecord — публичный shape, который видит test-handler. Дублирует поля
// trustme/spy.SentRecord, чтобы handler не импортировал spy package и
// adapter в cmd/api конвертировал между ними.
//
// PII fields (FIO/IIN/Phone) и PDFBase64 намеренно отсутствуют — security.md
// hard rule запрещает PII в response bodies любых endpoint'ов; rendered PDF
// содержит overlay'енные ФИО/ИИН/IssuedDate. Вместо raw value test API
// экспонирует sha256-fingerprint префикс / полный sha256 hash PDF.
type SentRecord struct {
	DocumentID       string
	ShortURL         string
	AdditionalInfo   string
	ContractName     string
	FIOFingerprint   string
	IINFingerprint   string
	PhoneFingerprint string
	PDFSha256        string
	SentAt           time.Time
	Err              string
}

// OutboxRunner abstracts ContractSenderService so /test/trustme/run-outbox-once
// can drive one tick synchronously without importing the contract package.
type OutboxRunner interface {
	RunOnce(ctx context.Context)
}

// SpyStore is the subset of trustme/spy.SpyStore the test handler reads.
type SpyStore interface {
	List() []SentRecord
	Clear()
	RegisterFailNext(additionalInfo, reason string, count int)
	RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int)
}
