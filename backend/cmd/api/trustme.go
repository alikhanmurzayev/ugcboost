package main

import (
	"fmt"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme/spy"
)

// trustMeRig bundles the TrustMe-related dependencies main.go hands to the
// service layer and the test-API. SpyStore is non-nil iff TrustMeMock=true OR
// EnableTestEndpoints=true — test-api/trustme handlers depend on it for
// /test/trustme/spy-* endpoints. Per Decision #17 of intent v2: there is no
// Tee mode (TrustMe has no sandbox).
type trustMeRig struct {
	Client   trustme.Client
	SpyStore *spy.SpyStore
}

// setupTrustMe builds the Client per (TrustMeMock, EnableTestEndpoints).
//   - TrustMeMock=true → SpyOnlyClient (default local + staging + tests).
//   - TrustMeMock=false → RealClient (default prod). EnableTestEndpoints
//     stays false in prod, so SpyStore is nil and /test/trustme/* returns 404.
//
// Fail-fast: при TrustMeMock=false проверяем что TrustMeBaseURL и
// TrustMeToken — non-empty. Иначе worker молча шлёт каждый запрос с пустым
// Authorization header и часами retry'ит впустую.
//
// On staging Alikhan flips TRUSTME_MOCK=false manually for a one-off real
// run with the prod token — that path leaves SpyStore nil and the test-API
// trustme handlers return 404 (no spy) rather than confusing data.
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
		Client: trustme.NewRealClient(cfg.TrustMeBaseURL, cfg.TrustMeToken, nil),
	}, nil
}

// trustMeSpyAdapter wraps spy.SpyStore to satisfy trustmeport.SpyStore without
// leaking the spy package across the handler boundary.
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
			DocumentID:       r.DocumentID,
			ShortURL:         r.ShortURL,
			AdditionalInfo:   r.AdditionalInfo,
			ContractName:     r.ContractName,
			FIOFingerprint:   r.FIOFingerprint,
			IINFingerprint:   r.IINFingerprint,
			PhoneFingerprint: r.PhoneFingerprint,
			PDFBase64:        r.PDFBase64,
			SentAt:           r.SentAt,
			Err:              r.Err,
		}
	}
	return out
}

func (a *trustMeSpyAdapter) Clear() { a.store.Clear() }

func (a *trustMeSpyAdapter) RegisterFailNext(additionalInfo, reason string, count int) {
	a.store.RegisterFailNext(additionalInfo, reason, count)
}

func (a *trustMeSpyAdapter) RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int) {
	a.store.RegisterDocument(additionalInfo, documentID, shortURL, contractStatus)
}
