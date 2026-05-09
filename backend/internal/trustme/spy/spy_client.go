package spy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
)

// Client — SpyOnlyClient. Реализует trustme.Client через локальное хранилище
// без сетевых вызовов. Используется local + staging + во всех e2e тестах.
type Client struct {
	store *SpyStore
	now   func() time.Time
}

// NewClient собирает spy-клиент. nowFn опциональный — для детерминированных
// SentAt в тестах; nil → time.Now().UTC().
func NewClient(store *SpyStore, nowFn func() time.Time) *Client {
	if store == nil {
		panic("trustme/spy: NewClient requires non-nil store")
	}
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &Client{store: store, now: nowFn}
}

// SendToSign имитирует TrustMe: генерирует document_id = "spy-" +
// hash(additionalInfo)[:10], short_url — соответствующий URL. RegisterFailNext
// возвращает синтетическую ошибку и пишет запись с Err.
func (c *Client) SendToSign(ctx context.Context, in trustme.SendToSignInput) (*trustme.SendToSignResult, error) {
	first := pickFirstRequisite(in.Requisites)
	if reason, ok := c.store.consumeFailNext(in.AdditionalInfo); ok {
		c.store.Record(SentRecord{
			AdditionalInfo:   in.AdditionalInfo,
			ContractName:     in.ContractName,
			FIOFingerprint:   Fingerprint(first.FIO),
			IINFingerprint:   Fingerprint(first.IINBIN),
			PhoneFingerprint: Fingerprint(first.PhoneNumber),
			PDFSha256:        HashPDFBase64(in.PDFBase64),
			SentAt:           c.now(),
			Err:              reason,
		})
		return nil, errors.New(reason)
	}
	docID := documentIDFromAdditionalInfo(in.AdditionalInfo)
	shortURL := "https://test.trustme.kz/uploader/" + docID
	c.store.Record(SentRecord{
		DocumentID:       docID,
		ShortURL:         shortURL,
		AdditionalInfo:   in.AdditionalInfo,
		ContractName:     in.ContractName,
		FIOFingerprint:   Fingerprint(first.FIO),
		IINFingerprint:   Fingerprint(first.IINBIN),
		PhoneFingerprint: Fingerprint(first.PhoneNumber),
		PDFSha256:        HashPDFBase64(in.PDFBase64),
		SentAt:           c.now(),
	})
	return &trustme.SendToSignResult{
		DocumentID: docID,
		ShortURL:   shortURL,
		FileName:   "spy-" + docID + ".pdf",
	}, nil
}

// SearchContractByAdditionalInfo возвращает документ из knownDocuments,
// если RegisterDocument был вызван для этого additionalInfo; иначе
// trustme.ErrTrustMeNotFound.
func (c *Client) SearchContractByAdditionalInfo(ctx context.Context, additionalInfo string) (*trustme.SearchContractResult, error) {
	if additionalInfo == "" {
		return nil, errors.New("trustme/spy: empty additionalInfo")
	}
	doc, ok := c.store.lookupKnown(additionalInfo)
	if !ok {
		return nil, trustme.ErrTrustMeNotFound
	}
	return &trustme.SearchContractResult{
		DocumentID:     doc.DocumentID,
		ShortURL:       doc.ShortURL,
		ContractStatus: doc.ContractStatus,
	}, nil
}

// DownloadContractFile спай возвращает синтетические байты «spy-signed-<docID>».
// Реальная подпись добавится chunk 17 webhook-flow'ом — здесь сигнатура
// фиксируется только для совместимости интерфейса.
func (c *Client) DownloadContractFile(ctx context.Context, documentID string) ([]byte, error) {
	if documentID == "" {
		return nil, errors.New("trustme/spy: empty document_id")
	}
	return []byte(fmt.Sprintf("spy-signed-%s", documentID)), nil
}

// pickFirstRequisite возвращает первый ряд requisites или пустой Requisite.
func pickFirstRequisite(rqs []trustme.Requisite) trustme.Requisite {
	if len(rqs) == 0 {
		return trustme.Requisite{}
	}
	return rqs[0]
}

// documentIDFromAdditionalInfo — детерминированный «spy-<10hex>». Тот же
// additionalInfo даёт тот же docID — нужно для re-send-после-сбоя сценария
// (sha256 идентичный, document_id идентичный).
func documentIDFromAdditionalInfo(additionalInfo string) string {
	sum := sha256.Sum256([]byte("trustme-spy:" + additionalInfo))
	return "spy-" + hex.EncodeToString(sum[:5])
}
