---
title: 'Backend: рассылка приглашений и ремайндеров через бот (chunk 12)'
type: 'feature'
created: '2026-05-08'
status: 'done'
baseline_commit: '2d6c3d2a07b2a7ad41994627f1f5457a120df152'
context:
  - _bmad-output/implementation-artifacts/intent-campaign-notifications-backend.md
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
  - docs/standards/
---

> Перед началом работы агент ОБЯЗАН полностью загрузить все файлы из `docs/standards/`
> (без сокращений). Источник правды по продуктовой модели — `intent-campaign-notifications-backend.md`
> и `design-campaign-creator-flow.md`. Этот спек — implementation slice, не дублирует их полностью.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** В chunk 10 уже есть таблица `campaign_creators` (статусы «Запланирован» / «Приглашён» / «Отказался» / «Согласился»), но нет ручек для рассылки приглашений и ремайндеров через Telegram-бот. Без них admin не может перевести креаторов из «Запланирован»/«Отказался» в «Приглашён» и не может ремайндить тех, кто завис без ответа. Также нужно защититься от смены `tma_url` после рассылок — старая ссылка в боте сломается.

**Approach:** Две admin-ручки `POST /campaigns/{id}/notify` и `POST /campaigns/{id}/remind-invitation` со strict-422 batch-validation и partial-success runtime delivery (per-creator мелкий tx). Расширение `PATCH /campaigns/{id}` с локом на смену `tma_url` при наличии `invited_count > 0`. Доставка через новый синхронный метод в `internal/telegram.Notifier` с inline web_app button (обязательно для chunk 14 TMA initData).

## Boundaries & Constraints

**Always:**
- handler→service→repository слои, transactions через `dbutil.WithTx` (стандарт).
- Audit per-creator в той же tx, что и UPDATE; success-логи ПОСЛЕ `WithTx`.
- Authz через `*AuthzService` (`CanNotifyCampaignCreators`, `CanRemindCampaignCreators` — admin only).
- API типы — codegen из `openapi.yaml`; mockery `all: true`; constants для имён колонок/таблиц.
- Stdout-логи: без PII (`chat_id` не логируем); допустимо `campaign_id`, `creator_id`, счётчики.
- Сообщение бота = текст + одна inline-кнопка типа `web_app` с `url = tma_url`. Plain text URL запрещён — TMA initData (chunk 14) приходит только через web_app.

**Ask First:**
- Любое изменение state-машины `campaign_creators` или появление 5-го статуса.
- Любое изменение существующих ручек A1/A2/A3 (chunk 10).
- Изменения уже прогнанной миграции `20260507044135_campaign_creators.sql` in-place.

**Never:**
- Не делаем TMA-ручки T1/T2 agree/decline (chunk 14).
- Не делаем «ремайндер по подписанию» (Группа 7, отдельный design).
- Не делаем фронт (chunk 13).
- Не плодим отдельный `CampaignNotificationService` — расширяем `*CampaignCreatorService`.
- Не модифицируем generic `APIError` для structured details — кастомные per-endpoint схемы.
- Не плодим миграции — chunk 10 покрывает все нужные поля (`invited_at/_count`, `reminded_at/_count`, `decided_at`).

## I/O & Edge-Case Matrix

