// Package spy реализует in-memory mock TrustMe-клиента: SpyOnlyClient + ring
// store записанных запросов. Используется на local + staging + во всех e2e
// тестах. Per Decision #17 intent-trustme-contract-v2: TrustMe не имеет
// sandbox, поэтому Tee-режим не делаем.
package spy

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// storeCapacity — ring 5000 по образцу telegram (см.
// `internal/telegram/spy_store.go:14`). Под ~50 параллельных staging e2e
// тестов с большим запасом.
const storeCapacity = 5000

// SentRecord — снимок одного исходящего SendToSign-запроса для test-API.
// PII (ФИО/ИИН/Телефон) хранится как sha256-фингерпринт (security.md hard
// rule запрещает PII в любых response bodies — даже test-only ручек). Raw
// PII живёт только в момент вызова SendToSign и не персистится.
//
// PDFSha256 — hex sha256 от исходного base64 PDF, который мы отправили в
// TrustMe. Сам PDF не сохраняется ни в memory ring, ни в API-ответ:
// rendered PDF содержит овellay'енные значения (ФИО/ИИН/IssuedDate), а
// security.md запрещает экспонировать PII в response bodies любых ручек,
// включая test-only. Ring капы 5000 × ~30-200KB PDF = ~1GB RAM на
// долгоживущем процессе — отказ от хранения PDF также убирает этот баг.
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

// Fingerprint возвращает первые 16 hex-символов sha256(value). Используется
// для опубликовываемых через test-API снимков SentRecord — security.md
// запрещает PII в response bodies, но e2e нужно сравнение «тот же ли FIO/IIN».
func Fingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

// HashPDFBase64 — полный hex sha256 от base64-encoded PDF. В отличие от
// Fingerprint (8 байт = 16 hex для sub-second collision-free), здесь полная
// 32-байтная hash сохраняется: e2e сравнивает PDF retry'ев на побайтную
// идентичность.
func HashPDFBase64(pdfBase64 string) string {
	if pdfBase64 == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(pdfBase64))
	return hex.EncodeToString(sum[:])
}

// failNextEntry — запланированная ошибка, фалит ровно один следующий
// SendToSign по additionalInfo (или N штук подряд).
type failNextEntry struct {
	reason string
	count  int
}

// SpyStore — потокобезопасное хранилище SentRecord + регистраций fail-next +
// «known»-документов (для имитации Phase 0 recovery, когда TrustMe «знает»
// наш orphan).
type SpyStore struct {
	mu             sync.Mutex
	records        []SentRecord
	failNext       map[string]*failNextEntry
	failNextAnyN   int
	failNextAnyMsg string
	known          map[string]knownDocument
}

type knownDocument struct {
	DocumentID     string
	ShortURL       string
	ContractStatus int
}

func NewSpyStore() *SpyStore {
	return &SpyStore{
		records:  make([]SentRecord, 0, storeCapacity),
		failNext: make(map[string]*failNextEntry),
		known:    make(map[string]knownDocument),
	}
}

// RegisterFailNext очерёдывает синтетический сбой для следующих `count`
// SendToSign'ов с указанным additionalInfo. Если additionalInfo == ""
// (wildcard), фейлит следующие `count` вызовов независимо от additionalInfo —
// нужно для e2e Phase 0 recovery теста, где contract_id неизвестен заранее.
// count<=0 → 1.
func (s *SpyStore) RegisterFailNext(additionalInfo, reason string, count int) {
	if count <= 0 {
		count = 1
	}
	if reason == "" {
		reason = "spy: synthetic failure"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if additionalInfo == "" {
		s.failNextAnyN = count
		s.failNextAnyMsg = reason
		return
	}
	s.failNext[additionalInfo] = &failNextEntry{reason: reason, count: count}
}

// RegisterDocument имитирует «TrustMe уже принял этот документ» — следующий
// SearchContractByAdditionalInfo вернёт зарегистрированный документ.
// Используется e2e-сценарием Phase 0 finalize-without-resend.
func (s *SpyStore) RegisterDocument(additionalInfo, documentID, shortURL string, contractStatus int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known[additionalInfo] = knownDocument{
		DocumentID:     documentID,
		ShortURL:       shortURL,
		ContractStatus: contractStatus,
	}
}

// Clear сбрасывает все записанные SentRecord + failNext + known. Используется
// e2e между сценариями — обычно через test-api /test/trustme/spy-clear.
func (s *SpyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = s.records[:0]
	s.failNext = make(map[string]*failNextEntry)
	s.failNextAnyN = 0
	s.failNextAnyMsg = ""
	s.known = make(map[string]knownDocument)
}

// Record добавляет запись в ring. Старые вытесняются FIFO.
func (s *SpyStore) Record(rec SentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) >= storeCapacity {
		s.records = s.records[1:]
	}
	s.records = append(s.records, rec)
}

// List возвращает копию ring в порядке вставки.
func (s *SpyStore) List() []SentRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SentRecord, len(s.records))
	copy(out, s.records)
	return out
}

// consumeFailNext возвращает (reason, true), если для additionalInfo
// зарегистрирован fail (либо wildcard). Декрементирует counter; когда дошёл
// до 0 — удаляет entry. Specific match имеет приоритет над wildcard.
func (s *SpyStore) consumeFailNext(additionalInfo string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.failNext[additionalInfo]; ok {
		reason := entry.reason
		entry.count--
		if entry.count <= 0 {
			delete(s.failNext, additionalInfo)
		}
		return reason, true
	}
	if s.failNextAnyN > 0 {
		reason := s.failNextAnyMsg
		s.failNextAnyN--
		if s.failNextAnyN == 0 {
			s.failNextAnyMsg = ""
		}
		return reason, true
	}
	return "", false
}

func (s *SpyStore) lookupKnown(additionalInfo string) (knownDocument, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.known[additionalInfo]
	return doc, ok
}
