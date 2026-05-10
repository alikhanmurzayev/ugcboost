---
title: "Intent: chunk 18 — extend campaign creators UI with new statuses"
type: intent
status: draft
created: "2026-05-09"
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
parent_intent: _bmad-output/implementation-artifacts/intent-trustme-contract-v2.md
related_specs:
  - _bmad-output/implementation-artifacts/spec-17-trustme-webhook.md
  - _bmad-output/implementation-artifacts/archive/2026-05-09-spec-16-trustme-outbox-worker.md
---

# Intent: chunk 18 — extend campaign creators UI with new statuses

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/`. Применимы все. Особенно: `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`, `naming.md`, `security.md`, `backend-codegen.md` (для regen-pipeline), `review-checklist.md`. Каждое правило — hard rule.

Pre-req: chunk 17 (`POST /trustme/webhook`) в main. До тех пор ветка живёт параллельно chunk 17 (тот же бранч `alikhan/chunk-17-trustme-webhook`) и пишется только intent + spec; код стартует когда chunk 17 смержен.

## Тезис

Chunk 18 — расширить страницу кампании в `web` тремя новыми статус-группами (`signing`, `signed`, `signing_declined`), которые backend начинает возвращать после chunk 16/17. Минимальная backend-правка: дополнить enum `CampaignCreatorStatus` в `backend/api/openapi.yaml` + `make generate-api` (обновятся клиенты web/tma/landing/e2e + `internal/api/server.gen.go`); основная работа — фронт `frontend/web/src/features/campaigns/creators/`: новые ключи i18n, дополнение `CAMPAIGN_CREATOR_GROUP_ORDER`, новые ветки в exhaustive switch'ах (`buildStatusColumns`, `actionForStatus`, `groupedRows`-инициализатор), unit-тесты + Playwright e2e через реальный backend.

## Скоуп

**В чанк 18 входит:**

- `backend/api/openapi.yaml`: enum `CampaignCreatorStatus` расширяется до `[planned, invited, declined, agreed, signing, signed, signing_declined]`. Обновляется `description` (добавить три новые строки про signing/signed/signing_declined). `make generate-api` регенерирует все клиенты + backend `internal/api/server.gen.go` + e2e clients.
- `frontend/web/src/shared/constants/campaignCreatorStatus.ts`: добавить три значения в `CAMPAIGN_CREATOR_STATUS` const-объект и в `CAMPAIGN_CREATOR_GROUP_ORDER`. Compile-time exhaustiveness-check (`MissingStatus`/`_orderIsExhaustive`) автоматически потребует этого после regen.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json`: добавить ключи `campaignCreators.groups.signing/signed/signing_declined` и `campaignCreators.currentStatus.signing/signed/signing_declined`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx`: в `groupedRows`-инициализаторе добавить три ключа; в `actionForStatus` — для трёх новых статусов вернуть `{}` (без mass-action mutation, без actionLabel); комментарий "Defensive: backend may ship a new status before the frontend bundle knows it" остаётся как safety-net на будущее.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx`: в `buildStatusColumns` добавить три case'а — `signing` отдаёт только `decided-at`, `signed` и `signing_declined` отдают `invited-pair` + `decided-at`. Exhaustive `_exhaustive: never` после regen потребует этого, иначе TS-ошибка.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx`: для трёх новых статусов компонент рендерит секцию **без** mass-action button (action.mutation === undefined → нет CTA), **без** remove-кнопки (per row). Логика уже есть (`onRemove` опционален, `actionLabel`/`mutation` опциональны) — просто проверяем что новые ветки корректно сваливаются в "render-only".
- Unit-тесты: дополнить `CampaignCreatorsSection.test.tsx` (рендер новой группы, отсутствие mass-action), `CampaignCreatorsTable.test.tsx` (новые ветки `buildStatusColumns`), `CampaignCreatorGroupSection.test.tsx` (баннер validation с `currentStatus = 'signed' / 'signing'` показывает правильный label из i18n).
- Playwright e2e: новый сценарий в `frontend/e2e/web/` (или дополнение существующего `campaign-creators.spec.ts` — уточняется при scout'е). Проводит креатора через реальный backend по флоу: `agree` (TMA) → `runOutboxOnce` (testapi из chunk 16) → polling `cc.status === 'signing'` → проверяем секцию `signing` на `/campaigns/:id` → `POST /trustme/webhook` со status=3 → polling `cc.status === 'signed'` → проверяем секцию `signed`. Аналогично — для `signing_declined` (status=9). По максимуму через бизнес-ручки.

**Out of scope (не входит):**

- TMA — статусы `signing/signed/signing_declined` не показываются в TMA (там креатор видит ТЗ + кнопки agree/decline; после `agreed` страница больше не открывается с возможностью действий).
- Landing — не затронута (нет страниц про кампанию).
- Скачивание signed PDF (`signed_pdf_content` через `DownloadContractFile`) — отложено на отдельный mini-PR (per spec-17 § Out of scope).
- Mass-action на signing_declined (повторное приглашение) — требует расширения backend state-machine `signing_declined → invited`, не закладывается.
- Расширение API CampaignCreator новыми timestamps (`contractSignedAt`, `contractSigningDeclinedAt`) — не делаем; UI в новых группах показывает уже существующий `decidedAt` (момент TMA-решения, остаётся актуальным).

## Принятые решения

1. **API-поля — минимум.** Расширяем только `CampaignCreatorStatus` enum в `openapi.yaml`. Никаких новых timestamp-полей в `CampaignCreator`. UI в новых группах показывает группу + статус + уже существующий `decidedAt`. Самый узкий слайс (см. развилку #1).

2. **Колонки per status — разные по группам:**
   - `signing`: только `decided-at` (минимум, момент TMA-согласия).
   - `signed`: `invited-pair` (count·time) + `decided-at`.
   - `signing_declined`: `invited-pair` + `decided-at`.
   Реализация — расширение switch в `buildStatusColumns` с тремя новыми case-ами.

3. **Порядок групп — по pipeline:**
   ```
   planned → invited → declined → agreed → signing → signed → signing_declined
   ```
   Хронология этапов; `signing_declined` рядом с `signed` (терминалы TrustMe-фазы) — симметрично с `declined`/`agreed` (терминалы TMA-фазы).

4. **Кнопки действий — нигде, как у agreed.** Все три новые группы — без mass-action и без индивидуального remove. Backend уже запрещает DELETE после agreed (`CAMPAIGN_CREATOR_REMOVE_AFTER_AGREED`); notify/remind на эти статусы тоже не вызываются. Чистый индикаторный view.

5. **Тексты i18n — с акцентом на «договор»** (различимы от TMA-`declined`):

   ```
   campaignCreators.groups:
     signing:           Подписывают договор
     signed:            Договор подписан
     signing_declined:  Отказались от договора

   campaignCreators.currentStatus:
     signing:           Подписывает договор
     signed:            Подписал(а) договор
     signing_declined:  Отказал(ась) от договора
   ```

6. **План PR — один.** OpenAPI enum + regenerate всех клиентов + frontend UI/i18n + unit + e2e — в одном PR. Атомарно: контракт и его потребители не разъезжаются.

7. **E2E — реальный backend через бизнес-ручки.** Креатор проводится через flow: TMA `agree` → testapi `run-outbox-once` (chunk 16) → polling `cc.status='signing'` (через `GET /campaigns/{id}/creators` с retry до 5s) → public webhook `POST /trustme/webhook` (chunk 17) → polling `cc.status='signed'`. Polling-helper'ы предотвращают флаки от worker'а (cron `@every 10s`, но `runOutboxOnce` синхронный по ручке) и от eventual consistency. Тесты используют существующий SpyOnly TrustMe-клиент через `testapi` ручки.

## Экраны / UI surface

- `/campaigns/:id` — `CampaignDetailPage` → `CampaignCreatorsSection`. После расширения секция отображает до 7 групп (если в кампании есть креаторы во всех статусах). Каждая группа — `CampaignCreatorGroupSection` с `CampaignCreatorsTable`.
- Drawer `AddCreatorsDrawer` — не затрагиваем (показывает доступных к добавлению, не текущих в кампании).
- `RemoveCreatorConfirm` — не затрагиваем (вызывается только из секций где `onRemove` определён).

## Тесты

**Unit (vitest + RTL):**
- `CampaignCreatorsSection.test.tsx`: рендер секций для всех 7 статусов (где есть строки); три новые секции — без CTA-кнопки, без remove-кнопки в строках.
- `CampaignCreatorsTable.test.tsx`: `buildStatusColumns` для signing → `[decided-at]`, signed/signing_declined → `[invited-pair, decided-at]`. Exhaustive switch coverage.
- `CampaignCreatorGroupSection.test.tsx`: `validationDetails[].currentStatus` ∈ {signing, signed, signing_declined} → баннер показывает правильную локализацию.

**E2E (Playwright):**
- Расширить `frontend/e2e/web/campaign-creators.spec.ts` (или соседний spec, уточняется при scout): новый `test()` "campaign creator moves through signing → signed". Setup через backend (testapi/seed admin + creator + campaign + contract template), agree через TMA, run-outbox-once, polling cc.status='signing', screenshot/assert на секцию signing, webhook со status=3, polling cc.status='signed', assert на секцию signed. Симметричный тест на signing_declined через webhook status=9.
- Polling-helper'ы (`waitForCcStatus(campaignId, creatorId, expectedStatus, timeoutMs=5000)`) в `frontend/e2e/helpers/api.ts`.
- `data-testid` на новых секциях/строках наследуется от существующих паттернов (`campaign-creator-{kind}-{id}` и т.п.) — без специальных testid'ов под статус (фильтрация в тестах по содержимому таблицы родительской секции).

## Риски и edge cases

- **Race с параллельным chunk 17.** До слияния chunk 17 в main стартовать код-фазу нельзя — backend не вернёт `signing/signed/signing_declined` без webhook handler'а. Spec фиксирует «pre-req chunk 17 в main»; работа агента в spec-mode ограничена intent + spec.
- **Existing rows backfill.** Миграция chunk 16 не делает backfill — старые `agreed`-ряды остаются `agreed`. Новые статусы появляются только для новых движений по state-machine. UI обратно совместим — старые group'ы продолжают работать.
- **Soft-deleted кампания** — `CampaignCreatorsSection` уже не рендерится при `campaign.isDeleted=true`. Изменений не нужно.
- **Defensive дроп строки** в `groupedRows` (`if (!bucket) continue`) — оставляем для safety на будущее, но после расширения он не должен срабатывать; в unit-тесте проверяем что все 7 валидных статусов корректно распределяются.
- **OpenAPI enum в strict-server.** После регенерации backend `internal/api/server.gen.go` потребует `signing/signed/signing_declined` как валидные значения в response-сериализации. Без этой правки strict-server в chunk 17 (когда вернёт `cc.status='signed'`) не сможет сериализовать. **Это критичная составляющая chunk 18 — она должна выйти раньше или вместе с реальной выкаткой webhook'а на staging.**

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md` (Группа 7, chunk 18)
- Parent intent: `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md`
- Pair (chunk 17): `_bmad-output/implementation-artifacts/spec-17-trustme-webhook.md`
- Прецедент (chunk 15 — расширение страницы статусами): архив или диф PR (история chunk 15)
- Стандарты: `docs/standards/`
