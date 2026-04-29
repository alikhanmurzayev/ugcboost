---
title: "Scout: привязка Telegram-аккаунта к заявке (chunk 1 онбординга креатора)"
type: scout
status: ready-for-plan
created: "2026-04-29"
roadmap: "_bmad-output/planning-artifacts/creator-onboarding-roadmap.md"
chunk: 1
---

# Scout: привязка Telegram-аккаунта к заявке

> **Преамбула для следующего агента (планировщик / реализатор).**
> Перед любым действием по этому артефакту — **полностью** загрузить в контекст
> все файлы из `docs/standards/` (читать каждый файл целиком, без выборочного
> поиска). Это hard rule проекта (`feedback_artifacts_standards_preamble`).
> Стандарты обновляются как living documents, и любое расхождение с ними —
> finding. Без полной загрузки стандартов — не приступать.

---

## 1. Цель chunk

Связать поданную с лендинга заявку (`creator_applications.id`) с конкретным
Telegram-аккаунтом креатора. Триггер — нажатие кнопки на success-screen
лендинга, которая открывает `https://t.me/<bot>?start=<application_id>`.
Telegram автоматически отправляет команду `/start <application_id>` в наш
бот. Бот: (a) валидирует payload, (b) сохраняет привязку «Telegram user →
application», (c) отвечает креатору сообщением с подтверждением.

TMA в этом chunk **не делается** — бот пока умеет только `/start`. Архитектура
закладывается с расчётом на расширение (TMA, статус-сообщения от админа,
напоминания).

## 2. Текущее состояние (факты)

### 2.1 Что уже есть

- **Лендинг отдаёт `telegram_bot_url`.**
  `frontend/landing/src/api/creator-applications.ts` шлёт POST
  `/creators/applications`, в success-screen открывается
  `data.telegram_bot_url` (формат `https://t.me/<TELEGRAM_BOT_USERNAME>?start=<application_id>`).
- **Backend строит deep-link.** `handler/creator_application.go:89` —
  `buildTelegramBotURL`, `url.PathEscape(username)` + `url.QueryEscape(applicationID)`.
- **Config готов.** `config.Config.TelegramBotUsername` (required env);
  `TelegramMock bool` (для будущих режимов).
  `.env.example` — `TELEGRAM_BOT_USERNAME=ugcboost_staging_bot`.
- **Фича-флаг `TELEGRAM_MOCK=true`** в .env.example уже стоит.
- **Зарезервированный пакет.** `backend/internal/integration/telegram/`
  существует, пуст. Предусмотрен под интеграции (`telegram`, `livedune`,
  `trustme`, `email`, `storage` — все пустые).
- **Cron работает в том же процессе.** `cmd/api/main.go:67-75` запускает
  `cron.New(cron.WithSeconds())` и регистрирует в closer. Прецедент для
  background-goroutine рядом с HTTP-сервером есть.
- **Closer + LIFO graceful shutdown** работает: `internal/closer/closer.go`,
  shutdown в `main.go` выполняется по SIGINT/SIGTERM с таймаутом из
  `cfg.ShutdownTimeout`.
- **Repository-слой.** `creator_applications` — таблица из миграции
  `20260420181753_creator_applications.sql`. Status — `pending|approved|rejected|blocked`,
  partial unique index по `iin` для активных статусов.
  `CreatorApplicationRepo`: `HasActiveByIIN`, `Create`, `GetByID`, `DeleteForTests`.
  Pattern для добавления полей: миграция → row struct (`db:"col"`,
  опционально `insert:"col"`) → константа `CreatorApplicationColumn{Field}` →
  при необходимости новый метод репо.
- **Service layer.** `CreatorApplicationService` — `Submit` (с `dbutil.WithTx`
  и audit в той же tx), `GetByID`. `RepoFactory` объявлен в сервисе как
  интерфейс — добавление новой репы требует расширения этого интерфейса.
- **Audit.** `audit_constants.go` — пара `AuditAction*` / `AuditEntityType*`
  на каждое действие. Запись через `writeAudit(ctx, auditRepo, action,
  entityType, entityID, oldVal, newVal)`. `actor_id` уже nullable
  (`audit_logs_nullable_actor.sql`) — у бота актёра не будет (system action).
- **HTTP сервер устроен через codegen.** Любые публичные ручки —
  `backend/api/openapi.yaml` → `oapi-codegen` → `api.HandlerFromMux`.
  Прямой `r.Post(...)` запрещён (исключение — `health`).