| Сценарий | Вход / state | Ожидание | Ошибки |
|---|---|---|---|
| **A4 happy** | админ, кампания active, все creator_ids в «Запланирован» / «Отказался», бот доставляет всем | 200 `{undelivered:[]}`; status=invited; `invited_count++`, `invited_at=now()`; на re-invite из declined: `reminded_count=0, reminded_at=NULL, decided_at=NULL`; per-creator audit `campaign_creator_invite`; 1 spy-message с web_app.url=tma_url на каждого | — |
| **A5 happy** | админ, все creator_ids в «Приглашён» | 200 `{undelivered:[]}`; status не меняется; `reminded_count++`, `reminded_at=now()`; audit `campaign_creator_remind` | — |
| **Batch-validation** | один из creator_ids в `agreed` / отсутствует в campaign_creators | 422 кастомная схема: `{error:{code:"campaign_creator_batch_invalid", details:[{creator_id, reason:"wrong_status"|"not_in_campaign", current_status?}]}}`; БД, spy, audit без изменений | validate-pass собирает ВСЕ нарушения, не first-fail |
| **Partial-success** | спай-notifier фейлит часть creator_ids (Forbidden / network) | 200 `{undelivered:[{creator_id, reason:"bot_blocked"|"unknown"}]}`; для failed counters/audit без изменений; для delivered — как в happy | reason mapping: 403 / "bot was blocked" / "user is deactivated" → `bot_blocked`; прочее → `unknown` |
| **Soft-deleted кампания** | `is_deleted=true` | 404 `CAMPAIGN_NOT_FOUND` для A4/A5; spy без вызовов; БД без изменений | проверка через `assertCampaignActive` (как в chunk 10) |
| **PATCH tma_url с invited_count>0** | у любого creator кампании `invited_count>0` и в PATCH новый `tma_url` ≠ текущий | 422 `CAMPAIGN_TMA_URL_LOCKED` атомарно — даже если в том же запросе менялся `name`; БД и audit без изменений | `tma_url` совпадает с текущим (no-op) → лок не срабатывает |
| **Дубли в creatorIds** | один creator_id 2+ раз | 422 `CAMPAIGN_CREATOR_IDS_DUPLICATES` (handler-уровень, как в A1) | min 1 / max 200; пустой массив → `CAMPAIGN_CREATOR_IDS_REQUIRED`; > 200 → `CAMPAIGN_CREATOR_IDS_TOO_MANY` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- добавить 2 новых operation'а + кастомные response schemas (`CampaignCreatorBatchInvalidError`, `CampaignNotifyResponse`); расширить описание PATCH `/campaigns/{id}` (добавить 422 `CAMPAIGN_TMA_URL_LOCKED`).
- `backend/internal/domain/errors.go` + `domain/campaign.go` + `domain/campaign_creator.go` -- новые `CodeCampaignTmaURLLocked`, `CodeCampaignCreatorBatchInvalid`; `ErrCampaignTmaURLLocked` (`*ValidationError`); сериализуемая структура `CampaignCreatorBatchInvalidError` с `Details []BatchValidationDetail`.
- `backend/internal/repository/campaign_creator.go` -- добавить методы:
  - `ListByCampaignAndCreators(ctx, campaignID, creatorIDs []string) ([]*CampaignCreatorRow, error)` для validation pre-pass.
  - `ApplyInvite(ctx, id string, fromStatus string) (*CampaignCreatorRow, error)` — UPDATE с set status=invited, invited_count+=1, invited_at=now, reminded_count=0 ON declined-source, reminded_at=NULL, decided_at=NULL ON declined-source. Возвращает обновлённую строку для audit.
  - `ApplyRemind(ctx, id string) (*CampaignCreatorRow, error)` — UPDATE reminded_count+=1, reminded_at=now (статус не меняется).
  - `ExistsInvitedInCampaign(ctx, campaignID string) (bool, error)` для PATCH-lock.
