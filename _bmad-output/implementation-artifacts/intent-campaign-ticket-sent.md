# Intent: чекбокс «Билет отправлен» в админской таблице креаторов кампании

> **Стандарты обязательны.** Перед реализацией исполнитель полностью загружает
> `docs/standards/` и сверяет каждое решение с релевантным стандартом
> (`backend-architecture.md`, `backend-repository.md`, `frontend-state.md`,
> `frontend-components.md`, `backend-codegen.md`, `security.md` и т.д.).
> Артефакт фиксирует итоговое состояние, не историю обсуждения.

## Контекст / проблема

На странице кампании `/campaigns/{campaignId}` (admin-only) есть таблица
креаторов, сгруппированных по статусу participation. После подписания
договора (`campaign_creators.status = 'signed'`) операционный шаг —
отправить креатору авиа/жд-билет, чтобы он прилетел на съёмку. Сейчас
отметки «билет отправлен» нигде нет: оператор не видит, кому ещё нужно
отправить билет, а кому уже отправили; список ведётся в голове / в
сторонней табличке.

## Решение

В группе `signed` таблицы креаторов кампании
(`CampaignCreatorsTable.tsx`) добавляется колонка «Билет отправлен» с
чекбоксом. Клик флипает поле в обе стороны через
`PATCH /campaigns/{id}/creators/{creatorId}`. Optimistic update — UI
переключается мгновенно; ошибка → откат + toast.

Допустимы оба направления (поставить и снять) — кейс «ошибочно отметил».
Для остальных статус-групп колонка/чекбокс не показывается: до подписания
билет не отправляют.

## API surface

### Эндпоинт

`PATCH /campaigns/{campaignId}/creators/{creatorId}` — универсальный
patch participation. Сейчас в body одно поле; в будущем сюда же можно
добавить другие toggleable-флаги без новых ручек.

### Request body

```yaml
CampaignCreatorPatchInput:
  type: object
  properties:
    ticketSent:
      type: boolean
  # минимум одно поле; пустой body → 422
```

Семантика:

- `ticketSent: true` → `ticket_sent_at = NOW()`.
- `ticketSent: false` → `ticket_sent_at = NULL`.

### Response

Обновлённый `CampaignCreator` целиком — фронт кладёт свежий снапшот в
react-query кэш по `campaignCreatorKeys.list(campaignId)`.

В schema `CampaignCreator` добавляется поле:

```yaml
ticketSentAt:
  type: string
  format: date-time
  nullable: true
```

### Авторизация и валидации

- Только `admin` (как и весь экран кампании).
- Если `status != 'signed'` → 422, `CodeValidation`, message
  «Билет можно отметить только подписавшему договор креатору».
- Пустой patch body (нет ни одного поля) → 422, `CodeValidation`.

## Domain / Repository

### Миграция

Новая forward-миграция (`make migrate-create NAME=campaign_creators_ticket_sent_at`):

```sql
-- +goose Up
ALTER TABLE campaign_creators
    ADD COLUMN ticket_sent_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE campaign_creators
    DROP COLUMN ticket_sent_at;
```

Default не ставится — `NULL` означает «не отправлено». CHECK не нужен.

### Row + константы

В `backend/internal/repository/campaign_creator.go`:

- В `CampaignCreatorRow` добавить поле `TicketSentAt *time.Time` с тегом
  `db:"ticket_sent_at"`. Без `insert`-тега: при создании participation
  поле всегда `NULL`.
- Константа `CampaignCreatorColumnTicketSentAt = "ticket_sent_at"`.

### Метод репо

`UpdateTicketSentAt(ctx, id string, sentAt *time.Time) (*CampaignCreatorRow, error)` —
ложится в существующий паттерн `ApplyInvite` / `ApplyRemind` /
`ApplyDecision` (operate по `id`, RETURNING строка):

```sql
UPDATE campaign_creators
SET ticket_sent_at = $1, updated_at = NOW()
WHERE id = $2
RETURNING <selectColumns>
```

`sql.ErrNoRows` (0 rows affected) → сервис интерпретирует как 404.

Чтение существующее: `GetByCampaignAndCreator(ctx, campaignID, creatorID)`
(`campaign_creator.go:148`) — переиспользуется без изменений.

## Service + audit

Метод
`CampaignCreatorService.PatchParticipation(ctx, campaignID, creatorID string, patch api.CampaignCreatorPatchInput) (*domain.CampaignCreator, error)`:

1. `repo.GetByCampaignAndCreator(ctx, campaignID, creatorID)` →
   `sql.ErrNoRows` → `domain.NewBusinessError(CodeNotFound, ...)` (404).
2. Если `row.Status != CampaignCreatorStatusSigned` →
   `domain.NewValidationError(CodeValidation, "Билет можно отметить только подписавшему договор креатору")` (422).
3. **No-op early return.** Текущее значение
   `(row.TicketSentAt != nil) == *patch.TicketSent` → возвращаем
   текущий row без UPDATE и без audit (защита от шумного аудита и
   лишних NOW()-перезаписей при повторных кликах).
4. `sentAt := nil`; если `*patch.TicketSent == true` →
   `sentAt = pointer.To(time.Now().UTC())`.
5. `dbutil.WithTx`: внутри одной транзакции —
   `UpdateTicketSentAt(tx, row.ID, sentAt)` + `auditRepo.Create(...)`.
