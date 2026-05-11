# Intent: ремайндер креаторам в статусе «подписывает договор»

## Преамбула: стандарты

Перед реализацией агент обязан полностью прочитать `docs/standards/` —
все файлы целиком. Этот интент описывает только дельту над существующим
кодом; все общие правила (слои, codegen, EAFP, RepoFactory,
audit-в-tx, naming, security, тесты) живут в стандартах и применяются
без исключений.

Ключевые стандарты для этой фичи:
`backend-architecture.md`, `backend-codegen.md`, `backend-errors.md`,
`backend-repository.md`, `backend-transactions.md`,
`backend-testing-unit.md`, `backend-testing-e2e.md`,
`frontend-api.md`, `frontend-components.md`,
`frontend-testing-unit.md`, `frontend-testing-e2e.md`,
`naming.md`, `security.md`.

## Проблема

Некоторые креаторы зависают в статусе `signing` — контракт отправлен
через TrustMe, СМС-ссылка пришла, но креатор не подписал и молчит.
Возможные причины: не понял что надо сделать, потерял СМС, отложил и
забыл. У админа сейчас нет рычага влияния — кнопок-ремайндеров для
этой группы на странице кампании нет (есть только notify для
`planned`/`declined` и remind-invitation для `invited`).

## Тезис

Добавляем третий ремайндер по симметрии с `remind-invitation`:
кнопка в группе `signing` → новый эндпоинт
`POST /campaigns/{id}/remind-signing` → новая `batchOpSpec` с
`allowedStatuses={signing}` и без `requireContractTemplate`,
переиспользует `repo.ApplyRemind` (инкремент `reminded_count` +
обновление `reminded_at`, без смены статуса) и шлёт через
`notifier.SendCampaignInvite` новый текст-напоминание про
СМС-ссылку TrustMe. Миграции не нужны — все колонки уже есть.

## User flow (админ-UI)

На странице деталей кампании (`/campaigns/:id`) в секции
«Креаторы» — группа `signing` (заголовок «Подписывают договор»).
Добавляем для этой группы те же контролы, что у группы `invited`:

- Чекбокс на каждой строке + общий «выбрать все».
- Счётчик выбранных.
- Кнопка «Разослать ремайндер» (та же копия что у `invited`),
  состояние loading — «Отправка…».
- По клику → мутация → POST на новый эндпоинт со списком выбранных
  `creatorIds` → результат: список `undelivered` отображается через
  тот же существующий механизм toast/inline-feedback, что и для
  `remind-invitation`.
- После успеха — invalidate query на список креаторов кампании,
  чтобы `reminded_count` / `reminded_at` обновились в UI.

Статус строки не меняется (остаётся `signing`) — только метрики
ремайндеров обновляются.

## API surface

Новый эндпоинт, симметричный `POST /campaigns/{id}/remind-invitation`:

```
POST /campaigns/{id}/remind-signing
operationId: remindCampaignCreatorsSigning
auth: admin-only (тот же middleware что у соседних эндпоинтов)
request: CampaignCreatorBatchInput (переиспользуем — { creatorIds: uuid[1..200] })
responses:
  200: CampaignNotifyResult (переиспользуем — { data: { undelivered: [...] } })
  404: campaign not found / soft-deleted
  422 CAMPAIGN_CREATOR_BATCH_INVALID: ≥1 креатор не прикреплён или не в статусе signing
```

Никаких новых схем — только новый path. Схемы
`CampaignCreatorBatchInput` и `CampaignNotifyResult` уже есть в
`backend/api/openapi.yaml`. В `description` нового path-секции явно
прописать симметрию с `remind-invitation` и что статус **не**
меняется (только `reminded_count` / `reminded_at`).

В отличие от `notify` — **нет** проверки `requireContractTemplate`:
для creator в статусе `signing` контракт уже отправлен и шаблон
кампании уже существует (в этот статус нельзя попасть без него).

## Текст уведомления (Telegram)

Существующий текст при переходе в `signing` (`campaignContractSentText`
в `backend/internal/telegram/notifier.go`):

> Мы отправили вам соглашение на подпись по СМС на номер телефона,
> указанный при регистрации 📄
>
> Перейдите по ссылке из СМС и подпишите соглашение
>
> Если есть вопросы, можете обратиться к @aizerealzair

