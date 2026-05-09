package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
	hmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
)

// fakeRunner implements trustmeport.OutboxRunner — records that RunOnce was called.
type fakeRunner struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeRunner) RunOnce(_ context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
}

// fakeSpyStore implements trustmeport.SpyStore for tests.
type fakeSpyStore struct {
	mu       sync.Mutex
	records  []trustmeport.SentRecord
	cleared  int
	failNext []failArgs
	docs     []docArgs
}

type failArgs struct {
	additionalInfo string
	reason         string
	count          int
}

type docArgs struct {
	additionalInfo string
	documentID     string
	shortURL       string
	contractStatus int
}

func (f *fakeSpyStore) List() []trustmeport.SentRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]trustmeport.SentRecord, len(f.records))
	copy(out, f.records)
	return out
}

func (f *fakeSpyStore) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
	f.records = nil
}

func (f *fakeSpyStore) RegisterFailNext(additionalInfo, reason string, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = append(f.failNext, failArgs{additionalInfo, reason, count})
}

func (f *fakeSpyStore) RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = append(f.docs, docArgs{additionalInfo, documentID, shortURL, contractStatus})
}

func newTrustMeTestHandler(t *testing.T, runner trustmeport.OutboxRunner, spy trustmeport.SpyStore) http.Handler {
	t.Helper()
	auth := hmocks.NewMockTestAPIAuthService(t)
	pool := mocks.NewMockPool(t)
	repos := hmocks.NewMockTestAPICleanupRepoFactory(t)
	store := hmocks.NewMockTokenStore(t)
	log := logmocks.NewMockLogger(t)
	return newTestAPIRouter(t, NewTestAPIHandler(auth, pool, repos, store,
		telegram.NewHandler(nil, log), telegram.NewSentSpyStore(), "",
		runner, spy, log))
}

func TestTrustMeRunOutboxOnce_RunsAndReturns204(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	spy := &fakeSpyStore{}
	router := newTrustMeTestHandler(t, runner, spy)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/run-outbox-once", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 1, runner.calls)
}

func TestTrustMeRunOutboxOnce_NoRunner_ReturnsValidation(t *testing.T) {
	t.Parallel()
	router := newTrustMeTestHandler(t, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/run-outbox-once", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNoContent, w.Code)
}

func TestTrustMeSpyList_ReturnsRecords(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	spy := &fakeSpyStore{records: []trustmeport.SentRecord{
		{
			DocumentID:       "doc-1",
			ShortURL:         "https://t.tct.kz/uploader/doc-1",
			AdditionalInfo:   "ct-1",
			ContractName:     "Договор UGC",
			FIOFingerprint:   "fingerprint-fio",
			IINFingerprint:   "fingerprint-iin",
			PhoneFingerprint: "fingerprint-phone",
			PDFBase64:        "JVBERi0xLg==",
			SentAt:           now,
		},
	}}
	router := newTrustMeTestHandler(t, &fakeRunner{}, spy)

	req := httptest.NewRequest(http.MethodGet, "/test/trustme/spy-list", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, "ct-1", resp.Data.Items[0]["additionalInfo"])
	require.Equal(t, "doc-1", resp.Data.Items[0]["documentId"])
}

func TestTrustMeSpyClear_204(t *testing.T) {
	t.Parallel()
	spy := &fakeSpyStore{records: []trustmeport.SentRecord{{AdditionalInfo: "x"}}}
	router := newTrustMeTestHandler(t, &fakeRunner{}, spy)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-clear", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 1, spy.cleared)
}

func TestTrustMeSpyFailNext(t *testing.T) {
	t.Parallel()
	spy := &fakeSpyStore{}
	router := newTrustMeTestHandler(t, &fakeRunner{}, spy)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-fail-next",
		strings.NewReader(`{"additionalInfo":"ct-1","reason":"boom","count":2}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, spy.failNext, 1)
	require.Equal(t, "ct-1", spy.failNext[0].additionalInfo)
	require.Equal(t, "boom", spy.failNext[0].reason)
	require.Equal(t, 2, spy.failNext[0].count)
}

func TestTrustMeSpyRegisterDocument(t *testing.T) {
	t.Parallel()
	spy := &fakeSpyStore{}
	router := newTrustMeTestHandler(t, &fakeRunner{}, spy)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-register-document",
		strings.NewReader(`{"additionalInfo":"ct-1","documentId":"doc-X","contractStatus":2}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, spy.docs, 1)
	require.Equal(t, "ct-1", spy.docs[0].additionalInfo)
	require.Equal(t, "doc-X", spy.docs[0].documentID)
	require.Equal(t, 2, spy.docs[0].contractStatus)
	require.Equal(t, "https://test.trustme.kz/uploader/doc-X", spy.docs[0].shortURL)
}

func TestTrustMeSpyClear_NoSpy_ReturnsValidation(t *testing.T) {
	t.Parallel()
	router := newTrustMeTestHandler(t, &fakeRunner{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-clear", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNoContent, w.Code)
}

func TestTrustMeSpyRegisterDocument_NoSpy_ReturnsValidation(t *testing.T) {
	t.Parallel()
	router := newTrustMeTestHandler(t, &fakeRunner{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-register-document",
		strings.NewReader(`{"additionalInfo":"ct-1","documentId":"doc-X"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotEqual(t, http.StatusNoContent, w.Code)
}

func TestTrustMeSpyRegisterDocument_CustomShortURL(t *testing.T) {
	t.Parallel()
	spy := &fakeSpyStore{}
	router := newTrustMeTestHandler(t, &fakeRunner{}, spy)

	req := httptest.NewRequest(http.MethodPost, "/test/trustme/spy-register-document",
		strings.NewReader(`{"additionalInfo":"ct-1","documentId":"doc-X","shortUrl":"https://custom/doc-X"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, spy.docs, 1)
	require.Equal(t, "https://custom/doc-X", spy.docs[0].shortURL)
}
