---
title: 'E2E cleanup leaks — campaign_creators и contracts'
type: bugfix
created: '2026-05-10'
status: done
baseline_commit: 8d5ee2f6172e3a215ccdbc78087dd54cb26ffc0e
context:
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-codegen.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** После прогона E2E тестов на staging в БД остаются `campaign_creators` и `contracts` строки. `RegisterCampaignCreatorCleanup` использует production DELETE, который блокирует удаление при terminal-статусах (agreed/signing/signed/declined) — 422 молча логируется через `t.Logf`. `contracts` таблица вообще не имеет cleanup-инфраструктуры. `CampaignRepo.DeleteForTests` делает простой DELETE без каскада — FK блокирует удаление кампании при наличии campaign_creators строк.

**Approach:** Добавить `campaign_creator` и `contract` в testapi `CleanupEntityType` enum; реализовать hard-delete методы в repo-слое (bypassing business-layer guards); переключить `RegisterCampaignCreatorCleanup` на testapi endpoint; добавить `RegisterContractCleanup` с вызовом из `SetupCampaignWithSigningCreator`; починить каскад в `CampaignRepo.DeleteForTests`.

## Boundaries & Constraints

**Always:**
- Compound ID для campaign_creator cleanup: `"campaignID:creatorID"` (двоеточие-разделитель), парсить через `strings.SplitN(id, ":", 2)`
- LIFO-порядок cleanup в `SetupCampaignWithSigningCreator`: contract → campaign_creator → creator → creator_application → campaign
- Фактическое имя метода в RepoFactory — `NewContractsRepo` (не `NewContractRepo`)
- FK `campaign_creators.contract_id → contracts ON DELETE SET NULL`: при удалении contracts Postgres сам обнулит ссылку — дополнительных шагов не нужно

**Ask First:**
- Если при реализации обнаружатся FK-зависимости на contracts кроме campaign_creators.contract_id

**Never:**
- Изменять production-логику `RemoveCampaignCreator` сервиса
- DDL-миграции — только application-level код
- Трогать frontend E2E cleanup (removeCampaignCreator в helpers/api.ts) — frontend тесты не доводят campaign_creators до terminal-статусов

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Cleanup campaign_creator в terminal status | `Type=campaign_creator, Id="campID:creatorID"`, status=agreed | 204, строка удалена | N/A |
| Cleanup campaign_creator — строки нет | `Type=campaign_creator, Id="campID:creatorID"`, row absent | 404 (sql.ErrNoRows → ErrNotFound) | RegisterCampaignCreatorCleanup принимает 404 как success |
| Cleanup contract | `Type=contract, Id=uuid`, row существует | 204, contracts удалён, campaign_creators.contract_id SET NULL | N/A |
| Неверный формат ID | `Type=campaign_creator, Id="single-uuid"` | 400 ValidationError | — |

</frozen-after-approval>

## Code Map