6. После `WithTx` — `logger.Info` без PII (campaignID, creatorID UUID,
   action). Маппинг row → `domain.CampaignCreator` для API.

### Audit-row

- `action`: `"campaign_creator.ticket_sent.set"` если `ticketSent=true`,
  иначе `"campaign_creator.ticket_sent.unset"`.
- `entity_type = "campaign_creator"`, `entity_id = row.ID`.
- `actor_id` — из `context.WithValue` (admin user id из middleware).
- Payload — минимум `campaign_id` и `creator_id` для удобства фильтра.

### Пустой patch body

`patch.TicketSent == nil` (никакого поля не пришло) — 422 ещё **в
handler** (формат / минимум одно поле), до сервиса.

## Codegen

После правки `backend/api/openapi.yaml` (новый `PATCH`-эндпоинт,
`CampaignCreatorPatchInput`, поле `ticketSentAt` в `CampaignCreator`) —
обязательный прогон `make generate-api`. Это синхронизирует:
`backend/internal/api/server.gen.go`, `frontend/web/src/api/generated/schema.ts`,
`backend/e2e/apiclient/{types,client}.gen.go`. Ручные правки generated-файлов
запрещены (`backend-codegen.md`).

## Handler

`CampaignCreatorHandler.PatchParticipation` (oapi-codegen strict-server):

1. Авторизация — `authzService.RequireRole(ctx, Admin)` (как и существующие
   `addCampaignCreators` / `removeCampaignCreator`).
2. Валидация: `req.Body.TicketSent == nil` (вся структура пустая) → 422,
   `CodeValidation`, message «Передайте хотя бы одно поле для обновления».
3. `service.PatchParticipation(ctx, campaignID, creatorID, req.Body)`.
4. Маппинг ошибок: `errors.As BusinessError(CodeNotFound)` → 404;
   `ValidationError` → 422; иначе — стандартная 500.
5. Response 200 — обновлённый `CampaignCreator` целиком.

## UI

Файлы: `frontend/web/src/features/campaigns/creators/`.

- В `CampaignCreatorsTable.tsx` добавляется колонка «Билет отправлен» —
  показывается **только** в группе со `status === 'signed'`
  (контролируется через prop / условный рендер заголовка и ячеек).
- Чекбокс — компонент из существующей UI-библиотеки. Атрибуты:
  - `aria-label="Билет отправлен"`,
  - `data-testid="campaign-creator-ticket-sent-{creatorId}"`,
  - `disabled` пока `mutation.isPending`.
- Текст колонки — через `react-i18next`, ключ
  `campaigns.creators.ticketSent` в `locales/ru/campaigns.json`.

### Хук `usePatchCampaignCreator(campaignId)`

`useMutation`:

- `mutationFn` — openapi-fetch `PATCH /campaigns/{id}/creators/{creatorId}`.
- `onMutate(variables)` — optimistic update: записать новое
  `ticketSentAt` в react-query кэш по
  `campaignCreatorKeys.list(campaignId)` сразу. Сохранить snapshot для
  отката.
- `onError(_, _, ctx)` — откатить snapshot + toast «Не удалось обновить
  отметку. Попробуйте ещё раз».
- `onSettled` — `invalidateQueries(campaignCreatorKeys.list(campaignId))`
  (страхует от рассинхронизации; свежий снапшот из ответа уже в кэше).

Double-submit guard: достаточно `disabled` от `isPending` (одна row —
одна mutation, гонок нет). Если бы операция была медленной и админ
успевал ткнуть второй раз — добавили бы external `isSubmitting` per
row.

## Тесты

### Backend unit

- **Repo** (`campaign_creator_test.go`):
  `TestCampaignCreatorRepository_UpdateTicketSentAt` — `t.Run`'ы:
  set (timestamp + RETURNING row), unset (NULL + RETURNING row),
  not found (0 rows → `sql.ErrNoRows`), error propagation. Точные SQL
  + аргументы через pgxmock.
- **Service**: `TestCampaignCreatorService_PatchParticipation` —
  not found, status != signed → 422, no-op early return,
  set (`nil → time`), unset (`time → nil`), audit-row внутри tx
  (capture аргументов через `mock.Run`).
- **Handler**: `TestCampaignCreatorHandler_PatchParticipation` —
  empty body → 422, service ValidationError → 422,
  service NotFound → 404, success → 200 с корректным телом и
  captured-input для admin-actor из middleware-context.

### Backend e2e

Дополняем существующий `backend/e2e/campaign/campaign_test.go` —
сценариями PATCH: set, unset, status != signed → 422, несуществующий
creator → 404, пустой body → 422, проверка audit-row через
`testutil.AssertAuditEntry`. Без нового файла.

### Frontend unit

- `usePatchCampaignCreator.test.ts` — мок openapi-fetch: success →
  cache обновлён; error → snapshot откатывается, toast вызван.
- В тестах `CampaignCreatorsTable` (если есть, иначе рядом): чекбокс
  рендерится только в группе `signed`, клик вызывает мьютейшен,
  `disabled` пока pending.

### Frontend e2e (Playwright)

Дополняем существующий
`frontend/e2e/web/admin-campaign-creators-mutations.spec.ts` —
один happy-flow `test()`: seed creator со статусом `signed` →
клик чекбокса → проверить, что после ответа `data-testid` в
checked-состоянии. Без нового файла.
