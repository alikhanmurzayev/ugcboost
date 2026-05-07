---
title: "Design: добавление креатора в кампанию (chunks 10–15 campaign-roadmap)"
type: design
status: living
created: "2026-05-07"
updated: "2026-05-07"
---

# Design: процесс добавления креатора в кампанию

Living-документ уровня группы чанков 10–15 в `campaign-roadmap.md`. Описывает картину мира и продуктовые/архитектурные решения, к которым пришли в ходе дизайн-сессии 2026-05-07. Каждый из 6 PR-чанков (см. секцию «Нарезка на 6 PR-чанков») реализуется отдельной спекой через `bmad-quick-dev` со ссылкой на этот документ как на источник правды.

## Тезис

Проектируем M2M `campaign_creators` со state-машиной от ручного распределения до момента «креатор согласился». В админке три независимых ручных действия:

- add/remove креатора в кампанию,
- разослать первое/повторное приглашение выбранным,
- разослать ремайндер тем, кого пригласили, но не ответил.

Бот шлёт сообщение с одной inline-кнопкой → открывает TMA-страницу по секретному токену. В TMA креатор видит ТЗ и жмёт «согласиться» / «отказаться» — действия идут на бэк-ручки, идентифицирующие кампанию по `secret_token` и креатора по Telegram initData. Висячие случаи (приглашён без ответа / отказался) видны админу — чтобы менять состав и слать ремайндеры.

## Scope

- chunks 10–15 из `campaign-roadmap.md` (Группы 4, 5, 6).
- TrustMe-механика (chunks 16/17) и связанные с ней статусы / ручки / ремайндер по подписанию — **полностью out of scope** этого документа; будет покрыто отдельным design'ом.
- Группы 8 (pre-event reminder), 9 (билеты) — за scope'ом.

## Архитектурные опорные точки

- Telegram-бот — основной канал инициации коммуникации (рассылки сообщений со ссылкой). Сбор решения креатора по приглашению (согласие/отказ) — через TMA-страницу по `tma_url`. Inline-callback бота для решений по конкретной кампании в этом дизайне не используем.
- Кампании создаёт только админ; этот документ — про распределение креаторов в уже созданную кампанию.
- Все mutate-ручки — с audit + e2e.
- UI-эталоны для админки: `features/creators/`, `features/campaigns/`, `features/creatorApplications/`, `features/auth/`, `features/dashboard/`.
- Стандарты `docs/standards/` обязательны при реализации.

## Состояния `campaign_creators`

1. **«Запланирован»** — admin добавил в кампанию, приглашение ещё не отправлено
2. **«Приглашён»** — admin разослал приглашение в бот, креатор не отреагировал
3. **«Отказался»** — креатор нажал «отказаться» в TMA
4. **«Согласился»** — креатор нажал «согласиться» в TMA (терминальный для текущего scope; дальше — TrustMe в будущей сессии)

## Переходы

| Триггер | Кто | Из | В |
|---|---|---|---|
| add креатора в кампанию | admin | — | Запланирован |
| remove из кампании | admin | Запланирован / Приглашён / Отказался | (запись удалена) |
| отправить приглашение | admin | Запланирован / Отказался | Приглашён |
| ремайндер по приглашению | admin | Приглашён | Приглашён (фиксируем timestamp/попытку) |
| клик «согласиться» в TMA | креатор | Приглашён | Согласился |
| клик «отказаться» в TMA | креатор | Приглашён | Отказался |

**Правило remove:** удалить можно только до того, как креатор согласился. Из «Согласился» — нельзя (запись остаётся для TrustMe в будущей сессии).

**Повторное приглашение:** «Отказался» → «Приглашён» — тот же admin-action. Ограничения попыток нет.

**Идемпотентность TMA-кликов:** повторный клик на «согласиться» из «Согласился» → 200, ничего не меняем. Клик из несовместимого статуса (Запланирован, Отказался без re-invite, любой другой) → 422.

