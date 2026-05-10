---
title: "Чанк 18 — индикатор «договор подписан» в кампании (frontend)"
type: feature
created: "2026-05-09"
status: done
baseline_commit: 417ae0560067917264822049b788c2002ada00d3
context:
  - docs/standards/frontend-components.md
  - docs/standards/frontend-testing-e2e.md
  - docs/standards/backend-codegen.md
---

> **Преамбула — стандарты обязательны.** Перед любой строкой production-кода агент полностью загружает `docs/standards/`. Все правила — hard rules. Pre-req: chunk 17 (`POST /trustme/webhook`) в main.

<frozen-after-approval>

## Intent

**Problem:** После chunk 16/17 backend возвращает `cc.status` ∈ {`signing`, `signed`, `signing_declined`}, но `web` об этих значениях не знает: OpenAPI enum `CampaignCreatorStatus` содержит только `[planned, invited, declined, agreed]`, exhaustive switches в `CampaignCreatorsSection`/`CampaignCreatorsTable` отбросят такие строки (`if (!bucket) continue`). Админ не увидит факта подписи договора.

**Approach:** Расширить enum в `backend/api/openapi.yaml` тремя значениями + `make generate-api` (атомарно обновляет всех клиентов). Во `frontend/web` дополнить `CAMPAIGN_CREATOR_GROUP_ORDER`, `groupedRows`-инициализатор, `actionForStatus`, `buildStatusColumns` (compile-time exhaustiveness потребует этого автоматически), добавить i18n-ключи. Новые группы — read-only (без mass-action и без remove, как у `agreed`). Один PR; e2e через реальный backend.

## Boundaries & Constraints

**Always:**
- Enum-расширение в `backend/api/openapi.yaml` — единственная backend-правка. `*.gen.go` / `generated/schema.ts` — только через `make generate-api` (`backend-codegen.md`).
- Compile-time exhaustiveness (`_orderIsExhaustive`, `_exhaustive: never`) — оставляем; после regen TS триггерит ошибки на трёх известных местах, не даёт забыть.
- Порядок групп: `planned → invited → declined → agreed → signing → signed → signing_declined`.
- Колонки per status: `signing` → `[decided-at]`; `signed` / `signing_declined` → `[invited-pair, decided-at]`.
- Все три новые группы — без mass-action (`actionForStatus → {}`), без `onRemove` (расширить условие, по которому `agreed` сейчас идёт без remove).
- i18n — только русский, через `react-i18next`, никаких хардкод-строк.
- E2E — через бизнес-ручки: `tma agree` → `/test/trustme/run-outbox-once` → polling до `cc.status='signing'` → `POST /trustme/webhook` (raw `Authorization: <token>`) → polling до терминала. `data-testid` — стабильный, без зависимости от копирайта.

**Ask First:**
- Если параллельный chunk 17 уже расширил enum в своём PR (merge-конфликт после rebase) — HALT, согласовать с человеком.
- Если до моего PR strict-server backend начнёт падать на «invalid enum value» в response `GET /campaigns/{id}/creators` (chunk 17 в main, enum ещё не расширен) — HALT, поднять приоритет enum-фиксу.

**Never:**
- НЕ добавлять новые поля в `CampaignCreator` (никаких `contractSignedAt`/`contractId` и т.п.).
- НЕ показывать mass-action / remove в трёх новых группах.
- НЕ менять backend state-machine.
- НЕ дёргать webhook без предварительного polling'а `cc.status='signing'` (race с outbox-worker'ом → флаки).
- НЕ коммитить и не мержить автоматически (`feedback_no_commits` / `feedback_no_merge`).

## I/O & Edge-Case Matrix

