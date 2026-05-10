---
title: 'UTM-метки в заявках креаторов'
type: feature
created: '2026-05-10'
status: done
baseline_commit: 2ac9a69e0b0661f2c79d253302c2ea8aa122ce4a
context:
  - docs/standards/backend-codegen.md
  - docs/standards/backend-repository.md
  - docs/standards/backend-transactions.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Маркетинг хочет различать источники трафика заявок (чаты/рассылки), но лендинг не сохраняет UTM из URL и не передаёт их в submit. Админ при модерации не видит, откуда пришёл креатор.

**Approach:** Лендинг ловит 5 стандартных `utm_*` из query-string, кэширует в `sessionStorage` (last-click), отправляет в payload. Бэк добавляет 5 nullable text-колонок в `creator_applications`, отдаёт их в `CreatorApplicationDetailData`. Drawer админа показывает блок «Источник трафика» с непустыми парами между категориями и социалками.

## Boundaries & Constraints

**Always:**
- 5 flat camelCase полей `utm{Source,Medium,Campaign,Term,Content}` в API: `nullable: true`, `maxLength: 256`.
- Last-click: новые UTM из URL перезаписывают `sessionStorage.ugc_utm`; reload без UTM не очищает storage.
- В `CreatorApplicationDetailData` все 5 полей возвращаются всегда (null если пусто).
- Drawer-секция рендерится только если хотя бы одно UTM != null; внутри — только непустые пары.
- Audit `details` события `creator_application.submit` включает только non-null UTM-ключи (плоские: `utm_source`, ...).
- Handler-хелпер `nilIfEmpty(*string)` (trim+empty→nil) применяется к каждому UTM при маппинге request→domain.