## Поля `campaign_creators`

| Поле | Тип | Описание |
|---|---|---|
| `id` | UUID PK | |
| `campaign_id` | UUID FK → campaigns(id) | |
| `creator_id` | UUID FK → creators(id) | |
| `status` | enum (4 значения) | текущее состояние |
| `invited_at` | TIMESTAMPTZ NULL | время последней рассылки приглашения |
| `invited_count` | INT NOT NULL DEFAULT 0 | сколько раз слали приглашение (включая повторные после отказа) |
| `reminded_at` | TIMESTAMPTZ NULL | время последнего ремайндера по приглашению |
| `reminded_count` | INT NOT NULL DEFAULT 0 | счётчик ремайндеров по приглашению |
| `decided_at` | TIMESTAMPTZ NULL | время последнего решения креатора (согл./отказ) |
| `created_at`, `updated_at` | TIMESTAMPTZ DEFAULT now() | |

UNIQUE `(campaign_id, creator_id)` — жёсткий, raз remove физически удаляет запись.

История переходов и попыток — в `audit_logs` (хранилище). В UI отображаем только текущее состояние + последние timestamp'ы / счётчики.

## TMA secret_token — извлекаем из `tma_url`

Отдельного поля в `campaigns` не заводим (миграции расширения нет). Секретный токен — последний path-segment у `tma_url`. Парсинг строгий: ожидаем формат `https://<tma-host>/<token>`, где `<token>` — непустой непробельный segment без слэшей. На бэке для TMA-ручек — lookup кампании по `tma_url LIKE '%/' || token`. Допустимые символы и длина токена — фиксируются в стандарте формата при реализации (валидируется при создании/edit кампании).

## API-поверхность

Admin-ручки (auth: ROLE=ADMIN):

| # | Метод | Путь | Назначение |
|---|---|---|---|
| A1 | POST | `/campaigns/{id}/creators` | add (батч `creator_ids[]`) — все добавляются в статус «Запланирован» |
| A2 | DELETE | `/campaigns/{id}/creators/{creator_id}` | remove (422 если статус ≥ «Согласился») |
| A3 | GET | `/campaigns/{id}/creators` | список креаторов в кампании со статусами и счётчиками |
| A4 | POST | `/campaigns/{id}/notify` | приглашение по явному `creator_ids[]` |
| A5 | POST | `/campaigns/{id}/remind-invitation` | ремайндер по явному `creator_ids[]` |

TMA-ручки (auth: Telegram initData; идентифицируют креатора по `telegram_user_id`):

| # | Метод | Путь | Назначение |
|---|---|---|---|
| T1 | POST | `/tma/campaigns/{secret_token}/agree` | согласиться |
| T2 | POST | `/tma/campaigns/{secret_token}/decline` | отказаться |

## Поведение ручек: ошибки и idempotency

- **A1/A4/A5 — validation batch:** strict-422 на весь батч, если хотя бы один `creator_id` в несовместимом состоянии или невалиден. Никаких частичных вставок / нотификаций.
- **A4/A5 — runtime delivery:** **partial-success** (детали в § Edge cases). Принципиально отличается от validation: успешная validation → пытаемся доставить всем; кому не доставили — фиксируем в `undelivered: [...]` ответа, но не падаем.
- **A2 remove:** 404 если креатор не в кампании; 422 если статус ≥ «Согласился».
- **A3 list:** без пагинации/фильтров — весь список одной выдачей.
- **TMA auth (T1/T2):** Telegram initData в header → HMAC валидация → `telegram_user_id` → match на `creators.telegram_user_id`. Если креатор не найден или не в этой кампании — 403; в несовместимом статусе («Запланирован» / «Отказался» / decline-из-«Согласился») — 422; кампания soft-deleted — 404. Повторный клик «согласиться» из «Согласился» — 200 идемпотентно.

