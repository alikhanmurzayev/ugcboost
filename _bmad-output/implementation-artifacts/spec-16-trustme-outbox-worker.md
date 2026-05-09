---
title: 'Чанк 16 — TrustMe outbox-worker'
type: feature
created: '2026-05-09'
status: review-passed
baseline_commit: c570a5a2310fb202271c581a04eada3af7709027
context:
  - docs/standards/backend-transactions.md
  - docs/standards/backend-libraries.md
  - docs/standards/backend-repository.md
---

> **Преамбула — стандарты обязательны.** Перед любой строкой production-кода агент полностью загружает `docs/standards/`. Все правила — hard rules. `context` выше — критичные для этой работы; остальные читаются по релевантности кода.

> **Single source of truth для design-решений** — `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md` (Decisions #5, #8, #10, #12, #13, #15–17 + раздел «Параметры overlay-рендера»). Спека ниже задаёт только скоуп chunk 16, реализационные tasks и acceptance — НЕ повторяет архитектурные решения.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Креатор согласился (`campaign_creators.status='agreed'`) — нужен персонализированный PDF-договор, отправленный в TrustMe для электронной подписи. Без авто-отправки EFW-кампания на ~100 креаторов нерабочая.

**Approach:** Background outbox-worker (`@every 10s` cron в `cmd/api`) подбирает `agreed`-ряды без `contract_id`, накладывает overlay (FIO/IIN/дата выдачи) на `campaigns.contract_template_pdf`, шлёт в TrustMe `SendToSignBase64FileExt`, переводит статус `signing`. Фазы 0/1/2/3 + recovery orphan'ов — per intent Decision #10. Webhook (chunk 17) и frontend (chunk 18) — out of scope.

## Boundaries & Constraints

**Always:**
- Phase 0 (recovery, network вне Tx) → Phase 1 (claim, `FOR UPDATE SKIP LOCKED`, milliseconds Tx) → Phase 2 (render+persist+send, network вне Tx) → Phase 3 (finalize, audit ВНУТРИ Tx). Бот-уведомление ПОСЛЕ Tx (fire-and-forget).
- Phase 1 SELECT-фильтр: `cc.status='agreed' AND cc.contract_id IS NULL AND c.is_deleted=false AND length(c.contract_template_pdf)>0`. Soft-deleted и пустой шаблон — tombstone'ы.
- Anti-PII: stdout-логи только UUID/HTTP-метаданные/error без sensitive context. Audit_logs payload — UUID'ы only. `error.Message` anti-fingerprinting.
- TrustMe `Authorization` header без `Bearer`; rate-limit 4 RPS (per blueprint requirement).
- `TRUSTME_MOCK=true` → SpyOnlyClient (default local + staging); `false` → RealClient (default prod).
- Все правила `docs/standards/` — особенно `backend-transactions.md` (audit ВНУТРИ Tx, бот ПОСЛЕ Tx), `backend-repository.md` (RepoFactory + stom), `backend-libraries.md` (registry).

**Ask First:**
- Sandbox-проверка `POST /search/Contracts` фильтра по `additionalInfo` показала, что фильтр не работает — Phase 0 recovery теряет смысл. HALT и обсуди план B с человеком.

**Never:**
- TeeClient (TrustMe не имеет sandbox; только SpyOnly + Real).
- Leader-election / advisory-lock / TTL-lease (Variant A — accept Phase 0 dup risk; runbook на ручную очистку через ЛК TrustMe).
- Webhook-handler (chunk 17), frontend (chunk 18), endpoints upload/download шаблона (chunk 9a).
- Прямой `pool.Begin()` — только `dbutil.WithTx`.
- Логирование PII (ФИО, ИИН, телефон, контент PDF, sha256-fingerprint допустим для диагностики).
- Ручные структуры API request/response для test-API (через codegen).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output | Error Handling |
|----------|--------------|-----------------|----------------|
| Happy | `agreed`, шаблон загружен | contracts row, `cc.signing+contract_id`, audit `contract_initiated`, бот-уведомление, TrustMe-запрос отправлен | N/A |
| Empty template | `length(template)=0` | Не подбирается Phase 1 SELECT | N/A |
| Soft-deleted | `c.is_deleted=true` | Не подбирается | N/A |
| Send fail (Phase 2c) | network/5xx | orphan: `unsigned_pdf NOT NULL, document_id=NULL`, `cc.status=signing` | Next tick: Phase 0 recovery |
| Phase 0 known | `search/Contracts` вернул known doc | Finalize с известным `trustme_document_id`, без re-send | N/A |
| Phase 0 unknown + PDF есть | search не нашёл, `unsigned_pdf NOT NULL` | Re-send того же PDF (sha256 не меняется) | Retry next tick если падает |
| Concurrent recovery | 2 worker'а на тот же orphan | Возможен дубль в TrustMe | Runbook: ручная очистка через ЛК TrustMe |
| Render error | Битый шаблон / нет плейсхолдеров | Ряд остаётся; error в лог без PII | Next tick retry; chunk 9a валидация должна была отсечь |
| Wrong status | `cc.status` ≠ `agreed` | Не подбирается | N/A |

</frozen-after-approval>

## Code Map

**Создаём:**
- `backend/migrations/<ts>_chunk16_trustme_outbox.sql` — CREATE contracts, ALTER cc.contract_id FK, ALTER cc_status_check (`signing/signed/signing_declined`), partial indexes `idx_campaign_creators_outbox` и `idx_contracts_orphan`.
- `backend/internal/contract/sender_service.go` (+ `_test.go`) — `ContractSenderService.RunOnce` со всеми 4 phases.
- `backend/internal/contract/pdf_renderer.go` (+ `_test.go`) — `ContractPDFRenderer.Render(ctx, template []byte, data ContractData) ([]byte, error)`; embedded `LiberationSerif-Regular.ttf` через `//go:embed`; параметры overlay один-в-один с `_bmad-output/experiments/pdf-overlay/main.go`.
- `backend/internal/contract/fonts/LiberationSerif-Regular.ttf` — embedded TTF.
- `backend/internal/contract/testdata/template.pdf` — fixture со всеми 3 плейсхолдерами.
- `backend/internal/trustme/{client.go, real_client.go}` (+ `_test.go`) — `Client` interface (`SendToSign`, `SearchContractByAdditionalInfo`, `DownloadContractFile`) + DTO + `RealClient` (HTTP, rate-limit `golang.org/x/time/rate`).
- `backend/internal/trustme/spy/{spy_store.go, spy_client.go}` (+ `_test.go`) — ring 5000 FIFO + `SpyOnlyClient` с детерминированными ответами + `RegisterFailNext` + `RegisterDocument`.
- `backend/internal/repository/contracts.go` (+ `_test.go`) — `ContractRepo` interface + repo (Insert, SelectAgreedForClaim с `FOR UPDATE SKIP LOCKED`+JOIN, SelectOrphansForRecovery, UpdateUnsignedPDF, UpdateAfterSend); pgErr 23505 → domain error.
- `backend/internal/domain/{phone.go, contract_date.go}` (+ `_test.go`) — `NormalizePhoneE164`; `FormatIssuedDate(t, loc)` («D» месяц YYYY г. в Asia/Almaty).
- `backend/cmd/api/trustme.go` — `setupTrustMe(cfg, logger)` (по образцу `cmd/api/telegram.go:31-56`).
- `backend/internal/testapi/trustme.go` (+ `_test.go`) — handlers для 5 endpoint'ов.
- `backend/e2e/contract/contract_test.go` — `TestContractSending` с 5 сценариями (русский header-нарратив, `t.Parallel`, `testutil` composable).

**Изменяем:**
- `backend/internal/repository/factory.go:90+` — `NewContractsRepo(db dbutil.DB) ContractRepo`.
- `backend/internal/repository/campaign_creators.go` — `UpdateContractIDAndStatus(ctx, id, contractID, status)` + `SelectAgreedForClaim(ctx, limit)` (с JOIN campaigns/creators в одном запросе).
- `backend/cmd/api/main.go:70-77` — после `scheduler.Start()` добавить `scheduler.AddFunc("@every 10s", contractSenderSvc.RunOnce)`; setup TrustMe и сервис до этого.
- `backend/api/openapi-test.yaml` — endpoints `/test/trustme/{run-outbox-once, spy-list, spy-clear, spy-fail-next, spy-register-document}`.
- `docs/standards/backend-libraries.md` — добавить `ledongthuc/pdf`, `signintech/gopdf`, `golang.org/x/time/rate` с обоснованием (pure-Go без CGo для PDF; rate-limiting per TrustMe blueprint requirement).
- `backend/go.mod` / `go.sum` — новые зависимости.

**Reference (читать, не править):**
- `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md` — все архитектурные решения.
- `_bmad-output/experiments/pdf-overlay/main.go` — overlay reference implementation.
- `backend/internal/telegram/{sender.go:12, tee_sender.go:13, spy_store.go:40}` — Spy/Real паттерн (но без TeeClient; `Client` naming, не `Sender`).
- `backend/cmd/api/telegram.go:31-56` — образец `setupTelegram`.
- `backend/internal/dbutil/db.go:149` — `WithTx` signature.
- `backend/internal/repository/{factory.go:1-93, audit.go:67-77}` — RepoFactory + audit Create + nullable `actor_id`.
- `backend/migrations/20260507044135_campaign_creators.sql:39` — текущий `cc_status_check` (расширяем).
- `backend/internal/contract/extractor.go` (chunk 9a) — переиспользуем `ExtractPlaceholders` в `pdf_renderer.go`.
- `docs/external/trustme/{blueprint.apib, postman-collection.json}` — TrustMe API spec.

## Tasks & Acceptance

**Execution:**
- [x] Миграция (CREATE contracts со всеми полями per intent Decision #8; ALTER cc + constraint; partial indexes).
- [x] `internal/repository/contracts.go` + расширение `campaign_creators.go` (`UpdateContractIDAndStatus`, `SelectAgreedForClaim` с FOR UPDATE SKIP LOCKED) + `factory.go` (`NewContractsRepo`).
- [x] `internal/contract/pdf_renderer.go` + embedded TTF + fixture template.pdf.
- [x] `internal/contract/sender_service.go` — Phase 0/1/2/3 (Phase 0 — recoverOrphans; Phase 1 — claimAgreed; Phase 2 — renderAndSend с persist; Phase 3 — finalize с audit).
- [x] `internal/trustme/client.go` (interface + DTO) + `real_client.go` (HTTP+rate-limit) + `spy/{spy_store.go, spy_client.go}`.
- [x] `internal/domain/{phone.go, contract_date.go}` (`NormalizePhoneE164`, `FormatIssuedDate`).
- [x] `cmd/api/trustme.go` setup + wiring в `main.go` (cron `AddFunc` после `scheduler.Start()`).
- [x] OpenAPI test-yaml расширение + `make generate-api` + `internal/testapi/trustme.go` handlers.
- [x] Все unit-тесты (per-file `_test.go`) — pgxmock для repos, mock TrustMeClient для service, fixture для renderer, table-driven для domain helpers, httptest для RealClient, ring-buffer тесты для SpyStore.
- [x] `backend/e2e/contract/contract_test.go` — Happy + Send-fail-recovery; Empty/Soft-deleted/Known-orphan покрыты unit + repo-уровнем (см. inline комментарии Skip).
- [x] Update `docs/standards/backend-libraries.md` registry.
- [x] `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` — все зелёные.

**Acceptance Criteria:**
- Given `agreed` ряд + загруженный шаблон, when `RunOnce` запущен, then `cc.status='signing'`, `contract_id NOT NULL`, contracts row со всеми полями, audit `campaign_creator.contract_initiated` (actor_id=NULL, payload UUID-only), бот-уведомление ушло, TrustMe-запрос отправлен с корректным `additionalInfo=contract.id`, base64-PDF parseable через `ledongthuc/pdf`, нормализованным phone в `Requisites.PhoneNumber`.
- Given пустой шаблон / soft-deleted кампания / `cc.status≠agreed`, when `RunOnce` запущен, then ряд не подбирается, `agreed` остаётся.
- Given Phase 2c упал (`spy.RegisterFailNext`), when next tick `RunOnce`, then Phase 0: TrustMe-known → finalize без re-send (sha256 unsigned_pdf не меняется, spy-list не растёт после `spy-clear`); TrustMe-unknown с PDF → re-send того же `unsigned_pdf` (sha256 совпадает).
- Given Phase 1 concurrent claim (unit с pgxmock), when 2 параллельных Tx читают одни и те же ряды, then `FOR UPDATE SKIP LOCKED` гарантирует, что оба не возьмут одинаковый ряд.
- Given `EnableTestEndpoints=false`, when `/test/trustme/*` вызван, then 404.
- Coverage gate ≥80% per-method для всех изменённых пакетов; race detector зелёный во всех тестах.

## Verification

**Commands:**
- `make build-backend` — компиляция OK без warnings.
- `make lint-backend` — gofmt + golangci-lint clean.
- `make migrate-reset && make migrate-up` — миграция идемпотентна.
- `make test-unit-backend && make test-unit-backend-coverage` — unit зелёные, coverage gate проходит.
- `make generate-api` — regenerated `internal/testapi/server.gen.go` + e2e client без ручных правок.
- `make test-e2e-backend` — e2e зелёные через SpyOnlyClient.

**Manual checks:**
- На staging (`TRUSTME_MOCK=true`) — поднять, прогнать happy-path вручную через test-API (`/test/trustme/run-outbox-once`), проверить spy-list — запрос корректно сформирован (multipart, base64-PDF parseable, нормализованный phone в `Requisites.PhoneNumber`).