- `backend/api/openapi-test.yaml:398` — CleanupEntityType enum: добавить `campaign_creator`, `contract`
- `backend/internal/repository/campaign_creator.go:82–97` — CampaignCreatorRepo interface: добавить `DeleteForTests`
- `backend/internal/repository/contracts.go:139–150` — ContractRepo interface: добавить `DeleteForTests`
- `backend/internal/repository/campaign.go:93, 324–337` — CampaignRepo interface + DeleteForTests: исправить каскад
- `backend/internal/handler/testapi.go:38–44` — TestAPICleanupRepoFactory: +NewCampaignCreatorRepo, +NewContractsRepo
- `backend/internal/handler/testapi.go:186–208` — CleanupEntity switch: +case campaign_creator, +case contract
- `backend/e2e/testutil/campaign_creator.go` — RegisterCampaignCreatorCleanup: полная замена (убрать c/adminToken, использовать testclient)
- `backend/e2e/testutil/contract_setup.go` — RegisterContractCleanup: новый хелпер + вызов в SetupCampaignWithSigningCreator
- Callers ×9: `testutil/tma.go:124`, `tma/tma_test.go:393`, `campaign_creator/campaign_creator_test.go:219,252,388,505`, `campaign_creator/campaign_notify_test.go:116`, `creator_applications/approve_with_campaigns_test.go:119,120`

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi-test.yaml` -- добавить `campaign_creator` и `contract` в CleanupEntityType enum -- позволяет testapi принимать эти типы
- [x] `make generate-api` -- регенерация testapi/server.gen.go, testclient/types.gen.go, frontend/e2e/types/test-schema.ts -- обновляет сгенерированный код
- [x] `backend/internal/repository/campaign_creator.go` -- переиспользован существующий `DeleteByCampaignAndCreatorForTests(ctx, campaignID, creatorID)` (уже служил force-cleanup endpoint'у). Дубликат не плодим — по стандартам.
- [x] `backend/internal/repository/contracts.go` -- добавлен `DeleteForTests(ctx, id)` в интерфейс и реализацию (`DELETE FROM contracts WHERE id=$1`); FK ON DELETE SET NULL обнулит campaign_creators.contract_id автоматически
- [x] `backend/internal/repository/campaign.go` -- исправлен `DeleteForTests`: сначала `DELETE FROM campaign_creators WHERE campaign_id=$1`, затем `DELETE FROM campaigns WHERE id=$1`; handler оборачивает в `dbutil.WithTx` для атомарности
- [x] `backend/internal/handler/testapi.go` -- `NewCampaignCreatorRepo` уже был в TestAPICleanupRepoFactory; добавлен `NewContractsRepo`; добавлены case `testapi.CampaignCreator` (compound id parse) и `testapi.Contract` в CleanupEntity switch
- [x] `make generate-mocks` -- регенерация моков после изменения интерфейсов
- [x] `backend/e2e/testutil/campaign_creator.go` -- RegisterCampaignCreatorCleanup переписан под новую сигнатуру `(t, campaignID, creatorID)`; вызывает `tc.CleanupEntityWithResponse` с `Type: testclient.CampaignCreator, Id: campaignID+":"+creatorID`
- [x] `backend/e2e/testutil/contract_setup.go` -- добавлен `RegisterContractCleanup(t, contractID)`; вызывается в `SetupCampaignWithSigningCreator` после `require.NotEmpty(t, rec.AdditionalInfo, ...)`
- [x] Callers ×10 (на одного больше, чем в спеке — нашёлся ещё один в `campaign_creator_test.go:674`): `testutil/tma.go:124`, `tma/tma_test.go:400`, `campaign_creator/campaign_creator_test.go:239,272,408,525,674`, `campaign_creator/campaign_notify_test.go:134`, `creator_applications/approve_with_campaigns_test.go:119,120`
- [x] Тесты: `TestCampaignRepository_DeleteForTests` обновлён под двухстатементный каскад; добавлен `TestContractRepository_DeleteForTests`; добавлены handler-тесты для type=campaign_creator (success, compound id without colon, empty half, not found) и type=contract (success, not found)
- [x] `make test-unit-backend` и `make test-unit-backend-coverage` — оба прошли; `make lint-backend` — 0 issues

**Acceptance Criteria:**
- Given E2E тест доводит campaign_creator до status=agreed/signing/signed, when тест завершается, then campaign_creator строка отсутствует в БД (cleanup вернул 204)
- Given SetupCampaignWithSigningCreator создал contracts строку, when тест завершается, then contracts строка отсутствует в БД, campaign_creators.contract_id=NULL
- Given кампания с campaign_creators строками, when CleanupEntity type=campaign вызван, then campaign_creators удалены каскадом, кампания удалена без FK-ошибки
- Given CleanupEntity type=campaign_creator с Id без двоеточия, when вызван, then 400 ValidationError

## Design Notes

Compound ID `"campaignID:creatorID"` — callers всегда имеют оба значения; `campaign_creator.id` (UUID) недоступен на стороне тестов при части путей (approve_with_campaigns_test). UUID не содержит `:` — разделитель безопасен.

LIFO в SetupCampaignWithSigningCreator (last registered = first fired):
1. `RegisterContractCleanup` (регистрируется последним → fires first) — удаляет contracts строку, SET NULL на campaign_creators.contract_id
2. `RegisterCampaignCreatorCleanup` — hard-delete campaign_creator строки
3. `RegisterCreatorCleanup`
4. `RegisterCreatorApplicationCleanup`
5. `RegisterCampaignCleanup` (регистрируется первым → fires last) — кампания удаляется без FK-блокировки

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: no compilation errors
- `cd backend && go vet ./...` -- expected: no warnings
- `cd backend && make generate-api && git diff --stat -- '*.gen.go' 'test-schema.ts'` -- expected: no diff (сгенерированный код актуален)

## Suggested Review Order

**Контракт testapi (point of entry)**

- Расширили enum CleanupEntityType двумя новыми типами — отсюда расходится весь wire-up.
  [`openapi-test.yaml:466`](../../backend/api/openapi-test.yaml#L466)

**Test-only cleanup dispatch + cascade**

- Switch новых типов в handler: compound-id parser для campaign_creator и delegation для contract; campaign-case теперь обёрнут в WithTx из-за двухстатементного каскада.
  [`testapi.go:204`](../../backend/internal/handler/testapi.go#L204)

- RepoFactory интерфейс расширен `NewContractsRepo` — handler получает доступ к contracts.
  [`testapi.go:40`](../../backend/internal/handler/testapi.go#L40)

- Каскадное удаление: сначала drain `campaign_creators`, потом DELETE `campaigns`; FK без ON DELETE CASCADE.
  [`campaign.go:355`](../../backend/internal/repository/campaign.go#L355)

- Новый `DeleteForTests` для contracts: FK на campaign_creators.contract_id с ON DELETE SET NULL делает каскад прозрачным.
  [`contracts.go:512`](../../backend/internal/repository/contracts.go#L512)

**E2E testutil — register cleanup**

- RegisterCampaignCreatorCleanup переписан под новый testapi endpoint; уходит admin-token / production-DELETE с 422 на terminal-статусах.
  [`campaign_creator.go:93`](../../backend/e2e/testutil/campaign_creator.go#L93)

- Новый RegisterContractCleanup; используется в SetupCampaignWithSigningCreator сразу после получения contract_id, до промежуточных require, чтобы panic между ними не приводил к leak (правка после review).
  [`contract_setup.go:28`](../../backend/e2e/testutil/contract_setup.go#L28)

- Регистрация contract cleanup в фикстуре подписания (LIFO: fires first перед campaign_creator cleanup).
  [`contract_setup.go:162`](../../backend/e2e/testutil/contract_setup.go#L162)

**Тесты**

- Unit-тест `CampaignRepository.DeleteForTests` обновлён под двухстатементный каскад (success / no rows / drain failure / partial failure).
  [`campaign_test.go:728`](../../backend/internal/repository/campaign_test.go#L728)

- Новые unit-тесты `ContractRepository.DeleteForTests` (success / 404 / DB error).
  [`contracts_test.go:594`](../../backend/internal/repository/contracts_test.go#L594)

- Handler-тесты: campaign теперь через WithTx, добавлены case'ы campaign_creator (success / no colon / empty half / not found) и contract.
  [`testapi_test.go:287`](../../backend/internal/handler/testapi_test.go#L287)

**Callers (peripherals)**

- Все ×10 вызовов RegisterCampaignCreatorCleanup перешли на новую сигнатуру `(t, campaignID, creatorID)` — admin-client/token больше не передаются.
  [`tma.go:124`](../../backend/e2e/testutil/tma.go#L124)