Для ремайндера — тот же контекст, но с напоминающим заголовком и
без эмодзи в начале (по аналогии с `campaignRemindInvitationText`,
который короче и суше первичного `campaignInviteText`). Предложенный
текст (новый константный литерал
`campaignRemindSigningText`):

> Напоминаем, что мы отправили вам соглашение на подпись по СМС на
> номер телефона, указанный при регистрации.
>
> Перейдите по ссылке из СМС и подпишите соглашение.
>
> Если есть вопросы, можете обратиться к @aizerealzair

Текст — living. Контакт `@aizerealzair` копируется как есть (он
уже фигурирует в `campaignContractSentText` — единое контактное
лицо для договорных вопросов).

Доставка — через тот же `notifier.SendCampaignInvite(ctx, chatID,
text, tmaURL)`, что и `remind-invitation`. WebApp-кнопка
«Посмотреть» в TMA остаётся — она ведёт в карточку кампании, где
креатор увидит свой статус и контекст. Отдельный метод в notifier
не нужен.

## Изменения по слоям бэкенда

### Domain / audit constants

В `backend/internal/service/audit_actions.go` (или где живёт
константа `AuditActionCampaignCreatorRemind`) — добавить:

```go
AuditActionCampaignCreatorRemindSigning = "campaign_creator_remind_signing"
```

Существующий `AuditActionCampaignCreatorRemind` оставляем для
`remind-invitation` (его не переименовываем, чтобы не делать
исторический хвост в `audit_logs`).

### Telegram notifier

В `backend/internal/telegram/notifier.go`:
- Добавить константу `campaignRemindSigningText` с текстом выше.
- Добавить экспортируемый геттер `CampaignRemindSigningText() string`
  по симметрии с `CampaignRemindInvitationText()`.

### Service: новая batchOp

В `backend/internal/service/campaign_creator.go`:

1. Новая константа `batchOpRemindSigning batchOp = "remind_signing"`.
2. Новая запись в `batchOpSpecs`:
   ```go
   batchOpRemindSigning: {
       allowedStatuses: map[string]bool{
           domain.CampaignCreatorStatusSigning: true,
       },
       auditAction: AuditActionCampaignCreatorRemindSigning,
       text:        telegram.CampaignRemindSigningText(),
       apply: func(r repository.CampaignCreatorRepo, ctx context.Context, id string) (*repository.CampaignCreatorRow, error) {
           return r.ApplyRemind(ctx, id)
       },
       // requireContractTemplate: false — креатор уже в signing,
       // контракт уже отправлен.
   },
   ```
3. Новый публичный метод `RemindSigning(ctx, campaignID, creatorIDs)`
   — тонкая обёртка вокруг `s.dispatchBatch(ctx, ..., batchOpRemindSigning)`,
   ровно как `RemindInvitation`.

Никаких изменений в `dispatchBatch`, `applyDelivered`,
`getActiveCampaign` — они уже параметризованы через `batchOpSpec`.

### Repository

Изменений нет. `ApplyRemind` уже умеет инкрементировать
`reminded_count` и обновлять `reminded_at` без смены статуса —
ровно то, что нужно. Партиально-уникального индекса, чувствительного
к статусу `signing`, в схеме нет.

### Handler

В `backend/internal/handler/campaign_creator.go` — новый метод
`RemindCampaignCreatorsSigning`, копия `RemindCampaignCreatorsInvitation`
с заменой вызова сервиса на `RemindSigning`. Обработка ошибок
(404 `ErrCampaignNotFound`, 422 `CampaignCreatorBatchInvalidError`)
наследуется от общего error-маппера — отдельной обработки не
требуется.

### OpenAPI

В `backend/api/openapi.yaml` — новый path-блок
`/campaigns/{id}/remind-signing` с `operationId:
remindCampaignCreatorsSigning`, request `CampaignCreatorBatchInput`,
responses 200 `CampaignNotifyResult` / 404 / 422
`CAMPAIGN_CREATOR_BATCH_INVALID`. В `description` сослаться на
симметрию с `remind-invitation` и подчеркнуть, что статус не
меняется.

После правки openapi.yaml — `make generate-api` (perегенерирует
chi-server, openapi-fetch типы, e2e-клиенты).

### Routing

После `make generate-api` chi-server роутер автоматически подцепит
новый эндпоинт через `ServerInterfaceWrapper`. Ручной регистрации
роута не делаем.

### Миграции

