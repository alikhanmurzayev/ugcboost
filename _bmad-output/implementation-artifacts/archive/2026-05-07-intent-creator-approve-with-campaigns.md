---
title: "Intent: approve креатора с одновременным добавлением в выбранные кампании"
type: intent
status: living
created: "2026-05-07"
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
---

# Intent: approve creator application с опциональным добавлением в кампании

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `backend-architecture.md`, `backend-codegen.md`, `backend-errors.md`, `backend-transactions.md`, `backend-testing-unit.md`, `backend-testing-e2e.md`, `frontend-api.md`, `frontend-components.md`, `frontend-state.md`, `frontend-types.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

## Тезис

На экране модерации заявки креатора админ опционально выбирает одну или несколько кампаний. При approve креатор создаётся (как сейчас, в одной транзакции) и получает Telegram-уведомление (как сейчас, fire-and-forget после tx). После этого последовательно — отдельной транзакцией на каждую выбранную кампанию — креатор добавляется через существующий `campaignCreatorService.Add`. Расширяем существующую ручку `POST /creators/applications/{id}/approve` опциональным `campaignIds[]` в request body. Транзакционная сшивка всех операций в одну атомарную НЕ делается (стартап-MVP, цена рефакторинга высокая).

## Зафиксированные решения

| # | Решение | Reasoning |
|---|---|---|
| 1 | **Расширяем существующую ручку**, новый эндпоинт не создаём | Меньше API-поверхности, меньше дублирования логики. Поле `campaignIds[]` в body — опциональное; `null` / отсутствие / пустой массив = старое поведение (просто approve без add) |
| 2 | **Последовательные транзакции, не одна атомарная** | Дёшево реализовать, текущие сервисы не ломаем. Цена несогласованности (креатор создан, в часть кампаний не попал) низкая — админ это увидит и доделает вручную |
| 3 | **First-fail-stop**: при первом fail на add — стоп, ошибка наверх, оставшиеся кампании не пробуем | Простая семантика, простой UX. Админу проще разобраться с одной ошибкой, чем со списком частичных успехов/провалов |
| 4 | **Response не дорабатываем**: стандартная ошибка с actionable message ("Не удалось добавить креатора в кампанию X. Креатор создан, добавьте вручную через страницу кампании"). Уже добавленные в предыдущие кампании остаются (commit'ed) | Не вводим новую структурированную response-shape вида `{addedCampaignIds, failedCampaigns}`. Переиспользуем существующий error envelope |
| 5 | **Telegram-notify — между approve и add**: tx1 (approve+create+audit) → notify (fire-and-forget, как сейчас) → цикл по campaignIds (каждая своя tx) | Notify — часть успешного approve. Если add провалится — креатор уже валидный, telegram-сообщение уже ушло, состояние "креатор создан и онбоарден, не во всех кампаниях" — приемлемо |
| 6 | **UI: простой мультиселект кампаний по названию** | Не адаптируем `AddCreatorsDrawer` (он для обратного направления — креаторы→кампания, и сейчас параллельный агент дорабатывает его UX-баги). Используем стандартный multi-select / autocomplete по name. Деталей кампании в селекте не показываем |
| 7 | **Audit ничего нового**: `creator_application_approve` пишется в tx1 (как сейчас), `campaign_creator_add` пишется внутри `Add` per-creator (как сейчас) | Каждая операция уже содержит свой audit; новых event types не вводим. Семантика "approve без кампаний" vs "approve с кампаниями" восстанавливается из последовательности audit-rows по времени |
| 8 | **Validation в handler**: `campaignIds` опционален; пустой массив / отсутствие / `null` = "не добавлять никуда" (валидно, не 422). При непустом массиве — 422 на: длина > **20**, дубликаты UUID. Формат UUID — через сгенерированный тип контракта | Cap 20 — реалистичный потолок (админ выберет 1-5 на практике). Дубликаты — фронт сам не должен слать, single source of truth = strict-422. Симметрично с `addCampaignCreators`, но с опциональностью и меньшим cap |
| 9 | **Pre-validation существования кампаний — В HANDLER, ДО вызова service**: handler, получив непустой `campaignIds[]`, делает batch-check существования + не-soft-deleted всех ID через зависимость `campaignService.AssertActiveCampaigns(ctx, ids[])`. Если хоть одна не существует / удалена → **422, service approve вообще не вызывается, заявка не переходит в approved, креатор не создаётся** | Принципиально для семантики "корректные данные = approve проходит, мусорные данные = всё откатывается". Защищает от кейса "approve прошёл и креатор создан, а первый же add упал из-за давно удалённой кампании в селекте". Перенос в handler по решению tech lead'а — единая точка валидации входа, до сервиса доходят только провалидированные данные. Race между pre-check и циклом add — крайне редкая, обрабатывается first-fail-stop как defense in depth |
| 10 | **Service approve владеет циклом add'ов**: `creatorApplicationService.ApproveApplication(ctx, applicationID, actorUserID, campaignIDs []string)` принимает уже-валидированный slice. Внутри: tx1 (как сейчас) → notify (как сейчас) → цикл `for _, id := range campaignIDs { campaignCreatorService.Add(ctx, id, []string{creatorID}) }` с first-fail-stop. Новая зависимость в service: `campaignCreatorService.Add` через локальный интерфейс по go-convention | Service не валидирует существование (handler уже сделал), только оркестрирует sequencing. Цикл add inline — нет необходимости в отдельном `AddCreatorToMultipleCampaigns` методе на campaign-creator-service. Race "кампания удалена между handler-проверкой и циклом add" обработается через первую же error от `Add` |
| 11 | **Granular error code для pre-validation**: единый новый код `CAMPAIGN_NOT_AVAILABLE_FOR_ADD` (или согласовать имя при имплементации), 422. Не различаем "не существует" vs "soft-deleted" — для админа поведение идентичное (рефреш списка кампаний). Message: "Одна или несколько выбранных кампаний недоступны. Обновите список и попробуйте снова" | Один код = один UI-handler на фронте. Два разных кода (NOT_FOUND / DELETED) — over-engineering без UX-выгоды |
| 12 | **E2E тесты — отдельный новый файл**, существующие e2e на approve не трогаем | Новый flow approve+with+campaigns живёт своими e2e-сценариями (success-with-campaigns, validation errors, pre-validation 422 на несуществующую/удалённую кампанию, partial-failure mid-cycle). Старые e2e на approve без campaignIds остаются как baseline |
| 13 | **Frontend подгружает кампании через существующий `GET /campaigns`** с фильтром "только неудалённые" | Если эндпоинт уже по дефолту возвращает только `is_deleted=false` — используем как есть. Если показывает в т.ч. soft-deleted — расширяем фильтром (`?includeDeleted=false` или дефолт-флипом) в рамках того же PR. Реиспользуем pageSize до 100 одностраничной подгрузкой; при росте >100 кампаний — добавляем infinite scroll/server-search отдельным мини-PR (за scope этого intent'а). Это решение проверяет bmad-quick-dev на стадии scout |
| 14 | **UX после partial-failure**: dialog модерации не закрывается. Показываем inline-ошибку от бэка под submit-кнопкой ("Креатор создан, но не удалось добавить в кампанию X. Добавьте вручную через страницу кампании"). Кнопка "ОК" / "Закрыть" — админ сам закрывает после прочтения. Invalidate queries: applications-moderation list, creators list, campaign-creators для всех выбранных кампаний (рефетч уберёт уже-добавленных) | Critical: ошибка должна донести "креатор создан" — иначе админ повторно нажмёт approve и получит `ApplicationNotApprovable` 422 (заявка уже не в moderation). Текст message формирует backend в `error.Message`; фронт показывает как есть |
| 15 | **UX после success**: dialog закрывается как сейчас, без дополнительных подтверждений. Те же invalidate-queries что и при failure (для консистентности) | Минимум модальной нагрузки. Никаких "добавлен в N кампаний" — пользователь сам видит результат на странице кампаний |

## Sequence diagram (целевой flow)

```
Admin clicks "Approve" with campaignIds=[A, B, C]
    │
    ▼
