---
title: 'TrustMe SendToSign payload — CompanyName=ФИО + KzBmg env-flag'
type: 'feature'
created: '2026-05-09'
status: 'done'
baseline_commit: '059571d'
context:
  - docs/standards/backend-design.md
  - docs/standards/backend-libraries.md
  - docs/standards/backend-testing-unit.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** В личном кабинете TrustMe сторона договора (креатор) отображается как «Креатор», потому что в `Requisites[].CompanyName` мы шлём литерал `"Креатор"`. Параллельно — связка ИИН/ФИО/Phone не верифицируется ни TrustMe (`KzBmg=false`), ни нами через госреестр; теоретически возможно подписать договор за другого человека.

**Approach:** (A) `Requisites[].CompanyName ← composeFIO(...)` в обоих send-местах outbox-worker'а (`processClaim` Phase 2c + `resendOrphan` Phase 0); поднять `fio := composeFIO(...)` в локальную переменную и переиспользовать. (B) Новый config-флаг `TRUSTME_KZ_BMG` (default `false`) → хранится в `RealClient.kzBmg` через расширенный `NewRealClient(...)` → подставляется в `sendToSignDetails.KzBmg` в `SendToSign`. После deploy — runbook на prod (или staging с `TRUSTME_MOCK=false` + прод-токен) для одного теста.

## Boundaries & Constraints

**Always:**
- `composeFIO(...)` вычисляется один раз в локальную переменную `fio` перед литералом `Requisite{}` и используется и для `CompanyName`, и для `FIO`.
- `KzBmg` — config-параметр `RealClient`, не per-request `SendToSignInput` (симметрия с `baseURL`/`token`).
- `NewRealClient(...)` расширяется новым параметром `kzBmg bool`, без `Set*`-методов (`backend-design.md`: иммутабельность после `New*`).
- `TRUSTME_KZ_BMG` имеет `envDefault:"false"`, без `required:"true"` (не ломаем local/staging запуск).
- Сессия — spec-only. Остановиться на CHECKPOINT 1 после approval; имплементацию запускать только по явному go-ahead Алихана.

**Ask First:**
- Конфликт-резолв в `backend/internal/config/config.go` или `backend/cmd/api/trustme.go`, если параллельные агенты внесут правки до старта имплементации (на момент investigate `TrustMeWebhookToken` уже добавлен chunk-17 агентом в строку 74).
- Результат prod-trial'а: «требует подключения / нет тарифа» — открывать тикет с TrustMe (не код).

**Never:**
- Не трогать `backend/internal/contract/webhook_service.go`, `backend/internal/handler/trustme_webhook.go`, `backend/internal/domain/trustme_webhook.go`, `_bmad-output/implementation-artifacts/spec-17-trustme-webhook.md`, `_bmad-output/implementation-artifacts/intent-chunk-17-trustme-webhook.md`, `spec-18-*` или `intent-chunk-18-*` (WIP параллельных агентов).
- Не модифицировать `SpyOnlyClient` или `spy.SentRecord` (`CompanyName`/`KzBmg` в spy не наблюдаются — by design).
- Не расширять `SendToSignInput` под per-request `KzBmg`; не добавлять `FaceId`-config; не делать backward-compat для legacy-договоров с CompanyName="Креатор".
- Не удалять `_bmad-output/implementation-artifacts/intent-trustme-companyname-kzbmg.md` после spec-approval — living document до архивирования через `/finalize`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Send Phase 2c (happy path) | creator: last="Иванов", first="Иван", middle="Иванович"; `TRUSTME_KZ_BMG=false` | TrustMe payload: `Requisites[0].CompanyName="Иванов Иван Иванович"` == `FIO`; `details.KzBmg=false` | N/A |
| Orphan resend Phase 0 | тот же creator, существующий orphan-contract | те же поля в payload, что happy path | N/A |
| Middle name NULL | last="Иванов", first="Иван", middle=NULL | `composeFIO` → `"Иванов Иван"` без trailing space; CompanyName == FIO | N/A |
| KzBmg включён | `TRUSTME_KZ_BMG=true` | в multipart `details` JSON встречается `"KzBmg":true` | N/A |
| Env не задан | env пуст / отсутствует | `cfg.TrustMeKzBmg=false`; `details.KzBmg=false` (текущее поведение) | N/A |

</frozen-after-approval>

## Code Map