Не нужны. Все колонки (`reminded_count`, `reminded_at`,
`updated_at`) и значения enum уже существуют.

## Изменения по фронту (web)

### API-слой

`frontend/web/src/api/campaignCreators.ts` — добавить функцию
`remindCampaignCreatorsSigning(campaignId, creatorIds)`, копия
`remindCampaignCreatorsInvitation` с заменой path на
`/campaigns/{id}/remind-signing`.

### Хук мутаций

`frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.ts`
— добавить третий хук/поле `remindSigning` в возвращаемый объект,
который вызывает новую API-функцию. Сигнатура и обработка
`undelivered` идентичны существующим.

### Маппинг статуса на действие

`CampaignCreatorsSection.tsx`, функция `actionForStatus()` —
добавить case:

```ts
case CAMPAIGN_CREATOR_STATUS.SIGNING:
  return {
    actionLabel: t("campaignCreators.remindSigningButton"),
    actionSubmittingLabel: t("campaignCreators.remindSigningSubmitting"),
    mutation: mutations.remindSigning,
  };
```

Остальные case (`SIGNED`, `SIGNING_DECLINED`, `AGREED`) остаются с
пустым объектом — для них кнопок нет.

### i18n

`frontend/web/src/shared/i18n/locales/ru/campaigns.json`, секция
`campaignCreators` — добавить ключи:

```json
"remindSigningButton": "Разослать ремайндер",
"remindSigningSubmitting": "Отправка…"
```

Тексты копируют `remindButton` / `remindSubmitting` — кнопка делает
то же самое по сути (рассылка напоминания), отличается только
целевой статус. Если в будущем захотим разные копии — переименуем
точечно, в одном месте.

### Query invalidation

Должна работать автоматически — мутация (по аналогии с
`remind-invitation`) инвалидирует query со списком креаторов
кампании. Никаких новых query keys не вводим.

### TMA / Landing

Изменений нет. Это админская функция, в TMA не отображается.

## Audit

Audit-row пишется в той же per-creator транзакции что и
`ApplyRemind` — это уже обеспечено `dispatchBatch.applyDelivered`.
Новый action — `campaign_creator_remind_signing`. Snapshot до/после
— `oldCC` (status=signing, reminded_count=N) и `newCC`
(status=signing, reminded_count=N+1, reminded_at обновлён).

Никаких дополнительных полей в `audit_logs` не нужно — текущая
схема покрывает.

## Тесты

Качество тестов — обязательно, не «галочка». Все правила из
`backend-testing-unit.md`, `backend-testing-e2e.md`,
`frontend-testing-unit.md`, `frontend-testing-e2e.md` —
применяются буквально.

### Backend unit — service

`backend/internal/service/campaign_creator_test.go` —
`TestCampaignCreatorService_RemindSigning` по аналогии с
`TestCampaignCreatorService_RemindInvitation`. Каждый `t.Run`
с `t.Parallel()`, **новый набор моков на каждый сценарий** (без
протекания между t.Run). Порядок сценариев — по ходу
исполнения кода: ранние выходы первыми, happy path в конце.

Обязательные сценарии:

- campaign not found / soft-deleted → `ErrCampaignNotFound`
  (ассерт через `require.ErrorIs`, моки на CampaignRepo).
- creator не прикреплён к кампании → 422
  `CampaignCreatorBatchInvalidError` с `reason=not_in_campaign`
  для конкретного creatorId; **ничего** не отправлено
  (notifier.SendCampaignInvite не вызван — `AssertNotCalled`).
- creator в недопустимом статусе — table-driven:
  `planned` / `invited` / `agreed` / `signed` /
  `signing_declined` каждый → 422 c `reason=wrong_status` и
  правильным `current_status`. Для каждой ветки —
  `AssertNotCalled` на notifier и `ApplyRemind`.
- Telegram delivery failed → `undelivered` содержит correct
  reason от `MapTelegramErrorToReason` (`bot_blocked`, `unknown`).
  Проверяем: `ApplyRemind` **не** вызван для этого creator;
  для остальных в батче — вызван. Status и `reminded_count` не
  меняются на failed-row.
- Missing `telegram_user_id` (creator hard-deleted между
  validate-pass и delivery) → `undelivered` reason=unknown,
  error залогирован (через capturing logger), цикл продолжает.
