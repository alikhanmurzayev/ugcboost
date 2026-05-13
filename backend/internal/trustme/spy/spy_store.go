// Package spy — in-memory mock TrustMe-клиента: SpyOnlyClient + ring store
// SendToSign-вызовов. Local + staging + e2e. Per Decision #17 intent v2 —
// у TrustMe нет sandbox, поэтому Tee нет.
package spy

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// storeCapacity — ring 5000 (как telegram spy_store). Под ~50 параллельных
// staging e2e тестов с запасом.
const storeCapacity = 5000

// SentRecord — снимок одного исходящего SendToSign'а для test-API. Хранит
// сырые FIO/IIN/Phone — test endpoint gated EnableTestEndpoints (404 в
// проде), реальные ПД сюда не попадают, синтетические e2e-фикстуры безопасно
// возвращать как есть. PDF не храним целиком (5000 × ~100KB = ~500MB), только
// sha256 для assert'а identity между retry'ями.
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

// HashPDFBase64 — полный hex sha256 от base64-encoded PDF. e2e сравнивает
// побайтовую идентичность retry'ев.
func HashPDFBase64(pdfBase64 string) string {
	if pdfBase64 == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(pdfBase64))
	return hex.EncodeToString(sum[:])
}

// failEntry — постоянный синтетический сбой SendToSign на IIN. Держится до
// явного ClearFail, чтобы тест был устойчив к параллельным worker-тикам
// (Phase 0 recovery, повторные Phase 1 claim'ы по одному ряду).
type failEntry struct {
	reason string
}

// defaultRecordTTL — окно, после которого SentRecord считается протухшим и
// отбрасывается из List() / Record(). Spy-store держит сырые ПД между
// тестами одного процесса; час — компромисс между «не мешать ретраю в
// рамках одного прогона» и «не накапливать staging-замусоривание».
const defaultRecordTTL = time.Hour

// SpyStore — потокобезопасное хранилище записей + fail-injection +
// known-документов (Phase 0 finalize-without-resend). Записи затухают по
// TTL — без этого IIN-collision между прогонами на staging теоретически
// возможен.
//
// fail ключуется по IIN (а не по contract.id, как раньше): IIN известен
// тесту до того, как outbox создаёт contract.id, и не пересекается между
// параллельными tests/packages в одном backend-процессе. fail держится до
// явного ClearFail — count-based one-shot убран по той же причине (Phase 0
// recovery параллельного worker'а мог consume'нуть единственный fail до
// того, как тест успевал проверить spy-list).
type SpyStore struct {
	mu      sync.Mutex
	records []SentRecord
	failed  map[string]failEntry
	known   map[string]knownDocument
	ttl     time.Duration
	now     func() time.Time
}

type knownDocument struct {
	DocumentID     string
	ShortURL       string
	ContractStatus int
}

func NewSpyStore() *SpyStore {
	return &SpyStore{
		records: make([]SentRecord, 0, storeCapacity),
		failed:  make(map[string]failEntry),
		known:   make(map[string]knownDocument),
		ttl:     defaultRecordTTL,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// RegisterFail включает постоянный синтетический сбой SendToSign на IIN —
// каждый consumeFail возвращает reason до явного ClearFail. Overwrites any
// previous registration. Empty iin — no-op (handler validates).
func (s *SpyStore) RegisterFail(iin, reason string) {
	if iin == "" {
		return
	}
	if reason == "" {
		reason = "spy: synthetic failure"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[iin] = failEntry{reason: reason}
}

// ClearFail снимает fail-регистрацию для IIN. No-op если нет регистрации.
func (s *SpyStore) ClearFail(iin string) {
	if iin == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failed, iin)
}

// RegisterDocument — «TrustMe уже принял этот документ». Используется в e2e
// Phase 0 finalize-without-resend.
func (s *SpyStore) RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known[additionalInfo] = knownDocument{
		DocumentID:     documentID,
		ShortURL:       shortURL,
		ContractStatus: contractStatus,
	}
}

// Clear сбрасывает records + fail-injections + known.
func (s *SpyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = s.records[:0]
	s.failed = make(map[string]failEntry)
	s.known = make(map[string]knownDocument)
}

// Record добавляет запись. Старые вытесняются FIFO либо по TTL.
func (s *SpyStore) Record(rec SentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	if len(s.records) >= storeCapacity {
		s.records = s.records[1:]
	}
	s.records = append(s.records, rec)
}

// List возвращает копию ring в порядке вставки. Перед копированием
// отбрасывает записи старше TTL.
func (s *SpyStore) List() []SentRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	out := make([]SentRecord, len(s.records))
	copy(out, s.records)
	return out
}

// evictExpiredLocked отбрасывает протухшие записи. Caller обязан держать
// s.mu. Дёргается из List() и Record() — ленивый GC, без отдельной горутины.
func (s *SpyStore) evictExpiredLocked() {
	if s.ttl <= 0 {
		return
	}
	cutoff := s.now().Add(-s.ttl)
	i := 0
	for i < len(s.records) && s.records[i].SentAt.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.records = s.records[i:]
	}
}

// consumeFail возвращает (reason, true) если для iin есть fail-регистрация.
// Регистрация не удаляется — fail постоянный до явного ClearFail.
func (s *SpyStore) consumeFail(iin string) (string, bool) {
	if iin == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.failed[iin]
	if !ok {
		return "", false
	}
	return entry.reason, true
}

func (s *SpyStore) lookupKnown(additionalInfo string) (knownDocument, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.known[additionalInfo]
	return doc, ok
}
