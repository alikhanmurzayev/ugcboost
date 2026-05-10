package main

import (
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme/spy"
)

// trustMeRig bundles TrustMe deps. SpyStore non-nil только при TrustMeMock=true;
// /test/trustme/* отдаёт 404 если spy nil.
type trustMeRig struct {
	Client   trustme.Client
	SpyStore *spy.SpyStore
}

// setupTrustMe — TrustMeMock=true → SpyOnlyClient; false → RealClient
// (требует TrustMeBaseURL+Token, иначе fail-fast).
func setupTrustMe(cfg *config.Config) (*trustMeRig, error) {
	if cfg.TrustMeMock {
		store := spy.NewSpyStore()
		return &trustMeRig{
			Client:   spy.NewClient(store, nil),
			SpyStore: store,
		}, nil
	}
	if cfg.TrustMeBaseURL == "" {
		return nil, fmt.Errorf("TRUSTME_BASE_URL must be a non-empty value when TRUSTME_MOCK=false")
	}
	if cfg.TrustMeToken == "" {
		return nil, fmt.Errorf("TRUSTME_TOKEN must be a non-empty value when TRUSTME_MOCK=false")
	}
	return &trustMeRig{
		Client: trustme.NewRealClient(cfg.TrustMeBaseURL, cfg.TrustMeToken, cfg.TrustMeKzBmg, nil),
	}, nil
}

// trustMeSpyAdapter маппит spy.SpyStore → trustmeport.SpyStore, чтобы
// handler не импортировал spy.
type trustMeSpyAdapter struct {
	store *spy.SpyStore
}

func newTrustMeSpyAdapter(store *spy.SpyStore) *trustMeSpyAdapter {
	return &trustMeSpyAdapter{store: store}
}

func (a *trustMeSpyAdapter) List() []trustmeport.SentRecord {
	src := a.store.List()
	out := make([]trustmeport.SentRecord, len(src))
	for i, r := range src {
		out[i] = trustmeport.SentRecord{
			DocumentID:     r.DocumentID,
			ShortURL:       r.ShortURL,
			AdditionalInfo: r.AdditionalInfo,
			ContractName:   r.ContractName,
			NumberDial:     r.NumberDial,
			FIO:            r.FIO,
			IIN:            r.IIN,
			Phone:          r.Phone,
			PDFSha256:      r.PDFSha256,
			SentAt:         r.SentAt,
			Err:            r.Err,
		}
	}
	return out
}

func (a *trustMeSpyAdapter) Clear() { a.store.Clear() }

func (a *trustMeSpyAdapter) RegisterFailNext(iin, reason string, count int) {
	a.store.RegisterFailNext(iin, reason, count)
}

func (a *trustMeSpyAdapter) RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int) {
	a.store.RegisterDocument(additionalInfo, documentID, shortURL, contractStatus)
}
