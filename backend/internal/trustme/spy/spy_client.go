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

// Client — SpyOnlyClient. Реализует trustme.Client через in-memory store.
type Client struct {
	store *SpyStore
	now   func() time.Time
}

// NewClient собирает spy. nowFn опциональный (детерминированные SentAt).
func NewClient(store *SpyStore, nowFn func() time.Time) *Client {
	if store == nil {
		panic("trustme/spy: NewClient requires non-nil store")
	}
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &Client{store: store, now: nowFn}
}

// SendToSign имитирует TrustMe: detereministic document_id = "spy-" +
// hash(additionalInfo)[:10]. RegisterFailNext возвращает синтетическую ошибку.
func (c *Client) SendToSign(ctx context.Context, in trustme.SendToSignInput) (*trustme.SendToSignResult, error) {
	first := pickFirstRequisite(in.Requisites)
	if reason, ok := c.store.consumeFailNext(in.AdditionalInfo); ok {
		c.store.Record(SentRecord{
			AdditionalInfo: in.AdditionalInfo,
			ContractName:   in.ContractName,
			NumberDial:     in.NumberDial,
			FIO:            first.FIO,
			IIN:            first.IINBIN,
			Phone:          first.PhoneNumber,
			PDFSha256:      HashPDFBase64(in.PDFBase64),
			SentAt:         c.now(),
			Err:            reason,
		})
		return nil, errors.New(reason)
	}
	docID := documentIDFromAdditionalInfo(in.AdditionalInfo)
	shortURL := "https://test.trustme.kz/uploader/" + docID
	c.store.Record(SentRecord{
		DocumentID:     docID,
		ShortURL:       shortURL,
		AdditionalInfo: in.AdditionalInfo,
		ContractName:   in.ContractName,
		NumberDial:     in.NumberDial,
		FIO:            first.FIO,
		IIN:            first.IINBIN,
		Phone:          first.PhoneNumber,
		PDFSha256:      HashPDFBase64(in.PDFBase64),
		SentAt:         c.now(),
	})
	return &trustme.SendToSignResult{
		DocumentID: docID,
		ShortURL:   shortURL,
		FileName:   "spy-" + docID + ".pdf",
	}, nil
}

// SearchContractByAdditionalInfo — RegisterDocument, иначе ErrTrustMeNotFound.
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

// DownloadContractFile — синтетические байты для совместимости интерфейса
// (chunk 17 webhook flow).
func (c *Client) DownloadContractFile(ctx context.Context, documentID string) ([]byte, error) {
	if documentID == "" {
		return nil, errors.New("trustme/spy: empty document_id")
	}
	return []byte(fmt.Sprintf("spy-signed-%s", documentID)), nil
}

func pickFirstRequisite(rqs []trustme.Requisite) trustme.Requisite {
	if len(rqs) == 0 {
		return trustme.Requisite{}
	}
	return rqs[0]
}

// documentIDFromAdditionalInfo — детерминированный spy-<10hex>. Один и тот
// же additionalInfo → тот же docID (важно для re-send сценария).
func documentIDFromAdditionalInfo(additionalInfo string) string {
	sum := sha256.Sum256([]byte("trustme-spy:" + additionalInfo))
	return "spy-" + hex.EncodeToString(sum[:5])
}