## Счётчики и idempotency

- `invited_count` — инкрементируется при каждом успешном A4. Не сбрасывается никогда.
- `reminded_count` — инкрементируется при каждом успешном A5. **Сбрасывается** в 0 при следующем успешном A4 (после Отказался → Приглашён).
- Аналогично сбрасываются `reminded_at` (NULL) и `decided_at` (NULL) при новом A4 — текущий цикл начинается «с нуля».

## TMA-страница: контент и поведение

- Контент ТЗ хардкодится прямо в TMA-приложении: каждая кампания — отдельный route по `secret_token` с зашитым ТЗ + любые референсы. Приватной части ТЗ нет — креатор видит ТЗ целиком при первом открытии страницы.
- Бэк-API ТЗ не отдаёт; TMA шлёт только agree/decline.
- TMA всегда показывает полный UI (ТЗ + 2 кнопки) — никаких проверок статуса на фронте. Реакция на клик — то, что вернёт бэк:
  - `Запланирован` → 422 (приглашения не было)
  - `Приглашён` → 200, переход в `Согласился`/`Отказался`
  - `Отказался` → 422 на оба действия (требуется admin re-invite)
  - `Согласился` → 200 idempotent на agree; 422 на decline
  - кампания soft-deleted → 404
  - креатор не в кампании / не найден в creators по `telegram_user_id` → 403

## Тексты сообщений в боте

- Универсальные, **не содержат деталей конкретной кампании** (ни имени, ни ТЗ).
- Тело: предложение перейти по ссылке (приглашение) / напоминание перейти по ссылке (ремайндер). Ссылка = `tma_url` кампании.
- Точные формулировки прорабатываются на этапе реализации (копирайт). Тексты легко меняются позже.

## Audit-события

Принцип: **один event на креатора** (per-creator, не per-batch). Все события пишутся в той же транзакции, что и mutate-операция (`backend-transactions.md` § Аудит-лог).

| Event | Триггер | Actor | Payload |
|---|---|---|---|
| `campaign_creator.added` | A1 | admin user_id | `{campaign_id, creator_id}` |
| `campaign_creator.removed` | A2 | admin user_id | `{campaign_id, creator_id}` |
| `campaign_creator.invited` | A4 | admin user_id | `{campaign_id, creator_id, attempt_no}` |
| `campaign_creator.reminded` | A5 | admin user_id | `{campaign_id, creator_id, attempt_no}` |
| `campaign_creator.agreed` | T1 | NULL (public) | `{campaign_id, creator_id}` |
| `campaign_creator.declined` | T2 | NULL (public) | `{campaign_id, creator_id}` |

`attempt_no` для invited/reminded — равен пост-инкрементному значению `invited_count` / `reminded_count`.

Read-only ручки (A3) — без audit (по стандарту).

## Authz

- A1-A5 — `ROLE=ADMIN` через `AuthzService`. Прямые сравнения ролей в handler запрещены (стандарт).
- T1-T2 — без role-auth; идентификация через TMA initData (HMAC) → `creators.telegram_user_id`.

## Edge cases

- **Soft-deleted кампания** (`campaigns.is_deleted = true`): любая ручка на неё (A1-A5, T1, T2) → **404**. Старые ссылки в боте перестают работать сразу после soft-delete; креатор увидит ошибку доставки от TMA.
- **Бот заблокирован у креатора (runtime delivery error на A4/A5):** partial-success.
  - Кому доставили — `invited_count`/`reminded_count` инкрементируются, `invited_at`/`reminded_at` обновляются, audit `invited`/`reminded` пишется.
  - Кому не доставили — счётчики/timestamps **не меняются**, audit для них **не пишется**. Админ через A3 увидит, что у этих креаторов старая дата приглашения / нет приглашения вообще.
  - Response 200 с полями `delivered_count` + `undelivered: [{creator_id, reason}]`. Фронт показывает админу понятную ошибку со списком недоставленных.
  - Validation `strict-422` остаётся: если хоть один `creator_id` в несовместимом статусе — весь batch отклоняется ДО попытки доставки.