Handler ApproveCreatorApplication
    │
    ├── parse body, handler-validation (count ≤20, dedupe, UUID format) → 422 если нет
    │
    ├── PRE-VALIDATION (если campaignIDs непуст):
    │     ├── campaignService.AssertActiveCampaigns(ctx, campaignIDs)
    │     │     └── batch-check существования + is_deleted=false
    │     └── если хоть одна не найдена / удалена → 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD,
    │         service approve НЕ вызывается, креатор НЕ создаётся
    │
    ▼
Service ApproveApplication(applicationID, actorUserID, campaignIDs []string)
    │
    ├── tx1: dbutil.WithTx
    │     ├── transition application → approved
    │     ├── INSERT creator + socials + categories
    │     ├── INSERT audit (creator_application_approve)
    │     └── COMMIT
    │
    ├── notifyApplicationApproved(creator) — fire-and-forget (как сейчас)
    │
    ├── for each campaignID in campaignIDs:
    │     │
    │     ├── campaignCreatorService.Add(campaignID, [creatorID])
    │     │     ├── assertCampaignActive
    │     │     ├── tx_i: INSERT campaign_creator + audit (campaign_creator_add)
    │     │     └── COMMIT
    │     │
    │     └── on first fail: STOP, return error с actionable message
    │
    └── return creatorID (success) или error (partial failure: креатор создан, в часть кампаний попал, в первой fail-нувшей — нет)