- `backend/internal/repository/creator.go` -- добавить `GetTelegramUserIDsByIDs(ctx, []string) (map[string]int64, error)` для пакетного резолва telegram_user_id перед доставкой.
- `backend/internal/service/campaign_creator.go` -- добавить методы `Notify` и `RemindInvitation` (`returns (undelivered []NotifyFailure, err error)`); расширить `CampaignCreatorRepoFactory` интерфейс (`NewCreatorRepo`); инжектить `notifier *telegram.Notifier` (или интерфейс `CampaignInviteNotifier`) через конструктор.
- `backend/internal/service/campaign.go` -- расширить `UpdateCampaign`: после GetByID, если `in.TmaURL != oldRow.TmaURL`, проверить `ExistsInvitedInCampaign(ctx, id)` через новый repo-метод (через factory). При true → return `domain.ErrCampaignTmaURLLocked`. Расширить `CampaignRepoFactory` интерфейс `NewCampaignCreatorRepo`.
- `backend/internal/service/audit_constants.go` -- добавить `AuditActionCampaignCreatorInvite`, `AuditActionCampaignCreatorRemind`.
- `backend/internal/handler/campaign_creator.go` -- добавить handler'ы `NotifyCampaignCreators` и `RemindCampaignCreatorsInvitation` (валидация empty/too-many/duplicates как в `AddCampaignCreators`); вызов сервиса; формирование response с `undelivered`. Маппинг батч-валидационной ошибки сервиса в кастомную схему 422 — через ad-hoc respondCampaignBatchInvalid (или прямое возвращение `*api.NotifyCampaignCreators422JSONResponse{}`).
- `backend/internal/authz/campaign_creator.go` -- добавить `CanNotifyCampaignCreators`, `CanRemindCampaignCreators` (admin only).
- `backend/internal/telegram/notifier.go` -- добавить **синхронный** метод `SendCampaignInvite(ctx, chatID int64, text, tmaURL string) error`: один SendMessage с inline web_app keyboard, timeout 5s, без retry; ошибку возвращает наверх (мапится сервисом в reason). Текстовые шаблоны `inviteText` / `remindInvitationText` — package-level константы.
- `backend/internal/telegram/notifier_test.go` -- table-driven `mapTelegramErrorToReason` (новый helper в notifier.go) + тест `SendCampaignInvite` с spy-sender.
- `backend/cmd/api/main.go` -- если меняется конструктор `NewCampaignCreatorService` (инжект notifier) — обновить вызов.
- `backend/e2e/campaign_creator/` (или существующий тестовый файл) -- 8 сценариев из I/O Matrix; spy_store через `/test/*` или existing test endpoint для контролируемых fail'ов одного creator_id.
- `backend/internal/handler/testapi.go` -- если для partial-success e2e нужна возможность форсить fail для конкретного creator_id, добавить test endpoint вроде `POST /test/telegram/spy/fail-next`. Только в EnableTestEndpoints. Если spy_store уже умеет — переиспользовать.

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить notify/remind operations + кастомные response schemas; добавить 422 для PATCH с `CAMPAIGN_TMA_URL_LOCKED`. Запустить `make generate-api`.
- [x] `backend/internal/domain/errors.go` -- добавить `CodeCampaignTmaURLLocked`, `CodeCampaignCreatorBatchInvalid`.
- [x] `backend/internal/domain/campaign.go` -- добавить `ErrCampaignTmaURLLocked` (`*ValidationError`).
- [x] `backend/internal/domain/campaign_creator.go` -- добавить тип `CampaignCreatorBatchInvalidError struct { Details []BatchValidationDetail }` (реализует `error` через `Error()` + сериализуется handler'ом в response 422).
- [x] `backend/internal/repository/campaign_creator.go` -- 4 новых метода + расширение интерфейса `CampaignCreatorRepo`.
- [x] `backend/internal/repository/creator.go` -- `GetTelegramUserIDsByIDs`.
- [x] `backend/internal/service/audit_constants.go` -- 2 новые константы.
- [x] `backend/internal/service/campaign_creator.go` -- `Notify` + `RemindInvitation` + расширение конструктора (notifier dep + `NewCreatorRepo` в factory).
- [x] `backend/internal/service/campaign.go` -- расширение `UpdateCampaign` (lock check) + расширение `CampaignRepoFactory`.
- [x] `backend/internal/handler/campaign_creator.go` -- handler'ы `NotifyCampaignCreators` + `RemindCampaignCreatorsInvitation` + маппинг 422 batch-invalid в кастомную схему.
- [x] `backend/internal/authz/campaign_creator.go` -- 2 новых метода authz.
- [x] `backend/internal/telegram/notifier.go` -- `SendCampaignInvite` + `mapTelegramErrorToReason` + 2 текста.
- [x] `backend/cmd/api/main.go` -- обновлён вызов `NewCampaignCreatorService(... tgRig.Notifier ...)`.
- [x] `make generate-mocks` -- моки регенерены.
- [x] Unit-тесты по слоям (см. секцию Verification): handler / service / repository / authz / notifier.
- [x] `backend/e2e/...` -- 8 сценариев из I/O Matrix (campaign_notify_test.go).

**Acceptance Criteria:**
- Given кампания active с N creator'ами в «Запланирован», when admin вызывает `POST /campaigns/{id}/notify` со списком всех N, then 200 с `undelivered:[]`, статусы=Приглашён, `invited_count=1`, audit-row `campaign_creator_invite` per creator, spy_store содержит N сообщений, каждое — текст + InlineKeyboardMarkup.WebApp.URL=tma_url.
- Given креатор в кампании со status `agreed`, when admin вызывает A4 со списком, включающим этого креатора, then 422 с `details: [{creator_id, reason:"wrong_status", current_status:"agreed"}]`, БД и spy без изменений.
- Given у одного из creator'ов в батче бот вернёт Forbidden (через test-spy), when admin вызывает A4, then 200 с `undelivered:[{creator_id, reason:"bot_blocked"}]`, у failed creator counters/audit не изменены, у delivered — изменены.
- Given у любого creator кампании `invited_count > 0`, when admin делает PATCH `/campaigns/{id}` с новым `tma_url`, then 422 `CAMPAIGN_TMA_URL_LOCKED`, в БД tma_url не изменился, audit `campaign_update` не записан.
- Given non-admin user, when вызывает A4 или A5, then 403.
- Given кампания `is_deleted=true`, when admin вызывает A4 или A5, then 404.
- Все mutate-handler'ы покрыты `testutil.AssertAuditEntry` для соответствующих audit actions.
- `make lint-backend test-unit-backend test-unit-backend-coverage test-e2e-backend` проходит. Coverage gate ≥80% per-method для новых функций в handler/service/repository/middleware/authz.

## Spec Change Log

- **2026-05-08, реализация:** в Code Map добавлен test-only endpoint
  `POST /test/telegram/spy/fake-chat` (+ `/fail-next`) и `RegisterFakeChat`
  / `RegisterFailNext` в `internal/telegram/spy_store.go` — синтетические
  test chat_id'ы не достижимы для реального бота, без stub'а TeeSender
  возвращал бы "Bad Request: chat not found" и happy-path A4 никогда бы
  не имел `undelivered=[]`. Production-поверхность не задета — fake-chat
  registration работает только при `EnableTestEndpoints=true`.

## Design Notes

**Notifier — синхронный метод vs существующий fire-and-forget.** Существующие `Notify*` методы — async с retry, ошибка eaten в goroutine. Для partial-success нужен sync send с возвратом ошибки. Решение: новый метод `SendCampaignInvite` рядом с `fire`, не трогает существующее.

```go
// SendCampaignInvite sends one invite/remind message synchronously and returns
// the underlying SendMessage error so the service can map it to undelivered.
func (n *Notifier) SendCampaignInvite(ctx context.Context, chatID int64, text, tmaURL string) error {
    callCtx, cancel := context.WithTimeout(ctx, n.timeout)
    defer cancel()
    _, err := n.sender.SendMessage(callCtx, &bot.SendMessageParams{
        ChatID: chatID, Text: text,
        ReplyMarkup: &models.InlineKeyboardMarkup{
            InlineKeyboard: [][]models.InlineKeyboardButton{{{
                Text: "Посмотреть",
                WebApp: &models.WebAppInfo{URL: tmaURL},
            }}},
        },
    })
    return err
}

func mapTelegramErrorToReason(err error) string {
    if err == nil { return "" }
    s := err.Error()
    // Telegram returns "Forbidden: bot was blocked by the user" / "user is deactivated"
    if strings.Contains(s, "Forbidden") || strings.Contains(s, "blocked by the user") || strings.Contains(s, "user is deactivated") {
        return "bot_blocked"
    }
    return "unknown"
}
```

**Service flow (A4 / A5):**
1. `assertCampaignActive(ctx, campaignID)` (chunk 10 helper).
2. Read-only через pool: `ListByCampaignAndCreators` + `GetTelegramUserIDsByIDs`. Validate-pass собирает ВСЕ нарушения в `[]BatchValidationDetail`. Если есть — `return CampaignCreatorBatchInvalidError{Details: ...}`.
3. Loop per creator: `notifier.SendCampaignInvite` → если err: `undelivered = append(..., {creator_id, reason: mapTelegramErrorToReason(err)})`; иначе `dbutil.WithTx { ApplyInvite/ApplyRemind + writeAudit(oldCC, newCC) }`.
4. Return `undelivered`.

**Re-invite reset.** В `ApplyInvite` SQL: при переходе `declined → invited` — сбрасываем `reminded_count=0, reminded_at=NULL, decided_at=NULL`. Остальные счётчики сохраняются. Реализуется одним UPDATE через `CASE WHEN status=$declined THEN 0 ELSE reminded_count END` и т.п. — без двух pass'ов.

**Batch-invalid маппинг в response.** `domain.CampaignCreatorBatchInvalidError` — отдельный тип ошибки (не `*ValidationError`). Handler в strict-server обрабатывает её отдельной веткой: формирует `api.NotifyCampaignCreators422JSONResponse` (или общий тип `CampaignCreatorBatchInvalid422JSONResponse`) с `details`. Все остальные `*ValidationError` идут через стандартный `respondError` как раньше.

## Verification

**Commands:**
- `make generate-api` -- регенерация типов после правки openapi.yaml.
- `make lint-backend` -- gofmt + golangci-lint passes.
- `make test-unit-backend test-unit-backend-coverage` -- unit-тесты + per-method ≥80% gate проходит.
- `make test-e2e-backend` -- 8 e2e сценариев из I/O Matrix passes.

**Unit-тесты (mockery, t.Parallel(), race detector, ≥80% per-method):**
- `*CampaignCreatorService.Notify` — happy / batch-validation с ВСЕМИ details / partial-success / re-invite-reset / soft-delete / chat_id мап через `GetTelegramUserIDsByIDs`.
- `*CampaignCreatorService.RemindInvitation` — symmetric.
- `*CampaignService.UpdateCampaign` — PATCH-lock все ветки (tma_url меняется + invited>0 → lock; tma_url меняется + invited=0 → ok; tma_url не меняется → lock не срабатывает).
- `campaignCreatorRepository.ExistsInvitedInCampaign` / `ApplyInvite` / `ApplyRemind` / `ListByCampaignAndCreators` через pgxmock — точный SQL с литералами колонок.
- `creatorRepository.GetTelegramUserIDsByIDs` — pgxmock, маппинг id→tg_user_id.
- `Notifier.SendCampaignInvite` — spy-sender, проверка params (ChatID, Text, ReplyMarkup.InlineKeyboard[0][0].WebApp.URL).
- `mapTelegramErrorToReason` — table-driven (Forbidden/blocked/deactivated → bot_blocked; nil → ""; прочее → unknown).
- `AuthzService.CanNotifyCampaignCreators` / `CanRemindCampaignCreators` — admin allow / brand_manager forbid.
- Handler `NotifyCampaignCreators` / `RemindCampaignCreatorsInvitation` — empty / too-many / duplicates ветки + happy через mock service + 422 batch-invalid маппинг в кастомную схему.

**E2E (Playwright не нужен — это бэк):**
8 сценариев из I/O Matrix, каждый mutate-кейс с `testutil.AssertAuditEntry` для соответствующих audit actions (или их отсутствия для не-доставленных).

**Manual checks (self-check агента, обязательный — между unit и e2e):**
- curl A4 на свежесозданную кампанию → 200, undelivered list пуст / содержит ожидаемые reason.
- SELECT из `campaign_creators`, `audit_logs` — счётчики и audit записи сходятся со spec'ом.
- spy_store запись (через test-endpoint или дамп) — N сообщений, web_app.url правильный.
- Расхождение → агент сам фиксит код, перезапускает self-check, переходит к e2e в той же сессии. HALT только при продуктовой развилке.

## Suggested Review Order

**Service оркестрация (entry point)**

- Главный поток A4/A5 — validate-pass на pool, доставка и per-creator tx через `dbutil.WithTx`.
  [`campaign_creator.go:254`](../../backend/internal/service/campaign_creator.go#L254)

- Различия между notify/remind инкапсулированы в одну map-таблицу — нет switch'ей по op в коде.
  [`campaign_creator.go:201`](../../backend/internal/service/campaign_creator.go#L201)

- Sent-but-not-persisted: сообщение ушло, но tx упал → undelivered=`unknown`, batch продолжается (review patch).
  [`campaign_creator.go:329`](../../backend/internal/service/campaign_creator.go#L329)

- tma_url lock в UpdateCampaign — отказ при invited_count>0; no-op (тот же URL) пропускает проверку.
  [`campaign.go:99`](../../backend/internal/service/campaign.go#L99)

**Repository слой**

- `ApplyInvite` UPDATE одним запросом: status=invited, invited_count++, и CASE-сброс reminded/decided для declined-source.
  [`campaign_creator.go:198`](../../backend/internal/repository/campaign_creator.go#L198)

- `ExistsInvitedInCampaign` — `SELECT EXISTS(SELECT 1 ... WHERE invited_count>0)` через squirrel column-expression.
  [`campaign_creator.go:230`](../../backend/internal/repository/campaign_creator.go#L230)

- Validation pre-pass: read-only `ListByCampaignAndCreators` с IN-clause на batch creator_ids.
  [`campaign_creator.go:165`](../../backend/internal/repository/campaign_creator.go#L165)

- Резолв chat_ids одним запросом для всего батча.
  [`creator.go:191`](../../backend/internal/repository/creator.go#L191)

**API контракт + кастомное 422**

- Notify/remind operations + oneOf 422 (batch-invalid + стандартный validation).
  [`openapi.yaml:1296`](../../backend/api/openapi.yaml#L1296)

- 422 PATCH с `CAMPAIGN_TMA_URL_LOCKED` — описание отдельно от шаблонного.
  [`openapi.yaml:1213`](../../backend/api/openapi.yaml#L1213)

- writeBatchInvalid обходит закрытое поле union в strict-server и пишет ответ напрямую.
  [`response.go:115`](../../backend/internal/handler/response.go#L115)

**Handler + authz**

- Handler делегирует authz → validate batch shape → service; маппит result в API.
  [`campaign_creator.go:92`](../../backend/internal/handler/campaign_creator.go#L92)

- Общий `validateCampaignCreatorBatch` теперь покрывает Add / Notify / Remind — empty / too-many / duplicates.
  [`campaign_creator.go:135`](../../backend/internal/handler/campaign_creator.go#L135)

- Authz — admin-only для обоих новых эндпоинтов.
  [`campaign_creator.go:42`](../../backend/internal/authz/campaign_creator.go#L42)

**Telegram уровень**

- Синхронный `SendCampaignInvite` с inline web_app кнопкой — обязательно для chunk-14 initData.
  [`notifier.go:342`](../../backend/internal/telegram/notifier.go#L342)

- `MapTelegramErrorToReason` — sentinel-проверка + substring-fallback на канонические Telegram-сообщения.
  [`notifier.go:372`](../../backend/internal/telegram/notifier.go#L372)

- spy_store: per-chat one-shot fail-next + permanent fake-chat для test-only обхода реального бота.
  [`spy_store.go:58`](../../backend/internal/telegram/spy_store.go#L58)

- TeeSender короткозамыкает реальный бот при fail-next/fake-chat.
  [`tee_sender.go:60`](../../backend/internal/telegram/tee_sender.go#L60)

**Domain типы**

- Новые коды + `ErrCampaignTmaURLLocked` (`*ValidationError`) — стандартный путь через `respondError`.
  [`errors.go:74`](../../backend/internal/domain/errors.go#L74)

- `CampaignCreatorBatchInvalidError` — кастомный тип ошибки для оборачивания validation details.
  [`campaign_creator.go:79`](../../backend/internal/domain/campaign_creator.go#L79)

- 2 новых audit-action константы.
  [`audit_constants.go:23`](../../backend/internal/service/audit_constants.go#L23)

**Test infrastructure (test-only поверхность)**

- /test/telegram/spy/fake-chat + /fail-next регистрируют поведение spy_store на конкретный chat_id.
  [`testapi.go:283`](../../backend/internal/handler/testapi.go#L283)

- `SetupApprovedCreator` автоматически регистрирует fake-chat — chunk-12 e2e работает с любым TELEGRAM_MOCK режимом.
  [`creator.go:83`](../../backend/e2e/testutil/creator.go#L83)

**Тесты**

- E2E все 8 сценариев из I/O Matrix через сгенерированный клиент.
  [`campaign_notify_test.go:146`](../../backend/e2e/campaign_creator/campaign_notify_test.go#L146)

- Service unit-тесты Notify/Remind — happy, batch-validate, partial-success, re-invite reset, sent-but-not-persisted.
  [`campaign_creator_test.go:691`](../../backend/internal/service/campaign_creator_test.go#L691)

- Repository unit-тесты для всех 5 новых SQL-запросов через pgxmock с буквальными SQL-строками.
  [`campaign_creator_test.go:212`](../../backend/internal/repository/campaign_creator_test.go#L212)
