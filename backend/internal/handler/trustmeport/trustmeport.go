// Package trustmeport — DTO/интерфейсы между handler.TestAPI и TrustMe-
// адаптерами. Вынесено в подпакет ради избежания import cycle с mockery.
package trustmeport

import (
	"context"
	"time"
)

// SentRecord — публичный shape для test-handler. Дублирует
// trustme/spy.SentRecord; cmd/api adapter маппит между ними. Test endpoint
// gated EnableTestEndpoints (404 в проде), поэтому сырые FIO/IIN/Phone
// допустимы — синтетические e2e данные, не реальные ПД.
type SentRecord struct {
	DocumentID     string
	ShortURL       string
	AdditionalInfo string
	ContractName   string
	NumberDial     string
	FIO            string
	IIN            string
	Phone          string
	PDFSha256      string
	SentAt         time.Time
	Err            string
}

// OutboxRunner — синхронный driver outbox-тика для /test/trustme/run-outbox-once.
type OutboxRunner interface {
	RunOnce(ctx context.Context)
}

// SpyStore — подмножество trustme/spy.SpyStore, которое читает test-handler.
type SpyStore interface {
	List() []SentRecord
	Clear()
	RegisterFail(iin, reason string)
	ClearFail(iin string)
	RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int)
}