**Never:**
- НЕ показывать UTM в `CreatorApplicationListItem`, counts, TMA.
- НЕ менять `openapi-test.yaml`.
- НЕ добавлять CHECK или индексы в миграции.
- НЕ редактировать существующие миграции in-place — только новый goose-файл.

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected | Error Handling |
|---|---|---|---|
| Полный набор UTM | `?utm_source=chat&utm_medium=tg&utm_campaign=spring&utm_term=ugc&utm_content=banner` → submit | 5 значений в БД, detail-API отдаёт 5 строк, audit `details` 5 ключей | N/A |
| Прямой заход | URL без `utm_*`, sessionStorage пуст | 5 NULL, detail отдаёт 5 null, audit без UTM | N/A |
| Partial UTM | `?utm_source=fb&utm_campaign=q2` | source/campaign — значения, term/medium/content — NULL; audit — 2 ключа | N/A |
| UTM > 256 | API submit с `utmSource` 257 chars | 422 ValidationError | strict-server по схеме |
| Whitespace UTM | API submit с `utmSource: "   "` | NULL в БД, domain получает nil | handler `nilIfEmpty` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` — `CreatorApplicationSubmitRequest` (~ 2254) и `CreatorApplicationDetailData` (~ 2428): +5 nullable string по образцу `address`. `CreatorApplicationListItem` НЕ трогать.
- `backend/migrations/<ts>_creator_applications_utm.sql` — новая goose-миграция: `ALTER TABLE creator_applications ADD COLUMN utm_{key} TEXT` × 5; Down — симметричный `DROP COLUMN`.
- `backend/internal/domain/creator_application.go` — `CreatorApplicationInput` +5 `*string` (`UTM{Source,Medium,Campaign,Term,Content}`).
- `backend/internal/handler/creator_application.go` — локальный `nilIfEmpty(*string) *string`; маппинг 5 UTM из request→domain через него.
- `backend/internal/repository/creator_application.go` — `CreatorApplicationRow` +5 `*string` с тегами `db:"utm_*"` + `insert:"utm_*"`; +5 констант `CreatorApplicationColumnUTM*`. `selectColumns`/`InsertMapper` подхватятся через stom.
- `backend/internal/service/creator_application.go` — проброс UTM в Row без preprocessing; `auditNewValue` conditional add только non-null UTM.
- `backend/internal/{handler,service,repository}/creator_application_test.go` — расширения unit-тестов под UTM (см. Tasks).
- `backend/e2e/creator_application/creator_application_test.go` — расширить `TestSubmitCreatorApplication`: `t.Run("with utm")` (detail+audit) + базовый без UTM.
- `frontend/landing/src/lib/utm.ts` (новый) — `captureUTM()` (parse `window.location.search` → `sessionStorage.ugc_utm` JSON если есть хотя бы один key), `readUTM()` (return `Partial<Record<UtmKey, string>>`).
- `frontend/landing/src/lib/utm.test.ts` (новый, vitest) — полный/partial/empty/last-click/reload.
- `frontend/landing/src/pages/index.astro` — `captureUTM()` на DOMContentLoaded; в `collectFormData` дочитать `readUTM()` и добавить непустые UTM в payload (camelCase ключи).
- `frontend/e2e/landing/submit.spec.ts` — кейс с `?utm_source=test_chat&utm_campaign=spring`: submit → проверить detail через `helpers/api.ts`.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx` — секция «Источник трафика» между `categoryOtherText` и `socials`; conditional render; `data-testid="utm-section"` + `utm-{key}-value`; i18n `t("drawer.utm.*")`.
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — `drawer.utm.{title,source,medium,campaign,term,content}` с русскими лейблами.
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.test.tsx` — 3 кейса: все null → нет секции; full → 5 строк; partial → только непустые.

## Tasks & Acceptance

**Execution:**
- [x] OpenAPI: 5 nullable string полей в Submit/Detail (`maxLength: 256`); `make generate-api`.
- [x] Миграция через `make migrate-create NAME=creator_applications_utm`; Up/Down × 5 `ADD/DROP COLUMN`.
- [x] Domain `CreatorApplicationInput` +5 `*string`.
- [x] Handler: `nilIfEmpty` хелпер + маппинг 5 UTM.
- [x] Repository: Row +5 `*string` UTM + 5 констант.
- [x] Service: проброс UTM в Row; `auditNewValue` conditional add non-null.
- [x] `make generate-mocks` (только если интерфейсы изменились).
- [x] Handler unit: captured-input UTM (полный/partial/none/whitespace→nil).
- [x] Service unit: `JSONEq` на audit `details` с UTM в 3 вариантах.
- [x] Repository unit: SQL+args для INSERT и round-trip RowScan.
- [x] Backend e2e: расширить `TestSubmitCreatorApplication` с UTM-кейсом (detail + `AssertAuditEntry`).
- [x] Landing `lib/utm.ts` + `lib/utm.test.ts` (vitest).
- [x] Landing `index.astro`: `captureUTM` на DOMContentLoaded + интеграция `readUTM` в `collectFormData`.
- [x] Landing e2e: кейс с UTM в URL → submit → detail.
- [x] Web Drawer: секция «Источник трафика» + 3 unit-кейса.
- [x] Web i18n `creatorApplications.json`: `drawer.utm.*` ключи.

**Acceptance Criteria:**
- Given лендинг без UTM, when submit, then 5 utm-колонок NULL и audit `details` без UTM-ключей.
- Given лендинг с `?utm_source=chat`, when submit, then `utm_source='chat'`, остальные NULL; detail-API: `utmSource="chat"`; audit `details = {"utm_source":"chat"}`.
- Given API submit с `utmSource: "   "`, when handler, then domain-input nil и БД-колонка NULL.
- Given drawer для заявки с одним non-null UTM, when админ смотрит детали, then секция рендерится с одной парой.
- Given drawer для заявки без UTM, when админ смотрит детали, then секция отсутствует в DOM.
- `make test-unit-backend-coverage` / `make lint-web` / `make lint-landing` / `make test-unit-web` / `make test-unit-landing` / `make test-e2e-backend` / `make test-e2e-landing` — все зелёные.

## Verification

**Commands:**
- `cd backend && go build ./...` — 0 ошибок.
- `make generate-api && git status --short -- '*.gen.go' '*/schema.ts' '*/test-schema.ts'` — diff только под yaml-правки.
- `make test-unit-backend-coverage` — gate зелёный.
- `cd frontend/web && npx tsc --noEmit && npx eslint src/` — 0 ошибок.
- `cd frontend/landing && npx tsc --noEmit && npx eslint src/` — 0 ошибок.
- `make test-unit-web` / `make test-unit-landing` / `make test-e2e-backend` / `make test-e2e-landing` — зелёные.

**Manual checks:**
- Лендинг с `?utm_source=test&utm_campaign=spring2026` → DevTools → Session Storage → `ugc_utm` JSON содержит эти поля.
- `/creators/applications/{id}` под админом возвращает 5 UTM-полей.
- Drawer этой заявки → секция «Источник трафика» с двумя строками.

## Spec Change Log

- **2026-05-10 (review iteration 1)**: добавлен handler-level `validateUTMField`
  с длиной ≤ 256 рун и отказом на ASCII control chars (включая NUL, который
  Postgres TEXT не примет). Триггер — finding ревью «maxLength=256 не enforce
  oapi-codegen'ом, прямой API-вызов сохранит мегабайты в TEXT-колонку».
  Сменён хелпер `nilIfBlank` → `validateUTMField` (имя `nilIfEmpty` из
  изначального intent поглощено в более широкий валидатор; trim+empty→nil
  семантика сохранена). KEEP: granular `domain.CodeValidation` ошибки с
  именем поля; pointer.ToString вместо инлайн-`&trimmed`.

## Suggested Review Order

**Контракт и схема данных**

- Точка входа: пять nullable `utm*` строк добавлены в Submit/Detail (List не тронут).
  [`openapi.yaml:2330`](../../backend/api/openapi.yaml#L2330)

- 5 nullable TEXT-колонок без CHECK/индексов; Down симметричен.
  [`creator_applications_utm.sql:1`](../../backend/migrations/20260510061843_creator_applications_utm.sql#L1)

**Backend: handler-валидация и проброс**

- Точка входа handler: пять последовательных `validateUTMField` перед маппингом в domain.
  [`creator_application.go:42`](../../backend/internal/handler/creator_application.go#L42)

- Валидатор: trim → empty=nil → control-chars 422 → maxLen 256 → trimmed pointer.
  [`creator_application.go:87`](../../backend/internal/handler/creator_application.go#L87)

- Detail-маппер прокидывает 5 UTM в API-ответ.
  [`creator_application.go:215`](../../backend/internal/handler/creator_application.go#L215)

- `CreatorApplicationInput` += 5 `*string`.
  [`creator_application.go:194`](../../backend/internal/domain/creator_application.go#L194)

- Сервис: проброс UTM в Row + conditional add только non-null UTM в audit `details`.
  [`creator_application.go:216`](../../backend/internal/service/creator_application.go#L216)
  [`creator_application.go:511`](../../backend/internal/service/creator_application.go#L511)

- `CreatorApplicationDetail` += 5 UTM, маппер из Row → Detail.
  [`creator_application.go:773`](../../backend/internal/service/creator_application.go#L773)

- Repository: Row + 5 констант `CreatorApplicationColumnUTM*`; stom подхватит поля автоматически.
  [`creator_application.go:42`](../../backend/internal/repository/creator_application.go#L42)
  [`creator_application.go:60`](../../backend/internal/repository/creator_application.go#L60)

**Лендинг: capture / persist / send**

- Точка входа: модуль `lib/utm.ts` — last-click `captureUTM` + `readUTM`.
  [`utm.ts:1`](../../frontend/landing/src/lib/utm.ts#L1)

- Вызов `captureUTM()` на загрузке скрипта + интеграция `readUTM()` в `collectFormData`.
  [`index.astro:660`](../../frontend/landing/src/pages/index.astro#L660)

**Web admin Drawer + i18n**

- Условный рендер секции «Источник трафика» с testid'ами на значения.
  [`ApplicationDrawer.tsx:506`](../../frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx#L506)

- 6 i18n-ключей под `drawer.utm`.
  [`creatorApplications.json:27`](../../frontend/web/src/shared/i18n/locales/ru/creatorApplications.json#L27)

**Тесты**

- Handler unit: full / partial / whitespace→nil / oversize→422 / control-char→422.
  [`creator_application_test.go:186`](../../backend/internal/handler/creator_application_test.go#L186)

- Service unit: `mock.MatchedBy` + `jsonEqRaw` для audit details (3 варианта).
  [`creator_application_test.go:683`](../../backend/internal/service/creator_application_test.go#L683)

- Repository unit: SQL + args для INSERT, RETURNING-проекция, round-trip RowScan UTM.
  [`creator_application_test.go:228`](../../backend/internal/repository/creator_application_test.go#L228)

- Backend e2e: UTM round-trip через DB + audit `details`; базовый submit без UTM ассертит отсутствие ключей.
  [`creator_application_test.go:144`](../../backend/e2e/creator_application/creator_application_test.go#L144)
  [`creator_application_test.go:343`](../../backend/e2e/creator_application/creator_application_test.go#L343)

- Landing vitest: capture/read 12 кейсов (full/partial/empty/last-click/перезапись/storage-throw).
  [`utm.test.ts:1`](../../frontend/landing/src/lib/utm.test.ts#L1)

- Landing e2e: `?utm_source=test_chat&utm_campaign=spring2026` → submit → admin GET detail.
  [`submit.spec.ts:200`](../../frontend/e2e/landing/submit.spec.ts#L200)

- Drawer unit: 3 кейса (все null → нет секции, full → 5 строк, partial → только непустые).
  [`ApplicationDrawer.test.tsx:195`](../../frontend/web/src/features/creatorApplications/components/ApplicationDrawer.test.tsx#L195)