| Scenario | Backend State | Expected UI |
|----------|--------------|-------------|
| `signing` | `cc.status='signing'` | Секция «Подписывают договор», колонка `decided-at`, нет mass-action / remove. |
| `signed` | `cc.status='signed'` | Секция «Договор подписан», колонки `invited-pair` + `decided-at`, нет mass-action / remove. |
| `signing_declined` | `cc.status='signing_declined'` | Секция «Отказались от договора», те же колонки, что у `signed`. |
| Все 7 статусов | По одной строке на каждый | 7 секций в pipeline-порядке. |
| Validation 422 на `planned`-mass-notify, среди `currentStatus` есть новый | Backend → 422 `CAMPAIGN_CREATOR_BATCH_INVALID` с `currentStatus: signing/signed/signing_declined` | Inline-баннер показывает «Подписывает договор» / «Подписал(а) договор» / «Отказал(ась) от договора» (через `currentStatus.{status}` локаль). |

</frozen-after-approval>

## Code Map

**Изменяем:**
- `backend/api/openapi.yaml` — enum `CampaignCreatorStatus` → `[planned, invited, declined, agreed, signing, signed, signing_declined]`; в `description` дописать строки про новые статусы.
- `frontend/web/src/shared/constants/campaignCreatorStatus.ts` — добавить `SIGNING/SIGNED/SIGNING_DECLINED` в `CAMPAIGN_CREATOR_STATUS` const-объект и в `CAMPAIGN_CREATOR_GROUP_ORDER` (хвост).
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` — 6 ключей: `campaignCreators.groups.{signing|signed|signing_declined}` (`Подписывают договор` / `Договор подписан` / `Отказались от договора`) и `campaignCreators.currentStatus.{signing|signed|signing_declined}` (`Подписывает договор` / `Подписал(а) договор` / `Отказал(ась) от договора`).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` — расширить `groupedRows` accumulator тремя ключами; в `actionForStatus` для трёх новых статусов вернуть `{}`; обновить условие, по которому `onRemove` НЕ передаётся (сейчас `status === AGREED`) → расширить на четыре статуса (через const-set `STATUSES_WITHOUT_REMOVE` либо явный `||`).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` — в `buildStatusColumns` добавить три case'а: `SIGNING` → `[decidedColumn(...)]`; `SIGNED` и `SIGNING_DECLINED` → `[pairColumn("invited", ...), decidedColumn(...)]`.

**Регенерируются (`make generate-api`, не править вручную):**
- `backend/internal/api/server.gen.go`, `backend/e2e/{apiclient,testclient}/types.gen.go`
- `frontend/{web,tma,landing}/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`

**Не изменяется:**
- `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx` — опциональные `onRemove` / `onSubmit` / `actionLabel` уже корректно сваливают компонент в read-only render.

**Тесты (frontend unit):**
- `CampaignCreatorsSection.test.tsx` — `groupedRows` распределяет 7 статусов; рендер новой группы показывает `data-testid="campaign-creators-group-signing"` (и т.п.) без `-action-`-кнопки; `onRemove` не передаётся в строки.
- `CampaignCreatorsTable.test.tsx` — три новых `it` под `buildStatusColumns`.
- `CampaignCreatorGroupSection.test.tsx` — расширить тест с `validationDetails[].currentStatus` ∈ {signing, signed, signing_declined}, ассерт локализованного текста.

**Тесты (Playwright e2e через реальный backend):**
- `frontend/e2e/helpers/api.ts` — добавить `tmaAgreeCampaign` (POST `/tma/campaigns/{secretToken}/agree` с `Authorization: tma <signedInitData>`; signing init-data — переиспользовать существующий TMA-helper или его аналог), `runTrustMeOutboxOnce` (POST `/test/trustme/run-outbox-once`, ассерт 204), `findTrustMeSpyByIIN` (GET `/test/trustme/spy-list` + фильтр), `triggerTrustMeWebhook(contractId, status, token)` (POST `/trustme/webhook` с `Authorization: Bearer <token>` — middleware ожидает Bearer scheme per RFC 6750, как реальный TrustMe-кабинет; payload `{contract_id, status, client:"", contract_url:""}`), `waitForCcStatus(campaignId, creatorId, expectedStatus, timeoutMs=5000)` (polling `GET /campaigns/{id}/creators` каждые 200мс).
- `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts` (новый) — два теста под `test.describe("Admin campaign creators TrustMe states — chunk 18")`:
  - `signing → signed`: setup admin + creator + campaign + uploadDummyContractTemplate → addCampaignCreators → notifyAsAdmin → tmaAgreeCampaign → runTrustMeOutboxOnce → waitForCcStatus(`signing`) → goto `/campaigns/:id` → expect group-signing visible, no action button → findTrustMeSpyByIIN (documentId) → triggerTrustMeWebhook(documentId, 3) → waitForCcStatus(`signed`) → expect group-signed visible.
  - `signing → signing_declined`: симметрично, webhook со status 9 → expect group-signing_declined visible.

## Tasks & Acceptance

**Execution (порядок по зависимостям):**
- [x] `backend/api/openapi.yaml` — enum + description.
- [x] `make generate-api` — все клиенты регенерируются.
- [x] `frontend/web/src/shared/constants/campaignCreatorStatus.ts` — три значения.
- [x] `frontend/web/src/shared/i18n/locales/ru/campaigns.json` — 6 ключей.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` — `buildStatusColumns` (триггерится `_exhaustive: never`).
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` — `groupedRows`, `actionForStatus`, условие `onRemove`.
- [x] Unit-тесты (`*.test.tsx`) расширены.
- [x] `frontend/e2e/helpers/api.ts` — пять helper'ов.
- [x] `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts` — два теста.
- [x] `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend` — всё зелёное.

**Acceptance Criteria:**
- Given кампания с креатором в каждом из 7 статусов, when admin открывает `/campaigns/:id`, then видно 7 секций в pipeline-порядке.
- Given креатор в `signing`, when страница отрендерилась, then в строке колонка `decided-at` непустая; mass-action и remove отсутствуют.
- Given креатор в `signed` / `signing_declined`, when страница отрендерилась, then видно `invited-pair` + `decided-at`; mass-action / remove отсутствуют.
- Given mass-notify по `planned` с одним участником в `signing` (или `signed` / `signing_declined`), when backend вернул 422 `CAMPAIGN_CREATOR_BATCH_INVALID`, then в баннере секции видно строку `<имя> → Подписывает договор` (соотв. локаль для других значений).
- Given e2e flow `tma agree → run-outbox-once → waitForCcStatus("signing") → triggerTrustMeWebhook(documentId, 3) → waitForCcStatus("signed")`, when страница после refetch обновилась, then `data-testid="campaign-creators-group-signed"` видна. Симметрично для status=9 → `signing_declined`.
- `npx tsc --noEmit` (через `make lint-web`) — clean: `_orderIsExhaustive` и `_exhaustive: never` подтверждают, что все 7 enum-значений обработаны.
- `make test-unit-web` зелёный; `make test-e2e-frontend` зелёный (оба новых теста + не сломаны существующие campaign-notify spec'ы).

## Design Notes

**Compile-time guard основной.** После regen `CampaignCreatorStatus` = string-union из 7 значений. TS немедленно ругается:
1. `_orderIsExhaustive` в `campaignCreatorStatus.ts` — пока в `CAMPAIGN_CREATOR_GROUP_ORDER` не добавлены новые значения.
2. `_exhaustive: never = status` в `buildStatusColumns` — пока в switch нет трёх новых case'ов.
Сборка не пройдёт, пока правки не выполнены — забыть невозможно.

**Webhook-payload в e2e.** Поле `contract_id` в webhook payload — это TrustMe-side document identifier (`contracts.trustme_document_id`), не наш `contracts.id`. Достаём через `findTrustMeSpyByIIN(...).documentId`. Поля `client` / `contract_url` backend игнорирует (PII не пишем) — отдаём пустые строки.

**Polling — необходим.** Переход `agreed → signing` происходит синхронно внутри `RunTrustMeOutboxOnce` (по контракту chunk 16), но read-after-write через отдельное HTTP-чтение `GET /campaigns/{id}/creators` имеет миллисекундный лаг. `waitForCcStatus` (polling 200мс, лимит 5с) страхует от флаков и от случая, когда worker не подобрал ряд — упадёт с явным сообщением, указывающим на backend-баг, а не на тестовую нестабильность.

## Verification

**Commands:**
- `make generate-api` — diff только в `*.gen.go` / `generated/schema.ts`.
- `make build-web` — TS-сборка clean (exhaustive checks триггерятся).
- `make lint-web` — tsc + eslint clean.
- `make test-unit-web` — vitest зелёный.
- `make test-e2e-frontend` — `admin-campaign-creators-trustme.spec.ts` оба теста зелёные.

**Manual checks:**
- Открыть `/campaigns/:id` для кампании с креаторами в новых статусах — порядок секций и тексты соответствуют решениям.
- DevTools Network: `GET /campaigns/{id}/creators` 200, нет `console.error` про unknown enum.

## Suggested Review Order

**Контракт (entry)**

- Расширение enum — единственная backend-правка. Описание добавлено для каждого нового state.
  [`openapi.yaml:3352`](../../backend/api/openapi.yaml#L3352)

**Constants & exhaustive guards**

- Группа `CAMPAIGN_CREATOR_GROUP_ORDER` обновлена; `_orderIsExhaustive` гарантирует compile-time fail при будущих расширениях enum.
  [`campaignCreatorStatus.ts:15`](../../frontend/web/src/shared/constants/campaignCreatorStatus.ts#L15)

- `buildStatusColumns` — switch с `_exhaustive: never` заставил добавить три case'а. SIGNED/SIGNING_DECLINED делят layout с DECLINED/AGREED.
  [`CampaignCreatorsTable.tsx:305`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L305)

- `actionForStatus` переписан в exhaustive switch — теперь добавление нового статуса даст compile-time error, как у `buildStatusColumns`.
  [`CampaignCreatorsSection.tsx:320`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L320)

**UI rendering**

- `groupedRows`-accumulator — Record-тип заставляет TS заполнить все 7 ключей.
  [`CampaignCreatorsSection.tsx:117`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L117)

- `STATUSES_WITHOUT_REMOVE` — read-only status set. Заменил inline-проверку `=== AGREED`, расширяется одной строкой.
  [`CampaignCreatorsSection.tsx:39`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L39)

**i18n**

- 6 новых ключей: `groups.{signing|signed|signing_declined}` для заголовков секций и `currentStatus.{...}` для inline-422-баннера.
  [`campaigns.json:99`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L99)

**E2E coverage**

- Pipeline `notify → tma agree → outbox-once → webhook(3|9)` через бизнес-ручки + раздельный poll до каждого ожидаемого статуса.
  [`admin-campaign-creators-trustme.spec.ts:97`](../../frontend/e2e/web/admin-campaign-creators-trustme.spec.ts#L97)

- 5 helper'ов: tma-agree, run-outbox-once, spy-by-iin, webhook (Bearer scheme — соответствует middleware), polling status.
  [`api.ts:954`](../../frontend/e2e/helpers/api.ts#L954)

**Unit coverage**

- 7-status pipeline rendering + read-only assertions для трёх новых секций.
  [`CampaignCreatorsSection.test.tsx`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx)

- 3 case'а под `buildStatusColumns`: signing (только decided), signed/signing_declined (invited-pair + decided).
  [`CampaignCreatorsTable.test.tsx`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx)

- 422-баннер ассертит локализованные тексты для всех трёх новых `currentStatus`.
  [`CampaignCreatorGroupSection.test.tsx`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.test.tsx)