- **Редактирование `tma_url` после рассылок:** PATCH-ручка обновления кампании отклоняет смену `tma_url` (422) если в `campaign_creators` этой кампании есть запись с `invited_count > 0`. Это ограничение реализуется в chunk 12.

## Индексы в миграции

Только UNIQUE `(campaign_id, creator_id)` — даёт композитный btree, покрывает A3 lookup. Дополнительных индексов сейчас не делаем (нагрузка низкая, таблица маленькая).

## Тестирование

Преемственно по `docs/standards/`. Специфично для этого design'а:

**Backend unit:**
- handler / service / repository ≥80% per-method (gate `make test-unit-backend-coverage`).
- Повторный A1 add того же креатора (не concurrent) → `ErrCreatorAlreadyInCampaign` (translation pgErr 23505 → domain). Race-теста на concurrent insert не делаем — одного non-concurrent кейса достаточно.
- TMA initData-валидация — блок тестов middleware.

**Backend e2e:**
- Full-flow на бизнес-ручках: create campaign → seed creators (через approve flow / test-API) → A1 add → A4 notify → A3 assert статус и audit-rows → emulate TMA T1 agree через test-helper, генерирующий валидный initData для `creators.telegram_user_id` → assert финальный статус, счётчики, audit.
- Strict-422 на batch с одним невалидным `creator_id` → полный rollback (никаких частичных вставок / нотификаций).
- Re-invite после отказа: A1 → A4 → T2 decline → A4 → assert `reminded_count = 0`, `reminded_at = NULL`, `decided_at = NULL`, `invited_count = 2`.
- Бот-нотификации проверяем через `internal/telegram/spy_store` — assert на тип сообщения / число вызовов / содержимое (`tma_url`).
- Все поля контракта — строгие assert'ы конкретными значениями, не «не пустое».

**Frontend:**
- TMA Playwright smoke: клик agree/decline на хардкодной странице по `secret_token`.
- Admin Playwright e2e — отдельные spec'и под chunks 11 / 13 / 15.

**Edge-case e2e:**
- Soft-deleted кампания → 404 на A1-A5, T1, T2.
- Partial-success A4: spy-notifier настроен на fail для одного `creator_id` → response 200 с `undelivered: [{...}]`, в БД у failed-креатора timestamps/счётчики не изменены, audit `invited` не записан.
- PATCH `tma_url` после первой рассылки → 422.

**Self-check агента** между unit и e2e — обязателен (curl + чтение БД + spy_store).

## Нарезка на 6 PR-чанков

| Chunk | Слой | Содержимое |
|---|---|---|
| **10** | бэк | миграция `campaign_creators` + A1 add + A2 remove + A3 list. Без бот-нотификаций, без TMA |
| **11** | фронт-admin | UI выбора креаторов в кампанию (использует A1/A2/A3 + список из chunk 1) |
| **12** | бэк | A4 notify + A5 remind, нотификации через `internal/telegram/notifier`, partial-success delivery, ограничение PATCH `tma_url` после рассылок |
| **13** | фронт-admin | кнопки «разослать приглашение» / «разослать ремайндер» + UI выбора подмножества + отображение undelivered |
| **14** | бэк + TMA | T1 agree + T2 decline, TMA initData middleware, TMA-страницы по `secret_token` (хардкод полного ТЗ + 2 кнопки) |
| **15** | фронт-admin | расширение страницы кампании: статусы, счётчики, последние timestamps (использует A3) |

Каждый chunk = отдельная spec через bmad-quick-dev = отдельный PR. Параллелизм: 12 стартует параллельно как только 10 в main; 11/13/15 — последовательно после своих бэк-зависимостей.