- **Test endpoints.** `internal/handler/testapi.go` + `backend/api/openapi-test.yaml`
  включаются при `EnableTestEndpoints` (любой не-prod). Cleanup-stack
  e2e-тестов чистит `creator_application` через
  `POST /test/cleanup-entity?type=creator_application&id=...`.
- **Библиотечный реестр** (`docs/standards/backend-libraries.md`).
  Telegram-клиента в реестре нет — потребуется обоснование выбора и
  пополнение реестра тем же PR.

### 2.2 Чего нет

- В коде **нет ни одной строки**, обращающейся к Telegram Bot API.
  `git ls-files | grep -iE 'telegram|bot'` — пусто (кроме deep-link строк
  в config/handler/landing).
- Поля `telegram_user_id` (или связной таблицы) в схеме БД нет.
- Token бота не заведён в config (`TelegramBotToken` отсутствует).
- В `cmd/api/main.go` нет polling-loop / webhook-handler.
- В OpenAPI нет endpoint'а для приёма webhook update'ов (если идём в режиме
  webhook).

## 3. Затронутые области (предполагаемые файлы)

> Точный набор зафиксирует план. Здесь — карта зон интереса.

**Контракт / OpenAPI**
- `backend/api/openapi.yaml` — добавить webhook endpoint **только если**
  выбран режим webhook (для long polling — endpoint не нужен).

**Конфиг**
- `backend/internal/config/config.go` — `TelegramBotToken` (required),
  опционально `TelegramWebhookSecret`, `TelegramPollingTimeout`, режим работы.
- `backend/.env.example` — добавить новые env с пометками.

**Миграция**
- `backend/migrations/<ts>_creator_applications_telegram.sql` —
  поле `telegram_user_id BIGINT NULL` (или отдельная таблица; решение в plan).
  Partial unique index, симметричный `iin`-индексу: `UNIQUE
  (telegram_user_id) WHERE status IN ('pending','approved','blocked')` —
  чтобы один TG-аккаунт не привязался к двум активным заявкам.

**Domain**
- `backend/internal/domain/creator_application.go` — новые ошибочные коды
  (`CodeApplicationAlreadyLinked`, `CodeTelegramAlreadyLinked`,
  `CodeApplicationNotFound`), доменные типы `TelegramLinkInput`,
  `TelegramUpdate` (или импорт типа из библиотеки), sentinel-ошибки.

**Repository**
- `backend/internal/repository/creator_application.go` — методы
  `LinkTelegram(ctx, applicationID, tgUserID) error` (с обработкой
  partial-unique violation → domain-error),
  `GetByTelegramUserID(ctx, tgUserID) (*CreatorApplicationRow, error)` (для
  re-issue: тот же TG к той же заявке).
- `backend/internal/repository/factory.go` — без изменений (используется тот
  же `CreatorApplicationRepo`).