- `backend/internal/contract/sender_service.go` -- хранит `trustMeCreatorCompanyName="Креатор"` константу (lines 36-37) + два call-site'а `Requisite{}` (`resendOrphan` line 205-210, `processClaim` line 308-313).
- `backend/internal/contract/sender_service_test.go` -- captured-input тесты `TestContractSenderService_RunOnce_HappyPath` (line ~164) и `TestContractSenderService_Phase0_Resend_WithRealRequisites` (line ~262) уже проверяют `FIO`/`IINBIN`/`PhoneNumber` через `mock.MatchedBy` — расширяем до `CompanyName`.
- `backend/internal/config/config.go` -- `TrustMeWebhookToken` уже на line 74 (chunk-17 агент); `TrustMeKzBmg` добавляется следом.
- `backend/internal/trustme/real_client.go` -- `RealClient` struct (lines 26-31), `NewRealClient` (line 35), `sendToSignDetails` (line 70-76, поле `KzBmg` уже есть но не наполняется), `SendToSign.details := sendToSignDetails{...}` (line 105-109).
- `backend/internal/trustme/real_client_test.go` -- ~6 call-sites `NewRealClient(srv.URL, "tk", srv.Client())` (lines 37, 73, 93, 112, 139, 156, 163, 180, 188, 199) — все обновляются под новую сигнатуру; добавляем новый `t.Run` под `TestRealClient_SendToSign`.
- `backend/cmd/api/trustme.go:36` -- единственный prod call-site `trustme.NewRealClient(...)`.

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/contract/sender_service.go` -- delete const `trustMeCreatorCompanyName` (lines 36-37); in `processClaim` lift `fio := composeFIO(c.CC.CreatorLastName, c.CC.CreatorFirstName, c.CC.CreatorMiddleName)` to top of function and reuse for `ContractData.CreatorFIO` (line 284), `Requisite.FIO` (line 310), and new `Requisite.CompanyName` (line 309); in `resendOrphan` lift `fio := composeFIO(requisites.CreatorLastName, requisites.CreatorFirstName, requisites.CreatorMiddleName)` before `Requisite{}` literal and use for both `CompanyName` (line 206) and `FIO` (line 207).
- [x] `backend/internal/contract/sender_service_test.go` -- extend `mock.MatchedBy(...)` in `TestContractSenderService_RunOnce_HappyPath` (line ~164) and `TestContractSenderService_Phase0_Resend_WithRealRequisites` (line ~262) — add `in.Requisites[0].CompanyName == "Иванов Иван Иванович"` assertion alongside existing `FIO` check.
- [x] `backend/internal/config/config.go` -- add `TrustMeKzBmg bool ` env:"TRUSTME_KZ_BMG" envDefault:"false" `` field directly after `TrustMeWebhookToken` (line 74); short godoc explaining feature flag for prod-trial of TrustMe «База мобильных граждан» check.
- [x] `backend/internal/trustme/real_client.go` -- add `kzBmg bool` field to `RealClient` struct; extend `NewRealClient` signature to `(baseURL, token string, kzBmg bool, httpClient *http.Client)` and store the flag; in `SendToSign` set `details.KzBmg = c.kzBmg` (replaces zero-value default).
- [x] `backend/internal/trustme/real_client_test.go` -- update all existing `NewRealClient(...)` call-sites to pass `false` for `kzBmg`; add `t.Run("kzBmg flag flows into details JSON", ...)` under `TestRealClient_SendToSign` — `httptest.Server` captures body, `require.Contains(t, string(body), `"KzBmg":true`)` after sending with `kzBmg=true`.
- [x] `backend/cmd/api/trustme.go` (line 36) -- pass `cfg.TrustMeKzBmg` to `NewRealClient`: `trustme.NewRealClient(cfg.TrustMeBaseURL, cfg.TrustMeToken, cfg.TrustMeKzBmg, nil)`.

**Acceptance Criteria:**
- Given creator с непустыми `last_name`/`first_name`/`middle_name`, when outbox-worker отправляет договор в TrustMe (Phase 2c или Phase 0 orphan resend), then `Requisites[0].CompanyName` в payload равно `composeFIO(...)` output и совпадает с `Requisites[0].FIO`.
- Given `TRUSTME_KZ_BMG=true` в env, when backend стартует и любой договор уходит через `RealClient.SendToSign`, then в JSON multipart-field `details` присутствует `"KzBmg":true`.
- Given `TRUSTME_KZ_BMG` не задан, when backend стартует, then `cfg.TrustMeKzBmg=false` и `details.KzBmg=false` в payload (нет регрессии относительно текущего поведения).
- Given реализация завершена, when выполнить `grep -rn "Креатор" backend/internal/contract/`, then результат пуст (литерал полностью удалён, не остался в комментариях).
- Given `make lint-backend && make test-unit-backend && make test-unit-backend-coverage` локально и в CI, when запуск, then все три проходят без findings.

## Spec Change Log

<!-- empty -->

## Design Notes

**Почему `KzBmg` в `RealClient`, а не в `SendToSignInput`:** глобальный config-флаг (один на instance), симметрия с уже хранящимися `baseURL`/`token`/`limiter`. Per-request гибкость не нужна — если завтра потребуется selective enabling per креатор, refactor тривиальный (один call-site `SendToSign`).

**Почему `fio` поднимается в локальную переменную:** в `processClaim` `composeFIO(...)` зовётся дважды для одних и тех же входных данных (line 284 — `ContractData.CreatorFIO`; line 310 — `Requisite.FIO`); добавляя третий call-site (`CompanyName`) лучше один раз вычислить и переиспользовать. В `resendOrphan` пока один call-site, но добавление `CompanyName` сразу ставит вопрос дубля — поднимаем заодно.

**Prod-trial runbook (out of code):**
1. Dokploy → backend env → выставить `TRUSTME_KZ_BMG=true`. Прод либо staging с `TRUSTME_MOCK=false` + прод-токен (`config.go:55-58` допускает one-off real-test на staging).
2. Restart backend container.
3. Тестовый креатор с **заведомо левой связкой ИИН↔Phone** (реальный ИИН + чужой телефон). Кампания → creator-decision `agreed` → outbox-worker через 10s отправит договор.
4. Наблюдать `contracts.last_error_message`, spy-логи (если staging+real), TrustMe ЛК. Исходы и реакция:
   - `status="Ok"` → фича не активна или TrustMe её игнорирует. Откат флага, тикет TrustMe.
   - `status="Error"` + errorText про «требует подключения / нет тарифа» → откат, тикет TrustMe на подключение.
   - `status="Error"` + errorText про несовпадение связки → желаемый исход. Держим флаг `true`, открываем followup на дефолт `envDefault:"true"`.

**Конфликт-резолв с chunk-17:** `TrustMeWebhookToken` уже добавлен агентом chunk-17 в `config.go:74`. Наш `TrustMeKzBmg` ставится строкой ниже, конфликт мерджа с `main` после merge chunk-17 будет тривиальным (две соседние строки). `cmd/api/main.go` chunk-17 трогает (webhook handler wiring) но не `cmd/api/trustme.go` — наш единственный production call-site `NewRealClient` — без конфликта.

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: clean build после расширения `NewRealClient` сигнатуры.
- `cd backend && go test ./internal/contract/... ./internal/trustme/... -count=1 -race` -- expected: pass; новые ассерты в `sender_service_test.go` и `real_client_test.go` зелёные.
- `make test-unit-backend` -- expected: полный unit suite зелёный (включая транзитивные тесты, использующие `NewRealClient`).
- `make test-unit-backend-coverage` -- expected: per-method coverage ≥80% в `internal/contract/`, `internal/trustme/`.
- `make lint-backend` -- expected: no findings.
- `grep -rn "Креатор" backend/internal/contract/` -- expected: пусто (полное удаление литерала).
- `grep -rn "TrustMeKzBmg\|TRUSTME_KZ_BMG" backend/` -- expected: hits только в `config.go`, `cmd/api/trustme.go`, и (опционально) `.env.example` если есть; никаких hardcoded `true` в production-коде.

## Suggested Review Order

**Feature-flag (вход)**

- Новое поле + env-переменная — единый источник правды для prod-trial.
  `backend/internal/config/config.go:80`

**TrustMe-клиент: проброс флага в payload**

- `kzBmg` — config-поле клиента, не per-request (симметрия с `baseURL`/`token`/`limiter`).
  `backend/internal/trustme/real_client.go:29`
- Сигнатура `NewRealClient` расширена `kzBmg bool` без `Set*`-методов.
  `backend/internal/trustme/real_client.go:37`
- `details.KzBmg = c.kzBmg` ставится перед каждым SendToSign.
  `backend/internal/trustme/real_client.go:110`

**CompanyName=ФИО в send-местах**

- `processClaim`: `fio` поднят, переиспользован тремя call-сайтами (CreatorFIO + Requisite.FIO + CompanyName).
  `backend/internal/contract/sender_service.go:281`
- `resendOrphan`: тот же паттерн перед `Requisite{}` литералом.
  `backend/internal/contract/sender_service.go:198`
- Константа `trustMeCreatorCompanyName="Креатор"` удалена (Decision #13 обновлён).
  `backend/internal/contract/sender_service.go:35`

**Прод call-site**

- `cfg.TrustMeKzBmg` пробрасывается в `NewRealClient` третьим аргументом.
  `backend/cmd/api/trustme.go:36`

**Тесты**

- captured-input ассерт `CompanyName == "Иванов Иван Иванович"` для Phase 2c.
  `backend/internal/contract/sender_service_test.go:168`
- captured-input ассерт `CompanyName == "Иванов Иван Иванович"` для Phase 0 resend.
  `backend/internal/contract/sender_service_test.go:266`
- Inverse `"KzBmg":false` assertion в дефолтном "success parses response" — guard от случайного `omitempty`-regression.
  `backend/internal/trustme/real_client_test.go:62`
- Новый `t.Run` "kzBmg flag flows into details JSON": httptest-capture body, `Contains("KzBmg":true)`.
  `backend/internal/trustme/real_client_test.go:121`