```

## Что точно НЕ делаем

- Атомарная сшивка approve+add в одну tx (требует рефактора `campaignCreatorService.Add`).
- Возврат структурированного `{addedCampaignIds, failedCampaigns}` в response.
- Параллельная попытка add во все кампании (для "best-effort" partial-success).
- Новые audit event types для approve-with-campaigns.
- Новый эндпоинт `POST /creators/applications/{id}/approve-with-campaigns`.
- Изменения в существующих e2e на approve без `campaignIds`.

## Кросс-cutting напоминания для bmad-quick-dev

- **OpenAPI**: расширить `approveCreatorApplication` request body с опциональным `campaignIds[]` (UUID, maxItems=20). Добавить response 422 коды `CAMPAIGN_IDS_TOO_MANY` / `CAMPAIGN_IDS_DUPLICATES` / `CAMPAIGN_NOT_AVAILABLE_FOR_ADD`. После правки — `make generate-api`.
- **Зависимости**: handler `ApproveCreatorApplication` получает новую зависимость на `campaignService.AssertActiveCampaigns(ctx, ids)`. Service `creatorApplicationService` получает новую зависимость на `campaignCreatorService.Add` через локальный интерфейс.
- **Audit**: ничего нового. tx1 пишет `creator_application_approve` (как сейчас), `Add` в каждой tx_i пишет `campaign_creator_add` (как сейчас). Logs успеха — после tx, не внутри (per `backend-transactions.md`).
- **Notifications**: `notifyApplicationApproved` — fire-and-forget между tx1 и циклом add (как сейчас), порядок не меняется.
- **Migration**: не нужна. Поле `campaignIds` живёт только в request body, не в схеме.
- **Frontend i18n**: новые ключи в `creatorApplications.json` (метка multi-select'а, hint "Опционально", error fallback). Сам error.Message приходит с бэка локализованным.
- **UI multi-select**: bmad-quick-dev на этапе scout выбирает компонент (или новый shared). Требования: search по name, max 20 selected, опциональность (можно отправить без выбора), disabled при isPending или пока список кампаний не загружен (submit-lock с async prereqs per `frontend-state.md`).

## Тестирование (high-level)

- **Backend unit handler approve**: моки на `creatorApplicationService` + `campaignService`. Сценарии: empty/null campaignIds (pass-through старого поведения), happy с N кампаний (mock возвращает active=true → service вызван с campaignIDs), pre-validation fail (mock active=false для одной → 422 без вызова service), validation fail (>20, duplicates → 422 без вызова service и без call AssertActiveCampaigns).
- **Backend unit service approve**: моки на `repoFactory` + `campaignCreatorService`. Сценарии: empty campaignIDs (только tx1 + notify, цикл не входит), happy с N (tx1 → notify → N вызовов Add → success), first-fail-stop (Add fail на 2-м из 3 → возвращена ошибка, 3-й не вызывался; через captured-input проверить что креатор создан в tx1).
- **Backend e2e (новый файл)**: success-with-campaigns (assert audit-rows для approve + per-campaign-add), validation 422 (>20 / дубли), pre-validation 422 (несуществующая / soft-deleted кампания → application остаётся в moderation, creator не создан, audit-row approve отсутствует), partial-failure mid-cycle (3 кампании, 2-я soft-deleted между call'ами — race-симуляция через test-helper или прямой UPDATE → assert: 1-я добавлена, 2-я fail, 3-я не пробовалась).
- **Frontend unit**: `ApproveApplicationDialog` — моки `useApprove` mutation с новым полем `campaignIds`. Сценарии: рендер multi-select, выбор кампаний, submit с/без выбора, error inline после partial-failure, invalidate-queries вызваны.
- **Frontend e2e (расширяем существующий или новый file)**: модерация → выбор 2 кампаний → approve → assert toast/inline success + переход креатора в обе кампании на странице каждой.
- **Self-check агента** между unit и e2e обязателен (curl на approve с campaignIDs, чтение БД на campaign_creators rows, чтение audit_logs).

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Параллельная работа (chunk 11 frontend mutations): `_bmad-output/implementation-artifacts/spec-campaign-creators-frontend-mutations.md` — НЕ пересекается по файлам (другая feature).
- Текущий approve-flow: `backend/internal/handler/creator_application.go:340`, `backend/internal/service/creator_application.go:1159`.
- Текущий add-creator-в-кампанию flow: `backend/internal/handler/campaign_creator.go:19`, `backend/internal/service/campaign_creator.go:48`.
- Стандарты: `docs/standards/`.