- Send OK + `ApplyRemind` ошибка → `undelivered` reason=unknown,
  error залогирован, остальной батч продолжает (per-creator
  tx изолированы).
- Happy path с батчем 3 creators → пустой `undelivered`,
  per-creator: `ApplyRemind` вызван **с правильным id**
  (`mock.EXPECT().ApplyRemind(ctx, "exact-uuid")`), audit-row
  написан с `action=campaign_creator_remind_signing`,
  `entity_type=campaign_creator`, snapshot before (status=signing,
  reminded_count=N) и after (status=signing,
  reminded_count=N+1, `reminded_at` свежий).
- **Captured-input** на notifier: через `mock.Run(func(args
  mock.Arguments) { ... })` достать `text` параметр и
  убедиться что это именно `telegram.CampaignRemindSigningText()`,
  **не** `CampaignRemindInvitationText` (защита от
  copy-paste-бага при добавлении batchOpSpec).

Проверка `t.Parallel()` на функции теста и на каждом t.Run.
Использовать новые моки внутри каждого `t.Run`, не общие.

### Backend unit — handler

`backend/internal/handler/campaign_creator_test.go` —
`TestCampaignCreatorHandler_RemindSigning`. Чёрный ящик через
`httptest`:

- 200: валидный батч → mock service.RemindSigning возвращает
  пустой undelivered → response — типизированный
  `CampaignNotifyResult` с пустым списком, status 200.
  Через `httptest.ResponseRecorder` → `json.Unmarshal` в
  generated `CampaignNotifyResult` → `require.Equal` целиком.
- 200 с partial: service возвращает `undelivered` с 1 элементом
  → response содержит этот элемент. Поля сравниваются типизированно.
- 422: service возвращает `CampaignCreatorBatchInvalidError` с
  `details` → handler возвращает 422 с `code=
  CAMPAIGN_CREATOR_BATCH_INVALID` и полным details-массивом.
- 404: service возвращает `ErrCampaignNotFound` → handler
  возвращает 404 с правильным error code.
- Body validation на input: пустой `creatorIds` / >200 → 422
  (через generated server interface wrapper, должен сработать
  schema-валидатор автоматически).
- Captured input в service.RemindSigning: через `mock.Run`
  убедиться что в service ушли **те же** creatorIds что в
  request body, и **тот же** campaignID что в path-param.
- Сырой JSON в тестах не использовать — request/response через
  типизированные структуры из `api/`.

### Backend моки

После добавления `RemindSigning` в `CampaignCreatorService` и
нового `auditAction` — `make generate-mocks` (mockery с
`all: true`), не ручные моки.

### Backend coverage / race

`make test-unit-backend-coverage` должен зелёный пройти —
каждый publically-видимый identifier нового кода ≥ 80%.
`go test -race` — обязательно, отключать запрещено.

### Backend e2e

`backend/e2e/campaign/campaign_test.go` (или подходящий
test-файл) — расширить существующий narrative test или
завести новый `TestCampaignCreatorRemindSigning`. Header-doc
на русском, нарратив (не bullet-list), упомянуть testapi,
testid-ключи если применимо, `E2E_CLEANUP`.

Setup-цепочка через `testutil` (composable хелперы, без
дублирующего локального кода): admin → brand → creator
(application approved → handle confirmed) → campaign с
contract template → add creator → notify (→ invited) → TMA
decision agree (→ agreed) → TrustMe spy-сценарий контракт
отправлен (→ signing).

Сценарии (внутри одного `Test*` через `t.Run`, каждый с
`t.Parallel()` если изолирован, либо последовательно если
делят setup):

- Happy path: POST `/campaigns/{id}/remind-signing` с
  валидным батчем → 200, `undelivered=[]`. Проверки через
  типизированные структуры из generated client:
  - GET `/campaigns/{id}/creators` → `reminded_count` = 1,
    `reminded_at` свежий (через `require.WithinDuration`
    с запасом 1 мин), `status=signing` без изменений.
  - Audit-row через `testutil.AssertAuditEntry` —
    `action=campaign_creator_remind_signing`, entity и
    before/after snapshot корректны.
  - Telegram-spy ассертит вызов с `chat_id` креатора и
    текстом из `CampaignRemindSigningText()`.
- Повторный remind-signing на тот же creator → 200,
  `reminded_count` = 2, новый audit-row.