**Bot integration package** — `backend/internal/integration/telegram/`
- `client.go` — обёртка над выбранной библиотекой (отправка сообщений,
  валидация webhook'а; через интерфейс — для unit-тестов сервиса).
- `bot.go` — диспатчер update'ов: парсит входящий update, маршрутизирует к
  обработчикам (`/start`, fallback). Транспорт-нейтрален: один и тот же
  диспатчер вызывается и из polling-loop, и из webhook-хендлера.
- `runner.go` — long-polling runner (если выбран polling) с graceful shutdown
  через context.
- `handler_start.go` — обработчик `/start <payload>`: валидация payload,
  вызов сервиса, формирование reply.

**Service layer**
- `backend/internal/service/creator_application_telegram.go` (новый файл рядом
  с `creator_application.go`) — `LinkTelegramToApplication(ctx, applicationID,
  tgUserID, tgMeta)` (с `dbutil.WithTx` + audit в той же tx). Audit-action
  новый: `AuditActionCreatorApplicationLinkTelegram` (system actor, actor_id=NULL).

**Cmd wiring**
- `backend/cmd/api/main.go` — старт runner'а после `pool.Ping`, регистрация
  closer'а; injection клиента/диспатчера/сервиса.

**HTTP webhook (только если режим webhook)**
- `backend/internal/handler/telegram_webhook.go` — приём update'ов
  (через codegen-роут), middleware-валидация secret-token (Telegram
  поддерживает `secret_token` header при `setWebhook`).

**Тесты**
- `backend/internal/integration/telegram/*_test.go` — unit на диспатчер и
  `/start` handler с моком клиента.
- `backend/internal/service/creator_application_telegram_test.go` — unit
  service-слоя.
- `backend/internal/repository/creator_application_test.go` — расширение
  существующего файла (тесты на `LinkTelegram` / `GetByTelegramUserID`).
- `backend/e2e/telegram/telegram_link_test.go` — e2e (отдельный пакет).
  Стратегия — закрытое тестирование без живого Telegram (см. §6.4).

**Тестовые ручки**
- `backend/api/openapi-test.yaml` — новый `/test/telegram/send-update`
  (имитация update'а от Telegram, доступна при `EnableTestEndpoints`). E2E
  использует её, чтобы прогнать `/start` без поднятия polling-loop'а или
  выпуска webhook'а наружу.

**Дока**
- `docs/standards/backend-libraries.md` — добавить выбранную Telegram-библиотеку
  в реестр + сноска про обоснование выбора.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — пометить
  chunk 1 как `[~]` при начале и `[x]` после merge.

## 4. Связанные паттерны (как уже делают похожие вещи)

- **Атомарность изменения + audit.**
  `service/creator_application.go:80-158` — `dbutil.WithTx` оборачивает
  все INSERTs и audit-запись, log-success **после** `WithTx` (стандарт
  `backend-transactions.md`).
- **Background goroutine + graceful shutdown.**
  `cmd/api/main.go:67-75` (cron). Регистрация в closer через `cl.Add(name,
  closeFunc)` — runner Telegram-бота должен делать то же самое: остановка
  по `Stop(ctx)` или отмене context'а.
- **Public action без аутентифицированного актёра.**
  Submit-ручка пишет audit с `actor_id = NULL` — тот же приём для бота
  (system action: `actor_role = "system"` или пустой).
- **Race-handling при partial unique index.**
  `repository/creator_application.go:105-118` — `errors.As` на
  `*pgconn.PgError`, проверка `Code == "23505"` + имени констрейнта,
  трансляция в `domain.Err*` (стандарт `backend-repository.md`). Тот же
  подход применить для `creator_applications_telegram_user_id_active_idx`.
- **Тестовые ручки для setup/teardown.**
  `handler/testapi.go` + cleanup-stack — паттерн для имитации Telegram update.
- **Codegen-first.** Все HTTP-ручки — через openapi.yaml. Webhook не
  исключение, если он публичный.

## 5. Архитектурные варианты (решаются на стадии plan)

> Перечислены без принятия решения — это работа планировщика. Каждый вариант
> с pros/cons, чтобы план мог быстро зафиксировать выбор без повторного
> исследования.

### 5.1 Транспорт: long polling vs webhook

| Аспект | Long polling | Webhook |
|---|---|---|
| Прод-ready | Да | Да |
| Локалка / e2e | Прост — без публичного URL | Сложно — нужен tunnel или подмена транспорта |
| Latency | ~1-2 сек (Telegram timeout) | <100 мс |
| Нагрузка на сервер | Постоянное соединение к API Telegram | По событию |
| Failure mode | Reconnect автоматический | Telegram повторяет push с retry |
| Безопасность | Не нужно валидировать вход | `secret_token` в header при `setWebhook` |
| Совместимость с Dokploy | Без изменений | Нужно проверить, что https-домен бота смотрит на наш сервис |

**Тривиальная стратегия:** long polling везде (включая прод) — простая, не
требует SSL/tunnel, MVP-достаточная. Переход на webhook — отдельный chunk.
План должен явно зафиксировать выбор.

### 5.2 Размещение бота: тот же процесс vs отдельный процесс

| Аспект | В процессе API (goroutine) | Отдельный бинарник `cmd/bot` |
|---|---|---|
| Сложность wiring | Минимум — переиспользуем pool, repo, services | Дублирование setup |
| Деплой | Один контейнер | Два контейнера, две конфиги |
| Изоляция отказов | Падение бота = падение API | Изолировано |
| Зависимость от БД | Один пул соединений | Два пула |

**Тривиальная стратегия:** в процессе API. Прецедент — cron уже там. Если
понадобится горизонтальное масштабирование (несколько API-инстансов) —
бот должен запускаться **только в одном** (lock в БД или leader election),
иначе double-processing update'ов. Для MVP — один инстанс API.

### 5.3 Схема хранения привязки

| Вариант | Pros | Cons |
|---|---|---|
| Колонка `creator_applications.telegram_user_id BIGINT NULL` | Простота, тривиальная миграция | Метаданные (username, first_name) либо мешать в строку, либо терять |
| Отдельная таблица `creator_application_telegram_links` | Гибче, можно хранить `username`, `first_name`, `linked_at`; легче расширить (например, на user-сущность) | Лишняя таблица для одного поля; JOIN при чтении |

**Тривиальная стратегия:** колонка для MVP (telegram_user_id + опционально
telegram_username). Метаданные нужны только для отображения; их можно
хранить минимально или вовсе не хранить (брать из API при необходимости).
Решение принимает план.

### 5.4 Библиотека Telegram Bot API

Кандидаты (на pkg.go.dev / Awesome Go, актуальны по состоянию 2026-04):

| Библиотека | Maintained | API стиль | Размер |
|---|---|---|---|
| `github.com/go-telegram/bot` | Активная | Идиоматичный, типизированный, plugin-friendly | Минимальный |
| `github.com/mymmrac/telego` | Активная | Generated из Bot API spec, типобезопасный | Средний |
| `github.com/go-telegram-bot-api/telegram-bot-api` | Замедлилась | Старая, императивная | Минимальный |
| `gopkg.in/telebot.v4` (`tucnak/telebot`) | Активная | Decorator-style маршрутизация | Средний |

**Критерии выбора (на стадии plan):** активная поддержка, чистый API,
совместимость с long polling и webhook, удобство мокинга для unit-тестов.
План должен пополнить `docs/standards/backend-libraries.md`.

### 5.5 Race-сценарии привязки

| Сценарий | Поведение |
|---|---|
| Заявка уже привязана к другому TG | Отказать с понятным сообщением — «Эта заявка уже привязана к другому Telegram-аккаунту». Безопасно: первый победил |
| Заявка уже привязана к этому же TG (повторный /start) | Идемпотентный успех — повторить reply без дублирующего audit |
| Этот TG уже привязан к **активной** заявке (rejected — не считается) | Отказать — «У вас уже есть активная заявка». Защищает от смены identity |
| Этот TG привязан к старой `rejected`-заявке, новая заявка с тем же TG | Разрешить (partial unique index `WHERE status IN ('pending','approved','blocked')` не блокирует rejected) |
| Заявка не существует / неверный UUID в payload | Reply: «Не удалось найти заявку. Подайте новую заявку на ugcboost.kz» |
| Заявка `rejected` / `blocked` | Reply объясняет статус, привязку не делаем |

План фиксирует точные тексты reply (тон — официальный, без жаргона) и
коды ошибок.

## 6. Тестирование

### 6.1 Unit-слой

- **`/start` handler** — мокаем сервис, проверяем парсинг payload,
  идемпотентность, обработку ошибок.
- **Service** — мокаем репо/audit, проверяем атомарность (race на
  partial unique → domain-error → reply).
- **Repository** — `pgxmock` на `LinkTelegram` / `GetByTelegramUserID`,
  включая SQLSTATE 23505 на indexed нарушении.

### 6.2 E2E

Стратегия — **закрытое тестирование** без живого Telegram. Тестовая ручка
`POST /test/telegram/send-update` (только при `EnableTestEndpoints`) принимает
JSON update в формате Telegram и пушит его в тот же диспатчер, что и
polling/webhook. Это **не отправка реальному Telegram** — это инжект
events в нашу логику.

Сценарии (минимум):
1. Happy path: создаётся заявка через POST `/creators/applications`, шлём
   `/start <appID>`, после — GET (admin) показывает `telegramUserId`
   привязанным; audit-row с `link_telegram` action.
2. Дубль /start от того же пользователя — идемпотентно, второй audit-row
   не пишется.
3. /start от другого пользователя на ту же заявку — reply «уже привязана»,
   привязка не меняется.
4. /start с несуществующим UUID — reply «не нашли заявку», ничего не
   пишется.
5. /start без payload (`/start` без аргумента) — reply-инструкция.

PII-guard: после happy-path grep `docker logs backend` по ИИН/ФИО/
phone/handle = 0.

### 6.3 Тестируем ли реальный Telegram?

Для MVP — нет. Это вне scope chunk 1. Проверка живого бота — manual smoke
после деплоя в staging (один человек прогоняет руками).

### 6.4 CI

Лучше всего — `make test-e2e-backend` уже существующим target (тестовый
endpoint поднимается вместе с api в Docker). Нужно убедиться, что без
`TELEGRAM_BOT_TOKEN` бот **не падает на старте** в test-режиме (флаг
`TELEGRAM_MOCK=true` или отсутствие token'а → no-op runner, диспатчер
работает только через test-endpoint).

## 7. Риски и edge cases

- **Двойной запуск polling-loop'а** при горизонтальном scaling приведёт к
  double-processing update'ов. План должен зафиксировать ограничение «один
  инстанс» или leader election.
- **Telegram retries webhook** — нужна идемпотентность по `update_id`
  (хранить последний обработанный update_id или дедуплицировать на
  application-уровне через идемпотентность бизнес-операции).
- **Race с самой заявкой:** теоретически в момент `/start` админ может
  поменять статус на `rejected`. Поведение должно быть детерминированным —
  делаем link только при активном статусе, иначе reply про статус.
- **Payload-инъекция.** Telegram передаёт `/start <payload>` как plaintext
  в сообщении. Парсер должен принимать только UUID (regex / `uuid.Parse`).
  Любой мусор → reply-инструкция.
- **Username бота меняется.** Если меняется `TELEGRAM_BOT_USERNAME`, старые
  deep-link'и в email/CRM/сторонних местах перестают работать — но мы их и
  не сохраняли. Риск низкий.
- **Лимиты Telegram API** — 30 messages/sec в одну группу. Для chunk 1
  трафик мизерный, не блокер. Учесть на этапе массовых рассылок.
- **Полная остановка бота при ошибке long polling** — runner должен
  переподключаться (exponential backoff), а не падать. Стандарт ошибок
  применяется.
- **Audit-row на link Telegram** должен быть в той же tx, что и
  `UPDATE creator_applications SET telegram_user_id = ...`. Иначе классика
  «tx откатилась, audit соврал».
- **PII в логах.** Telegram `username` / `first_name` — PII. В stdout
  `logger.Info` они недопустимы. В `audit_logs` — допустимы (стандарт
  `security.md`).
- **Длина username/first_name** не ограничена Telegram-ом строго, но в
  audit/db нужно cap'нуть (например, 64/256), иначе DoS.
- **Down-миграция.** Если поле/таблица NOT NULL — Down при наличии данных
  упадёт. Изначально делаем NULLABLE, тогда down безопасен.
- **i18n reply.** Тон — официальный, проф, грамотный (требование roadmap).
  Тексты — на русском.

## 8. Внешние зависимости

- **Token бота.** Alikhan предоставит. В .env.example оставляем пустую
  строку с пометкой «получить у tech lead», в Dokploy / GitHub Secrets
  заводится отдельно.
- **Username бота** — уже есть в config (`ugcboost_staging_bot`).
- **Доступ Telegram Bot API из CI.** При long polling и моках в CI бот
  не должен реально дёргать api.telegram.org. Test-режим: `TELEGRAM_MOCK=true`
  или отсутствие токена → клиент = mock implementation.

## 9. Оценка scope (для plan)

Не для финальной декомпозиции — ориентир, чтобы план не растёкся:

- Миграция + repo: ~1 файл миграции, 2 метода в repo, тесты.
- Telegram client + dispatcher: 4–5 файлов в `internal/integration/telegram/`,
  unit-тесты.
- Service: 1 новый метод (LinkTelegram), unit-тесты.
- Wiring в main.go: ~30 строк.
- Test endpoint + e2e: 1 testapi-handler + 1 e2e-файл.
- Стандарты: пополнение реестра libraries.

## 10. Дефолты, которые план может зафиксировать без вопросов

Если планировщик не находит причин отклониться — берёт эти дефолты:

- **Транспорт:** long polling.
- **Размещение:** в процессе API, отдельная goroutine, регистрация в closer.
- **Хранение:** колонка `telegram_user_id BIGINT NULL` + опционально
  `telegram_username TEXT NULL` + `telegram_linked_at TIMESTAMPTZ NULL` в
  `creator_applications`. Partial unique index по `telegram_user_id` для
  активных статусов.
- **Стратегия e2e:** через test-endpoint, без живого Telegram.
- **Reply-тоны:** официальный, на русском, без эмодзи и жаргона.
- **Команды бота в этом chunk:** только `/start`. Любое другое сообщение —
  reply-инструкция «откройте мини-приложение / подайте заявку на
  ugcboost.kz» (точный текст — в плане).

## 11. Что вне scope chunk 1

- Меню бота, /help, /status, /cancel — не делаем.
- TMA mini-app — chunk 2.
- Уведомления креатора (отклонение, запрос смены категории, призыв подписать
  договор) — следующие chunk'и (5/6).
- Webhook (как альтернатива polling) — отдельный chunk при необходимости.
- LiveDune, TrustMe — отдельные chunk'и онбординга.

---

**Артефакт готов для plan.** Следующий шаг — `/plan` с этим scout'ом.
Планировщик выбирает варианты из §5, фиксирует точные ID, тексты, имена
файлов, таски и acceptance.