- 422 wrong_status: пытаемся remind-signing для creator в
  статусе `agreed` (другой creator из того же setup) →
  `CAMPAIGN_CREATOR_BATCH_INVALID`, `current_status=agreed`.
  GET creators — ничего не изменилось, audit-row нет,
  telegram-spy не вызван.
- 422 not_in_campaign: creatorId не привязан к этой кампании
  → reason `not_in_campaign`.
- 404: soft-deleted campaign → `ErrCampaignNotFound`-аналог.
- Partial success: 2 creators в signing, один с
  заблокированным ботом (mock-trigger) → 200 с `undelivered`
  длины 1, reason `bot_blocked`; для второго —
  `reminded_count` инкрементирован, audit-row есть.

Hardcoded дат **нет** (относительные через `time.Now()`).
`t.Parallel()` на `Test*`, generated handles/emails, no
hardcoded seed-зависимостей.

### Frontend unit — хук мутации

`useCampaignNotifyMutations.test.ts` — расширить тестами на
`remindSigning`:

- Вызывает правильную API-функцию `remindCampaignCreatorsSigning`
  с правильным `campaignId` и `creatorIds`.
- При успехе возвращает корректный `CampaignNotifyResult`.
- При ошибке (4xx/5xx) — `onError` срабатывает, error не
  глотается.
- Мок самого API-клиента (не MSW — overhead).

### Frontend unit — компонент

`CampaignCreatorsSection.test.tsx` — добавить кейс:

- Для группы `signing` рендерится кнопка с текстом из
  `t("campaignCreators.remindSigningButton")` — i18n через
  реальный `I18nextProvider` с переводами (не мокать).
- Локатор — `getByRole('button', { name: ... })` или
  `getByTestId` (стабильный data-testid на кнопке).
- Loading state: при `isPending` кнопка disabled, текст —
  «Отправка…».
- Клик с выбранными чекбоксами → мутация вызвана с правильными
  creatorIds. Двойной клик блокирован (disabled / external
  isSubmitting flag).
- Loading / Error / Empty для группы `signing` обработаны как
  у других групп.

### Frontend e2e (Playwright)

В существующий cross-app кампанийный flow (или новый spec) —
секция про remind-signing. Header — `/** ... */` JSDoc, на
русском, нарратив. `data-testid` на новой кнопке.

Сценарий: после того как seed-helper приводит creator в
статус `signing` (через API-хелперы и spy TrustMe), админ
открывает страницу кампании, видит группу «Подписывают
договор», выбирает чекбоксом креатора, нажимает «Разослать
ремайндер» → ожидает toast/inline-сообщение «Доставлено N» →
повторно загружает счётчик «получили N напоминаний» (или
эквивалент) → инкремент виден.

`E2E_CLEANUP=true` по умолчанию, не оставлять `false` в
коммите.

### Локальный gate перед push

`make build-backend lint-backend test-unit-backend
test-unit-backend-coverage test-e2e-backend lint-web
test-unit-web test-e2e-frontend` — всё четыре стадии (build,
lint, unit+coverage, e2e), всё зелёное. Без bypass хуков.

## Cohesion check

- [x] Миграция БД — не нужна (все колонки и enum-значения уже есть).
- [x] Audit — `campaign_creator_remind_signing` в той же tx что
      `ApplyRemind`.
- [x] Frontend ↔ backend symmetric (path, schemas переиспользованы,
      mutation invalidation работает).
- [x] OpenAPI обновлён, `make generate-api` регенерирует клиентов.
- [x] Тексты Telegram — копируем стиль `campaignContractSentText`,
      выносим в константу с экспортируемым геттером.
- [x] Тесты — unit (service + handler) + e2e (backend + frontend).
- [x] Анти-fingerprinting / PII — не применимо: эндпоинт
      админ-only, входные данные — uuid[], никакого user-input в
      ошибки или логи не уходит.
- [x] Rate-limiting — не нужен, эндпоинт админ-only.
- [x] No-merge-into-running-agent: реализационный шаг откладывается
      до окончания параллельной задачи. Этот интент + последующая
      спека — статичные артефакты, готовые к hand-off.

## Открытые вопросы

Нет. Все детали выводятся из существующего кода `remind-invitation`
по симметрии.

## Следующий шаг

После того как параллельный агент закончит работу — handoff на
`bmad-quick-dev` с этим интентом в качестве входа. Спека
(`spec-{slug}.md`) генерируется уже там.
